package api

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"strait/internal/ratelimit"

	chimw "github.com/go-chi/chi/v5/middleware"
)

func (s *Server) profilingAuth(next http.Handler) http.Handler {
	if s.config.ProfilingSecret == "" {
		return s.internalSecretAuth(next)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := realIP(r, s.trustedProxies)
		if blocked, retryAfter := s.authLimiter.IsBlockedScoped(r.Context(), clientIP, ratelimit.AuthScopeInternalSecret); blocked {
			recordAuthDecision(r.Context(), "profiling_secret", "throttled")
			recordAuthRateLimitThrottled(r.Context(), "profiling_secret")
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			respondError(w, r, http.StatusTooManyRequests, ratelimit.BlockedError(retryAfter))
			return
		}

		secret := internalSecretFromRequest(r)
		if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(s.config.ProfilingSecret)) != 1 {
			s.authLimiter.RecordFailureScoped(r.Context(), clientIP, ratelimit.AuthScopeInternalSecret)
			recordAuthDecision(r.Context(), "profiling_secret", "failure")
			respondError(w, r, http.StatusUnauthorized, "invalid or missing profiling secret")
			return
		}

		s.authLimiter.ResetScoped(r.Context(), clientIP, ratelimit.AuthScopeInternalSecret)
		recordAuthDecision(r.Context(), "profiling_secret", "success")
		ctx := context.WithValue(r.Context(), ctxInternalCallerKey, true)
		s.serveWithSentryScope(next, w, r.WithContext(ctx))
	})
}

func (s *Server) profilingCIDRAllowlist(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(s.profilingAllowedCIDRs) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		clientIP := realIP(r, s.trustedProxies)
		if !ipInNets(clientIP, s.profilingAllowedCIDRs) {
			respondError(w, r, http.StatusForbidden, "profiling access denied")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) pprofAccessLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		slog.Info("pprof access",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_ip", realIP(r, s.trustedProxies),
			"request_id", chimw.GetReqID(r.Context()),
		)
	})
}
