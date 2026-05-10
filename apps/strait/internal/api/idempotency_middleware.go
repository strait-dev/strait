package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
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

// defaultIdempotencyCleanupTimeout is the fallback when
// config.IdempotencyCleanupTimeout is unset (test servers, missing
// env). It caps how long the middleware will block waiting for
// DeleteIdempotencyKey when releasing a pending row after a panic, a
// non-2xx response, or an overflowed body. The cleanup runs against a
// context that outlives the request (context.WithoutCancel) so the
// request handler canceling its own ctx does not strand the pending
// row for the full TTL.
const defaultIdempotencyCleanupTimeout = 5 * time.Second

// idempotencyCompositeKey returns a length-prefixed SHA-256 of the
// scoping components. Length prefixes make the encoding injective so
// two distinct (actor, path, env, key) tuples can never collide via
// separator confusion (e.g. a path containing ":env:"). The hex digest
// is also bounded to a known column-friendly length regardless of the
// input sizes.
func idempotencyCompositeKey(actorID, path, envID, key string) string {
	h := sha256.New()
	for _, part := range []string{actorID, path, envID, key} {
		// All four inputs are HTTP-bounded: keys ≤ maxIdempotencyKeyLength,
		// paths capped by chi's request line limit, actor/env ids fit in a
		// CHAR(36)-style column. Clamping to math.MaxUint32 is therefore a
		// noop in practice, but the explicit guard makes the int→uint32
		// conversion provably safe and silences gosec G115.
		n := min(len(part), math.MaxUint32)
		var lenBuf [4]byte
		binary.BigEndian.PutUint32(lenBuf[:], uint32(n)) // #nosec G115 -- clamped above to math.MaxUint32
		_, _ = h.Write(lenBuf[:])
		_, _ = h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// idempotencyCleanupTimeout returns the configured cleanup deadline,
// falling back to defaultIdempotencyCleanupTimeout when config is nil
// or unset. Centralizing the lookup avoids nil-deref in tests that
// build a Server without a fully-populated config.
func (s *Server) idempotencyCleanupTimeout() time.Duration {
	if s.config != nil && s.config.IdempotencyCleanupTimeout > 0 {
		return s.config.IdempotencyCleanupTimeout
	}
	return defaultIdempotencyCleanupTimeout
}

// idempotencyFailOpen reports whether the middleware should fall
// through to the handler (no dedupe) when the idempotency store is
// unreachable. Default is false: fail closed with 503 so non-idempotent
// operations are never executed twice during a store outage.
func (s *Server) idempotencyFailOpen() bool {
	return s.config != nil && s.config.IdempotencyFailOpen
}

// runIdempotencyCleanup executes DeleteIdempotencyKey against a
// detached, time-bounded context and surfaces failures via slog so an
// operator can alert on stuck pending rows. The detached context
// (context.WithoutCancel) means cleanup outlives a canceled request
// (timeout middleware, client disconnect). defer cancel() guarantees
// timer release even if DeleteIdempotencyKey panics.
func (s *Server) runIdempotencyCleanup(parentCtx context.Context, projectID, compositeKey, keyHash string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(parentCtx), s.idempotencyCleanupTimeout())
	defer cancel()
	if _, err := s.store.DeleteIdempotencyKey(cleanupCtx, projectID, compositeKey); err != nil {
		slog.Warn("idempotency cleanup failed; pending row may block retries until TTL",
			"idempotency_key_hash", keyHash,
			"project_id", projectID,
			"ttl", idempotencyKeyTTL,
			"error", err)
	}
}

// idempotencyMiddleware intercepts POST requests that carry an Idempotency-Key
// (or X-Idempotency-Key) header. It implements the insert-pending-first pattern:
//
//  1. Try to INSERT a pending row for the (project_id, compositeKey) pair.
//  2. If acquired, execute the handler, capture the response, and UPDATE the row.
//  3. If completed, replay the cached response with an Idempotency-Replayed header.
//  4. If pending (another request is processing), return 409 Conflict.
//
// The composite key is a hash of (actor, path, env, key) so two callers in
// the same project who pick the same Idempotency-Key string never share a
// cache entry. Only 2xx responses are cached; error responses delete the
// pending row so the key can be retried.
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

		compositeKey := idempotencyCompositeKey(
			actorFromContext(r.Context()),
			r.URL.Path,
			environmentIDFromContext(r.Context()),
			key,
		)
		keyHash := hashIdempotencyKey(key)

		status, respStatus, respBody, err := s.store.TryAcquireIdempotencyKey(r.Context(), projectID, compositeKey, idempotencyKeyTTL)
		if err != nil {
			slog.Error("idempotency key acquire failed",
				"idempotency_key_hash", keyHash,
				"project_id", projectID,
				"error", err)
			if !s.idempotencyFailOpen() {
				respondError(w, r, http.StatusServiceUnavailable,
					"idempotency store unavailable; retry later")
				return
			}
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

			// Use recover() to clean up unconditionally on panic, no
			// matter where in the post-acquire flow the panic happens
			// (handler, CompleteIdempotencyKey, or the cleanup paths
			// themselves). The original panic is re-raised so chi's
			// recoverer still produces a 500 for the client.
			cleanupOnce := false
			cleanup := func() {
				if cleanupOnce {
					return
				}
				cleanupOnce = true
				s.runIdempotencyCleanup(r.Context(), projectID, compositeKey, keyHash)
			}
			defer func() {
				if rec := recover(); rec != nil {
					cleanup()
					panic(rec)
				}
			}()

			next.ServeHTTP(cw, r)

			// If the captured body overflowed the cap, the caller still
			// got the full response but we cannot memoize a truncated
			// body for replay. Drop the pending row so retries proceed.
			if cw.overflow {
				slog.Warn("idempotency response exceeds cap; dropping cache entry",
					"idempotency_key_hash", keyHash,
					"project_id", projectID,
					"cap_bytes", maxIdempotencyResponseBytes)
				cleanup()
				return
			}

			// Only cache 2xx responses. Error responses delete the pending
			// row so the client can retry with the same key.
			if cw.statusCode >= 200 && cw.statusCode < 300 {
				if completeErr := s.store.CompleteIdempotencyKey(r.Context(), projectID, compositeKey, cw.statusCode, cw.body.Bytes()); completeErr != nil {
					slog.Error("idempotency key complete failed",
						"idempotency_key_hash", keyHash,
						"project_id", projectID,
						"error", completeErr)
					// Pending row would otherwise sit until TTL and
					// block every retry with 409. Better to clear it
					// even though we lose the replay cache for this
					// key — caller can safely retry.
					cleanup()
				}
			} else {
				cleanup()
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
