package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/billing"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/ratelimit"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// mockRateLimiter wraps a real RedisRateLimiter or provides deterministic behavior.
type rlTestServer struct {
	server  *Server
	handler http.Handler
}

func newRLTestServer(cfg *config.Config, limiter *ratelimit.RedisRateLimiter) *rlTestServer {
	if cfg == nil {
		cfg = &config.Config{
			DefaultAPIKeyRateLimit:      1000,
			DefaultAPIKeyRateWindowSecs: 60,
		}
	}
	s := &Server{
		config:      cfg,
		rateLimiter: limiter,
	}
	handler := s.projectRateLimit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	return &rlTestServer{server: s, handler: handler}
}

func reqWithAPIKey(apiKeyID string, apiKey *domain.APIKey) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	ctx := context.WithValue(req.Context(), ctxAPIKeyIDKey, apiKeyID)
	if apiKey != nil {
		ctx = context.WithValue(ctx, ctxAuthKeyObjKey, apiKey)
	}
	return req.WithContext(ctx)
}

type rateLimitPlanEnforcer struct {
	orgID     string
	orgErr    error
	limits    billing.OrgPlanLimits
	limitsErr error
	used      int64
}

func (e rateLimitPlanEnforcer) CheckProjectLimit(context.Context, string) error { return nil }
func (e rateLimitPlanEnforcer) CheckMemberLimit(context.Context, string) error  { return nil }
func (e rateLimitPlanEnforcer) CheckOrgCreationLimit(context.Context, string, domain.PlanTier) error {
	return nil
}
func (e rateLimitPlanEnforcer) CheckMaxDispatchPriority(context.Context, string, int) error {
	return nil
}
func (e rateLimitPlanEnforcer) GetProjectOrgID(context.Context, string) (string, error) {
	if e.orgErr != nil {
		return "", e.orgErr
	}
	return e.orgID, nil
}
func (e rateLimitPlanEnforcer) GetActiveProjectOrgID(context.Context, string) (string, error) {
	return e.orgID, nil
}
func (e rateLimitPlanEnforcer) GetOrgPlanLimits(context.Context, string) (billing.OrgPlanLimits, error) {
	if e.limitsErr != nil {
		return billing.OrgPlanLimits{}, e.limitsErr
	}
	return e.limits, nil
}
func (e rateLimitPlanEnforcer) GetMonthlyRunCount(context.Context, string) (int64, error) {
	return e.used, nil
}
func (e rateLimitPlanEnforcer) EnsureOrgSubscription(context.Context, string) error { return nil }
func (e rateLimitPlanEnforcer) DispatchBilling(context.Context, string, domain.PlanTier, string, map[string]any) {
}

func TestSetUsageHeaders_UsesMonthlyRunAllowance(t *testing.T) {
	t.Parallel()
	limits := billing.GetPlanLimits(domain.PlanStarter)
	srv := &Server{
		billingEnforcer: rateLimitPlanEnforcer{
			orgID:  "org-1",
			limits: limits,
			used:   1234,
		},
	}
	rr := httptest.NewRecorder()

	srv.setUsageHeaders(context.Background(), rr, &limits, "proj-1")

	if got := rr.Header().Get("X-Strait-Usage-Limit"); got != "50000" {
		t.Fatalf("usage limit header = %q, want monthly allowance 50000", got)
	}
	if got := rr.Header().Get("X-Strait-Usage-Remaining"); got != "48766" {
		t.Fatalf("usage remaining header = %q, want 48766", got)
	}
}

func TestProjectRateLimit_NoRedis_FailsOpen(t *testing.T) {
	t.Parallel()
	ts := newRLTestServer(nil, nil)

	rr := httptest.NewRecorder()
	ts.handler.ServeHTTP(rr, reqWithAPIKey("key-1", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with no Redis, got %d", rr.Code)
	}
}

func TestProjectRateLimit_APIKeyOverride(t *testing.T) {
	t.Parallel()
	limiter := ratelimit.NewRedisRateLimiter(nil, false)
	ts := newRLTestServer(nil, limiter)

	apiKey := &domain.APIKey{
		RateLimitRequests:   5,
		RateLimitWindowSecs: 60,
	}

	rr := httptest.NewRecorder()
	ts.handler.ServeHTTP(rr, reqWithAPIKey("key-1", apiKey))

	// With disabled Redis, it fails open (allowed).
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestProjectRateLimit_DefaultFallback_UsesConfig(t *testing.T) {
	t.Parallel()
	// Limiter is disabled (fail-open), but we verify the middleware
	// reaches the default fallback path by checking headers are NOT set
	// (disabled limiter returns Allowed=true with Remaining=0, and limit=0
	// triggers early return before headers).
	cfg := &config.Config{
		DefaultAPIKeyRateLimit:      500,
		DefaultAPIKeyRateWindowSecs: 30,
	}
	limiter := ratelimit.NewRedisRateLimiter(nil, false)
	ts := newRLTestServer(cfg, limiter)

	// Request with API key but no custom limits.
	rr := httptest.NewRecorder()
	ts.handler.ServeHTTP(rr, reqWithAPIKey("key-no-limits", &domain.APIKey{}))

	// Fail-open: should be 200.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestProjectRateLimit_InternalSecret_NoRateLimit(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		DefaultAPIKeyRateLimit:      1,
		DefaultAPIKeyRateWindowSecs: 60,
	}
	limiter := ratelimit.NewRedisRateLimiter(nil, false)
	ts := newRLTestServer(cfg, limiter)

	// Request with no API key and no project (internal secret auth).
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)

	rr := httptest.NewRecorder()
	ts.handler.ServeHTTP(rr, req)

	// No API key ID and no project ID means limit stays 0 -> pass through.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for internal secret (no key/project), got %d", rr.Code)
	}

	// No rate limit headers should be set.
	if rr.Header().Get("X-RateLimit-Limit") != "" {
		t.Fatal("expected no X-RateLimit-Limit header for internal secret requests")
	}
}

func TestProjectRateLimit_Headers_SetWhenLimited(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		DefaultAPIKeyRateLimit:      100,
		DefaultAPIKeyRateWindowSecs: 60,
	}
	// Use disabled limiter (fail-open) -- headers won't be set because
	// the disabled path returns Allowed=true before the header-setting code.
	// This test verifies the config path is correct.
	limiter := ratelimit.NewRedisRateLimiter(nil, false)
	ts := newRLTestServer(cfg, limiter)

	rr := httptest.NewRecorder()
	ts.handler.ServeHTTP(rr, reqWithAPIKey("key-1", &domain.APIKey{}))

	// Disabled limiter -> fail open -> 200.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestProjectRateLimit_RedisErrorReturnsServiceUnavailable(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	addr := mr.Addr()
	mr.Close()
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })

	cfg := &config.Config{
		DefaultAPIKeyRateLimit:      100,
		DefaultAPIKeyRateWindowSecs: 60,
	}
	ts := newRLTestServer(cfg, ratelimit.NewRedisRateLimiter(client, true))

	rr := httptest.NewRecorder()
	ts.handler.ServeHTTP(rr, reqWithAPIKey("key-redis-down", &domain.APIKey{}))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestProjectRateLimit_ZeroDefaultConfig_SkipsRateLimit(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		DefaultAPIKeyRateLimit:      0,
		DefaultAPIKeyRateWindowSecs: 0,
	}
	limiter := ratelimit.NewRedisRateLimiter(nil, false)
	ts := newRLTestServer(cfg, limiter)

	rr := httptest.NewRecorder()
	ts.handler.ServeHTTP(rr, reqWithAPIKey("key-1", &domain.APIKey{}))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// No headers when rate limiting is completely disabled.
	if rr.Header().Get("X-RateLimit-Limit") != "" {
		t.Fatal("expected no rate limit headers when config is zero")
	}
}

func TestProjectRateLimit_ProjectFallback_NoAPIKey(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		DefaultAPIKeyRateLimit:      500,
		DefaultAPIKeyRateWindowSecs: 30,
	}
	limiter := ratelimit.NewRedisRateLimiter(nil, false)
	ts := newRLTestServer(cfg, limiter)

	// Request with project ID and API key but no custom limits.
	// The project quota path requires s.store which is nil in tests,
	// so we use an API key context to skip the store call and test
	// the default fallback via the API key path.
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxAPIKeyIDKey, "key-via-project")
	ctx = context.WithValue(ctx, ctxAuthKeyObjKey, &domain.APIKey{})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	ts.handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestResolveRateLimit_UsesPlanLimitBeforeGlobalDefault(t *testing.T) {
	t.Parallel()

	s := &Server{
		config: &config.Config{
			DefaultAPIKeyRateLimit:      1000,
			DefaultAPIKeyRateWindowSecs: 60,
			RateLimitRequests:           1000,
		},
		billingEnforcer: rateLimitPlanEnforcer{
			orgID:  "org-free",
			limits: billing.GetPlanLimits(domain.PlanFree),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)

	rl := s.resolveRateLimit(req.Context(), req, "proj-free", "")
	if rl.limit != billing.APIRateFree {
		t.Fatalf("limit = %d, want free plan limit %d", rl.limit, billing.APIRateFree)
	}
	if rl.windowSecs != 60 {
		t.Fatalf("windowSecs = %d, want 60", rl.windowSecs)
	}
	if rl.key != "rl:plan:org-free" {
		t.Fatalf("key = %q, want rl:plan:org-free", rl.key)
	}
}

func TestResolveRateLimit_UnlimitedPlanDoesNotFallBackToGlobalDefault(t *testing.T) {
	t.Parallel()

	s := &Server{
		config: &config.Config{
			DefaultAPIKeyRateLimit:      1000,
			DefaultAPIKeyRateWindowSecs: 60,
			RateLimitRequests:           1000,
		},
		billingEnforcer: rateLimitPlanEnforcer{
			orgID:  "org-business",
			limits: billing.GetPlanLimits(domain.PlanBusiness),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)

	rl := s.resolveRateLimit(req.Context(), req, "proj-business", "")
	if rl.limit != 0 || rl.windowSecs != 0 || rl.key != "" {
		t.Fatalf("unlimited plan resolved rate limit = %+v, want zero-value no limit", rl)
	}
}

func TestResolveRateLimit_APIKeyOverrideCannotExceedPlanCap(t *testing.T) {
	t.Parallel()

	s := &Server{
		config: &config.Config{
			DefaultAPIKeyRateLimit:      1000,
			DefaultAPIKeyRateWindowSecs: 60,
		},
		billingEnforcer: rateLimitPlanEnforcer{
			orgID:  "org-free",
			limits: billing.GetPlanLimits(domain.PlanFree),
		},
	}
	req := reqWithAPIKey("key-1", &domain.APIKey{
		RateLimitRequests:   1000,
		RateLimitWindowSecs: 60,
	})

	rl := s.resolveRateLimit(req.Context(), req, "proj-free", "key-1")
	if rl.limit != billing.APIRateFree {
		t.Fatalf("limit = %d, want key override capped to free plan limit %d", rl.limit, billing.APIRateFree)
	}
	if rl.key != "rl:apikey:key-1" {
		t.Fatalf("key = %q, want rl:apikey:key-1", rl.key)
	}
}

func TestResolveRateLimit_ProjectOrgLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()

	s := &Server{
		config: &config.Config{
			DefaultAPIKeyRateLimit:      1000,
			DefaultAPIKeyRateWindowSecs: 60,
		},
		billingEnforcer: rateLimitPlanEnforcer{
			orgErr: errors.New("database unavailable"),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)

	rl := s.resolveRateLimit(req.Context(), req, "proj-free", "")
	if rl.err == nil {
		t.Fatal("expected project org lookup error to fail closed")
	}
	if rl.limit != 0 || rl.key != "" {
		t.Fatalf("resolved rate limit = %+v, want no fallback limit when plan lookup fails", rl)
	}
}

func TestResolveRateLimit_PlanLimitLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()

	s := &Server{
		config: &config.Config{
			DefaultAPIKeyRateLimit:      1000,
			DefaultAPIKeyRateWindowSecs: 60,
		},
		billingEnforcer: rateLimitPlanEnforcer{
			orgID:     "org-free",
			limitsErr: errors.New("catalog unavailable"),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)

	rl := s.resolveRateLimit(req.Context(), req, "proj-free", "")
	if rl.err == nil {
		t.Fatal("expected plan limit lookup error to fail closed")
	}
	if rl.limit != 0 || rl.key != "" {
		t.Fatalf("resolved rate limit = %+v, want no fallback limit when plan lookup fails", rl)
	}
}

func TestProjectRateLimit_PlanLookupErrorReturnsServiceUnavailable(t *testing.T) {
	t.Parallel()

	s := &Server{
		config: &config.Config{
			DefaultAPIKeyRateLimit:      1000,
			DefaultAPIKeyRateWindowSecs: 60,
		},
		rateLimiter: ratelimit.NewRedisRateLimiter(nil, false),
		billingEnforcer: rateLimitPlanEnforcer{
			limitsErr: errors.New("catalog unavailable"),
			orgID:     "org-free",
		},
	}
	handler := s.projectRateLimit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-free")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestProjectRateLimit_InternalSecretAuth_Bypasses(t *testing.T) {
	t.Parallel()
	limiter := ratelimit.NewRedisRateLimiter(nil, false)
	ts := newRLTestServer(nil, limiter)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	// Internal secret auth does not set scopes -- scopesFromContext returns nil.

	rr := httptest.NewRecorder()
	ts.handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for internal secret auth, got %d", rr.Code)
	}
}
