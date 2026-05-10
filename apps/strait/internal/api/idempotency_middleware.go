package api

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/store"
)

// idempotencyKeyTTL is the default time-to-live for idempotency keys.
const idempotencyKeyTTL = 24 * time.Hour

// maxIdempotencyResponseBytes caps the in-memory body buffer the
// middleware retains for replay. Mirrors maxRLSBufferedResponseBytes.
// Responses that exceed the cap are streamed to the client in full but
// are never memoized: the pending row is deleted instead so retries
// proceed without seeing a truncated cached body.
const maxIdempotencyResponseBytes = 16 << 20

// idempotencyCleanupTimeout caps how long the middleware will block
// waiting for DeleteIdempotencyKey when releasing a pending row after a
// panic or a non-2xx response. The cleanup runs against a context that
// outlives the request (context.WithoutCancel) so the request handler
// canceling its own ctx -- e.g. timeout middleware firing -- does not
// strand the pending row for the full TTL.
const idempotencyCleanupTimeout = 5 * time.Second

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

		// Scope the key to the calling actor, request path, and environment.
		// Without the actor prefix, two callers in the same project who pick
		// the same Idempotency-Key string would share a cache entry: a
		// low-privilege caller could read another caller's cached response
		// (or its 409 pending row would block them) by guessing the key.
		// Handler-level environment checks do not run when a completed key
		// is replayed from cache, so the env id must stay in the composite.
		compositeKey := actorFromContext(r.Context()) + ":" + r.URL.Path + ":env:" + environmentIDFromContext(r.Context()) + ":" + key

		status, respStatus, respBody, err := s.store.TryAcquireIdempotencyKey(r.Context(), projectID, compositeKey, idempotencyKeyTTL)
		if err != nil {
			slog.Error("idempotency key acquire failed",
				"idempotency_key_hash", hashIdempotencyKey(key),
				"project_id", projectID,
				"error", err)
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
			// Use a detached context so cleanup outlives a canceled
			// request (timeout middleware, client disconnect).
			panicked := true
			defer func() {
				if panicked {
					cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), idempotencyCleanupTimeout)
					defer cancel()
					_, _ = s.store.DeleteIdempotencyKey(cleanupCtx, projectID, compositeKey)
				}
			}()

			next.ServeHTTP(cw, r)
			panicked = false

			// If the captured body overflowed the cap, the caller still
			// got the full response but we cannot memoize a truncated
			// body for replay. Drop the pending row so retries proceed.
			if cw.overflow {
				slog.Warn("idempotency response exceeds cap; dropping cache entry",
					"idempotency_key_hash", hashIdempotencyKey(key),
					"project_id", projectID,
					"cap_bytes", maxIdempotencyResponseBytes)
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), idempotencyCleanupTimeout)
				_, _ = s.store.DeleteIdempotencyKey(cleanupCtx, projectID, compositeKey)
				cancel()
				return
			}

			// Only cache 2xx responses. Error responses delete the pending
			// row so the client can retry with the same key. Cleanup runs
			// on a detached context for the same reason as the panic
			// branch above.
			if cw.statusCode >= 200 && cw.statusCode < 300 {
				if completeErr := s.store.CompleteIdempotencyKey(r.Context(), projectID, compositeKey, cw.statusCode, cw.body.Bytes()); completeErr != nil {
					slog.Error("idempotency key complete failed",
						"idempotency_key_hash", hashIdempotencyKey(key),
						"project_id", projectID,
						"error", completeErr)
				}
			} else {
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), idempotencyCleanupTimeout)
				_, _ = s.store.DeleteIdempotencyKey(cleanupCtx, projectID, compositeKey)
				cancel()
			}
			return

		default:
			next.ServeHTTP(w, r)
		}
	})
}

// captureWriter wraps http.ResponseWriter to capture the response status
// and body for replay. Body capture is bounded by maxIdempotencyResponseBytes:
// once exceeded, overflow is set, the buffer stops growing, and the
// caller still receives the full bytes via the underlying writer.
type captureWriter struct {
	http.ResponseWriter
	statusCode  int
	body        bytes.Buffer
	wroteHeader bool
	overflow    bool
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
	if !cw.overflow {
		remaining := maxIdempotencyResponseBytes - cw.body.Len()
		if remaining <= 0 {
			cw.overflow = true
		} else if len(b) <= remaining {
			cw.body.Write(b)
		} else {
			cw.body.Write(b[:remaining])
			cw.overflow = true
		}
	}
	return cw.ResponseWriter.Write(b)
}
