package api

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/store"
)

// idempotencyKeyTTL is the default time-to-live for idempotency keys.
const idempotencyKeyTTL = 24 * time.Hour

// idempotencyMiddleware intercepts POST requests that carry an Idempotency-Key
// (or X-Idempotency-Key) header. It implements the insert-pending-first pattern:
//
//  1. Try to INSERT a pending row for the (project_id, key) pair.
//  2. If acquired, execute the handler, capture the response, and UPDATE the row.
//  3. If completed, replay the cached response with an Idempotency-Replayed header.
//  4. If pending (another request is processing), return 409 Conflict.
func (s *Server) idempotencyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			key = r.Header.Get("X-Idempotency-Key")
		}
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}

		if len(key) > maxIdempotencyKeyLength {
			respondError(w, r, http.StatusBadRequest,
				fmt.Sprintf("idempotency key must be %d characters or fewer", maxIdempotencyKeyLength))
			return
		}

		projectID := projectIDFromContext(r.Context())
		if projectID == "" {
			next.ServeHTTP(w, r)
			return
		}

		status, respStatus, respBody, err := s.store.TryAcquireIdempotencyKey(r.Context(), projectID, key, idempotencyKeyTTL)
		if err != nil {
			slog.Error("idempotency key acquire failed", "key", key, "project_id", projectID, "error", err)
			next.ServeHTTP(w, r)
			return
		}

		switch status {
		case store.IdempotencyComplete:
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Idempotency-Replayed", "true")
			w.WriteHeader(respStatus)
			_, _ = w.Write(respBody)
			return

		case store.IdempotencyPending:
			respondError(w, r, http.StatusConflict,
				"a request with this idempotency key is currently being processed")
			return

		case store.IdempotencyAcquired:
			cw := &captureWriter{ResponseWriter: w}
			next.ServeHTTP(cw, r)

			if err := s.store.CompleteIdempotencyKey(r.Context(), projectID, key, cw.statusCode, cw.body.Bytes()); err != nil {
				slog.Error("idempotency key complete failed", "key", key, "project_id", projectID, "error", err)
			}
			return

		default:
			next.ServeHTTP(w, r)
		}
	})
}

// captureWriter wraps http.ResponseWriter to capture the response status and body
// while still writing to the original writer.
type captureWriter struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (cw *captureWriter) WriteHeader(code int) {
	cw.statusCode = code
	cw.ResponseWriter.WriteHeader(code)
}

func (cw *captureWriter) Write(b []byte) (int, error) {
	if cw.statusCode == 0 {
		cw.statusCode = http.StatusOK
	}
	cw.body.Write(b)
	return cw.ResponseWriter.Write(b)
}
