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
//  1. Try to INSERT a pending row for the (project_id, compositeKey) pair.
//  2. If acquired, execute the handler, capture the response, and UPDATE the row.
//  3. If completed, replay the cached response with an Idempotency-Replayed header.
//  4. If pending (another request is processing), return 409 Conflict.
//
// The composite key includes the request path to prevent cross-endpoint replay.
// Only 2xx responses are cached; error responses delete the pending row so
// the key can be retried.
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

		// Scope the key to the request path and caller environment to prevent
		// cross-endpoint and cross-environment replay. Handler-level environment
		// checks do not run when a completed key is replayed from cache.
		compositeKey := r.URL.Path + ":env:" + environmentIDFromContext(r.Context()) + ":" + key

		status, respStatus, respBody, err := s.store.TryAcquireIdempotencyKey(r.Context(), projectID, compositeKey, idempotencyKeyTTL)
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
			_, _ = w.Write(respBody) // #nosec G705 -- respBody is our own cached JSON response, not user-controlled
			return

		case store.IdempotencyPending:
			respondError(w, r, http.StatusConflict,
				"a request with this idempotency key is currently being processed")
			return

		case store.IdempotencyAcquired:
			cw := &captureWriter{ResponseWriter: w}

			// If the handler panics, clean up the pending row so the key
			// can be retried instead of being stuck for the full TTL.
			panicked := true
			defer func() {
				if panicked {
					_, _ = s.store.DeleteIdempotencyKey(r.Context(), projectID, compositeKey)
				}
			}()

			next.ServeHTTP(cw, r)
			panicked = false

			// Only cache 2xx responses. Error responses delete the pending
			// row so the client can retry with the same key.
			if cw.statusCode >= 200 && cw.statusCode < 300 {
				if completeErr := s.store.CompleteIdempotencyKey(r.Context(), projectID, compositeKey, cw.statusCode, cw.body.Bytes()); completeErr != nil {
					slog.Error("idempotency key complete failed", "key", key, "project_id", projectID, "error", completeErr)
				}
			} else {
				_, _ = s.store.DeleteIdempotencyKey(r.Context(), projectID, compositeKey)
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
	statusCode  int
	body        bytes.Buffer
	wroteHeader bool
}

func (cw *captureWriter) WriteHeader(code int) {
	if !cw.wroteHeader {
		cw.statusCode = code
		cw.wroteHeader = true
		cw.ResponseWriter.WriteHeader(code)
	}
}

func (cw *captureWriter) Write(b []byte) (int, error) {
	if !cw.wroteHeader {
		cw.WriteHeader(http.StatusOK)
	}
	cw.body.Write(b)
	return cw.ResponseWriter.Write(b)
}
