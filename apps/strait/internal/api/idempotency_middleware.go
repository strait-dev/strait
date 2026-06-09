package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"path"
	"sync"
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

// canonicalizeIdempotencyPath collapses cosmetic path differences so
// callers that hit the same logical resource hash to the same composite
// key. Without this, a client retrying with "/v1/jobs/" or "/v1//jobs"
// would compute a different key than the original "/v1/jobs" call and
// re-execute the operation. path.Clean handles "..", "//", and trailing
// slashes. It intentionally preserves case because chi routes are
// case-sensitive and distinct routes must not share an idempotency cache key.
func canonicalizeIdempotencyPath(p string) string {
	if p == "" {
		return "/"
	}
	cleaned := path.Clean(p)
	// path.Clean on "/foo/" yields "/foo", on "/" yields "/", on "" yields ".".
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

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
	cleanupCtx, cancel := context.WithTimeout(store.ContextWithoutTx(context.WithoutCancel(parentCtx)), s.idempotencyCleanupTimeout())
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

		// Without an actor, two callers in the same project who pick
		// the same Idempotency-Key would collapse into one cache entry
		// and one of them would silently replay the other's response.
		// Internal-secret-authenticated requests (scheduler, orchestrator,
		// app→API hops) are trusted and need legitimate dedupe across
		// retries, so scope them under a constant "internal" actor; that
		// preserves dedupe within the trust boundary while keeping
		// genuinely anonymous callers off the cache entirely.
		actorID := actorFromContext(r.Context())
		if actorID == "" {
			if !isInternalCaller(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}
			actorID = "internal"
		}

		compositeKey := idempotencyCompositeKey(
			actorID,
			canonicalizeIdempotencyPath(r.URL.Path),
			environmentIDFromContext(r.Context()),
			key,
		)
		keyHash := hashIdempotencyKey(key)

		status, respStatus, respHeaders, respBody, err := s.store.TryAcquireIdempotencyKey(r.Context(), projectID, compositeKey, idempotencyKeyTTL)
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
			// Replay the captured headers verbatim so retries observe the
			// same Content-Type, Location, ETag, Set-Cookie, etc. as the
			// original response. Rows written before the header-snapshot
			// migration store nil headers; fall back to JSON for those.
			dst := w.Header()
			if len(respHeaders) > 0 {
				for k, vals := range respHeaders {
					dst[k] = append([]string(nil), vals...)
				}
			} else {
				dst.Set("Content-Type", "application/json")
			}
			dst.Set("Idempotency-Replayed", "true")
			w.WriteHeader(respStatus)
			_, _ = w.Write(respBody) // #nosec G705 -- respBody is our own cached response, not user-controlled
			return

		case store.IdempotencyPending:
			respondError(w, r, http.StatusConflict,
				"a request with this idempotency key is currently being processed")
			return

		case store.IdempotencyAcquired:
			cw := &captureWriter{ResponseWriter: w, headers: http.Header{}}

			// Use recover() to clean up unconditionally on panic, no
			// matter where in the post-acquire flow the panic happens
			// (handler, CompleteIdempotencyKey, or the cleanup paths
			// themselves). The original panic is re-raised so chi's
			// recoverer still produces a 500 for the client.
			// sync.Once rather than a plain bool: the cleanup closure is reachable
			// from the panic-recovery defer, the overflow branch, and the tx
			// rollback hook. A plain bool would be a TOCTOU hazard if any of those
			// ever ran concurrently.
			var cleanupOnce sync.Once
			cleanup := func() {
				cleanupOnce.Do(func() {
					s.runIdempotencyCleanup(r.Context(), projectID, compositeKey, keyHash)
				})
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
				complete := func() {
					// Detach from r.Context() because chi's timeout middleware
					// and client disconnects cancel it the moment the handler
					// returns; a canceled context here would race the cache
					// write and force the next retry to re-execute the
					// already-successful operation.
					completeCtx, completeCancel := context.WithTimeout(
						store.ContextWithoutTx(context.WithoutCancel(r.Context())),
						s.idempotencyCleanupTimeout(),
					)
					defer completeCancel()
					if completeErr := s.store.CompleteIdempotencyKey(completeCtx, projectID, compositeKey, cw.statusCode, cw.headers, cw.body.Bytes()); completeErr != nil {
						slog.Error("idempotency key complete failed",
							"idempotency_key_hash", keyHash,
							"project_id", projectID,
							"error", completeErr)
						// Drop the pending row so retries are not blocked by
						// 409 until TTL expires; replay cache is sacrificed.
						cleanup()
					}
				}
				if registerTxCompletionHooks(r.Context(), func(context.Context) {
					complete()
				}, func(context.Context) {
					cleanup()
				}) {
					return
				}
				complete()
			} else {
				cleanup()
			}
			return

		default:
			slog.Error("idempotency store returned unrecognized status; falling through",
				"idempotency_key_hash", keyHash,
				"project_id", projectID,
				"status", status)
			next.ServeHTTP(w, r)
		}
	})
}

// captureWriter wraps http.ResponseWriter to capture the response
// status, headers, and body for replay. Body capture is bounded by
// maxIdempotencyResponseBytes: once exceeded, overflow is set, the
// buffer stops growing, and the caller still receives the full bytes
// via the underlying writer. Headers are snapshotted at WriteHeader
// time so replays reproduce the original Content-Type, Location, ETag,
// Set-Cookie, etc.
type captureWriter struct {
	http.ResponseWriter
	statusCode  int
	headers     http.Header
	body        bytes.Buffer
	wroteHeader bool
	overflow    bool
}

func (cw *captureWriter) WriteHeader(code int) {
	if !cw.wroteHeader {
		cw.statusCode = code
		// Snapshot the handler's headers at the moment of commit. We
		// .Clone() because the underlying writer is allowed to mutate
		// the map after WriteHeader returns (e.g. trailers, internal
		// hop-by-hop adjustments) and we do not want the cache write
		// to race with that.
		cw.headers = cw.ResponseWriter.Header().Clone()
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

// Flush forwards to the underlying writer when it implements
// http.Flusher. Without this method, handlers that call w.(http.Flusher)
// from under idempotencyMiddleware would fail the type assertion, so any
// future SSE-style endpoint accepting Idempotency-Key would 500. The captured
// body is independent of flushes; bytes already streamed remain in cw.body.
func (cw *captureWriter) Flush() {
	if f, ok := cw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack lets WebSocket and similar long-lived upgrade handlers escape
// the buffered-capture path. Returns http.ErrNotSupported (matching
// Push and stdlib convention) when the underlying writer is not a
// Hijacker so callers using errors.Is can detect the missing capability.
func (cw *captureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := cw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Push forwards HTTP/2 server push when supported; otherwise returns
// http.ErrNotSupported so callers fall back to non-push delivery rather
// than misinterpret a silent no-op.
func (cw *captureWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := cw.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}
