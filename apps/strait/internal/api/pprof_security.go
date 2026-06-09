package api

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"strait/internal/config"
	"strait/internal/debug"
	"strait/internal/domain"
	"strait/internal/ratelimit"
	"strait/internal/telemetry"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const pprofAuthKind = "profiling"

// maxProfilingRequestsPerMinute caps authenticated profiling requests per client
// IP so a secret-holder cannot sustain unbounded back-to-back profiles.
const maxProfilingRequestsPerMinute = 30

type ProfilingHandlerDeps struct {
	Config      *config.Config
	RedisClient *redis.Client
	Metrics     *telemetry.Metrics
	Edition     domain.Edition
	Version     string
}

func NewProfilingHandler(deps ProfilingHandlerDeps) http.Handler {
	cfg := deps.Config
	if cfg == nil {
		cfg = &config.Config{}
	}
	if !cfg.ProfilingEnabled {
		return chi.NewRouter()
	}

	s := &Server{
		config:      cfg,
		metrics:     deps.Metrics,
		authLimiter: ratelimit.NewAuthLimiter(deps.RedisClient, deps.RedisClient != nil),
		edition:     deps.Edition,
		version:     deps.Version,
		startedAt:   time.Now(),
	}
	s.trustedProxies = parseTrustedProxies(cfg.TrustedProxies)
	s.profilingAllowedCIDRs = parseTrustedProxies(cfg.ProfilingAllowedCIDRs)
	return s.profilingRouter()
}

func (s *Server) profilingRouter() chi.Router {
	r := chi.NewRouter()
	s.mountProfilingRoutes(r)
	return r
}

func (s *Server) mountProfilingRoutes(r chi.Router) {
	r.Use(s.pprofAccessRecorder)
	r.Use(s.profilingAuth)
	r.Use(s.profilingCIDRAllowlist)
	r.Use(s.pprofProfileStartLogger)
	debug.MountPprofRoutes(r)
}

func (s *Server) profilingAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := clientIPFromRequest(r, s.trustedProxies)
		if blocked, retryAfter := s.authLimiter.IsBlockedScoped(r.Context(), clientIP, ratelimit.AuthScopeProfiling); blocked {
			recordAuthDecision(r.Context(), pprofAuthKind, "throttled")
			recordAuthRateLimitThrottled(r.Context(), pprofAuthKind)
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			respondError(w, r, http.StatusTooManyRequests, ratelimit.BlockedError(retryAfter))
			return
		}

		expectedSecret := s.config.ProfilingSecret
		if expectedSecret == "" {
			expectedSecret = s.config.InternalSecret
		}

		secret := internalSecretFromRequest(r)
		if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(expectedSecret)) != 1 {
			s.authLimiter.RecordFailureScoped(r.Context(), clientIP, ratelimit.AuthScopeProfiling)
			recordAuthDecision(r.Context(), pprofAuthKind, "failure")
			respondError(w, r, http.StatusUnauthorized, "invalid or missing profiling secret")
			return
		}

		s.authLimiter.ResetScoped(r.Context(), clientIP, ratelimit.AuthScopeProfiling)
		recordAuthDecision(r.Context(), pprofAuthKind, "success")

		// Throttle authenticated profiling too. Auth-failure lockout does not
		// bound a secret-holder who issues unbounded back-to-back profiles; the
		// CPU profiler is a singleton and each profile imposes continuous
		// overhead, so cap the per-IP request rate as well.
		if s.rateLimiter != nil {
			res, rlErr := s.rateLimiter.AllowStrict(r.Context(), "profiling:"+clientIP, maxProfilingRequestsPerMinute, time.Minute)
			if rlErr != nil {
				slog.Warn("profiling rate limit check failed; allowing", "error", rlErr)
			} else if !res.Allowed {
				recordAuthDecision(r.Context(), pprofAuthKind, "throttled")
				recordAuthRateLimitThrottled(r.Context(), pprofAuthKind)
				w.Header().Set("Retry-After", "60")
				respondError(w, r, http.StatusTooManyRequests, "profiling request rate limit exceeded")
				return
			}
		}

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

		clientIP := clientIPFromRequest(r, s.trustedProxies)
		if !ipInNets(clientIP, s.profilingAllowedCIDRs) {
			respondError(w, r, http.StatusForbidden, "profiling access denied")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) pprofAccessRecorder(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		endpoint := pprofEndpointName(r.URL.Path)
		if endpoint != "unknown" && s.metrics != nil && s.metrics.PprofRequests != nil {
			s.metrics.PprofRequests.Add(r.Context(), 1, metric.WithAttributes(
				attribute.String("endpoint", endpoint),
				attribute.String("status", strconv.Itoa(status)),
			))
		}

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"endpoint", endpoint,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_ip", clientIPFromRequest(r, s.trustedProxies),
			"request_id", chimw.GetReqID(r.Context()),
		}
		if status >= http.StatusBadRequest {
			slog.Warn("pprof access denied", attrs...)
			return
		}
		if endpoint == "profile" {
			slog.Info("pprof profile completed", attrs...)
		}
	})
}

func (s *Server) pprofProfileStartLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pprofEndpointName(r.URL.Path) == "profile" {
			slog.Info("pprof profile started",
				"method", r.Method,
				"path", r.URL.Path,
				"remote_ip", clientIPFromRequest(r, s.trustedProxies),
				"request_id", chimw.GetReqID(r.Context()),
			)
		}
		next.ServeHTTP(w, r)
	})
}

func pprofEndpointName(path string) string {
	switch path {
	case "/debug/pprof/profile":
		return "profile"
	case "/debug/pprof/allocs":
		return "allocs"
	case "/debug/pprof/goroutine":
		return "goroutine"
	case "/debug/pprof/mutex":
		return "mutex"
	case "/debug/pprof/block":
		return "block"
	default:
		return "unknown"
	}
}

func profilingAPIEnabled(cfg *config.Config) bool {
	if cfg == nil || !cfg.ProfilingEnabled {
		return false
	}
	if cfg.ProfilingManagementEnabled && !cfg.ProfilingAPIEnabled {
		return false
	}
	return cfg.ProfilingAPIEnabled || !cfg.ProfilingManagementEnabled
}
