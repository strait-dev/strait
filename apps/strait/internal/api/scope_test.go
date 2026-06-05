package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

// API Key tests.

func TestRequirePermission_APIKey_AllowsWildcard(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequirePermission_APIKey_AllowsMatchingScope(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequirePermission_APIKey_BlocksMissingScope(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_APIKey_EmptyScopesAllowAll(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (empty scopes should allow all)", w.Code, http.StatusOK)
	}
}

func TestRequirePermission_APIKey_NilScopesAllowAll(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	// nil scopes = internal secret auth shortcut
	ctx := context.WithValue(r.Context(), ctxActorTypeKey, "api_key")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (nil scopes = full access)", w.Code, http.StatusOK)
	}
}

func TestRequirePermission_APIKey_MultipleScopesWithMatch(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeRunsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{domain.ScopeJobsRead, domain.ScopeRunsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// Internal secret tests.

func TestRequirePermission_InternalSecret_AllowsAll(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxInternalCallerKey, true))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (internal secret should pass)", w.Code, http.StatusOK)
	}
}

func TestRequirePermission_InternalSecret_WithActorHeaders(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Internal secret with actor headers (for audit) — should still pass
	// because scopes are nil (shortcut fires before actor type check).
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	ctx = context.WithValue(ctx, ctxInternalCallerKey, true)
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (internal secret + actor headers = allowed)", w.Code, http.StatusOK)
	}
}

// Unknown actor type.

func TestRequirePermission_UnknownActorType_Rejected(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	// Must set scopes so the nil-scopes shortcut doesn't fire.
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "bogus")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (unknown actor type should be rejected)", w.Code, http.StatusForbidden)
	}
}

// User permission tests.

func userCtx(r *http.Request, projectID, userID string) *http.Request {
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{}) // non-nil, empty = DB permission path
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxProjectIDKey, projectID)
	ctx = context.WithValue(ctx, ctxActorIDKey, userID)
	return r.WithContext(ctx)
}

func TestRequirePermission_User_WithMatchingPermission(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeJobsRead, domain.ScopeJobsWrite}, nil
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := userCtx(httptest.NewRequest(http.MethodGet, "/", nil), "proj-1", "user-1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequirePermission_User_OIDCScopesDoNotBypassProjectRBAC(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeJobsRead}, nil
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{domain.ScopeJobsWrite})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_User_OIDCScopesAndProjectRBACBothRequired(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeJobsWrite}, nil
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{domain.ScopeJobsWrite})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequirePermission_User_MissingPermission(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeJobsRead}, nil
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := userCtx(httptest.NewRequest(http.MethodGet, "/", nil), "proj-1", "user-1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_User_MissingResourcePolicyDoesNotGrantAccess(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeJobsRead}, nil
	}
	ms.GetResourcePoliciesFunc = func(_ context.Context, projectID, resourceType, resourceID, userID string) ([]string, error) {
		if projectID != "proj-1" || resourceType != "job" || resourceID != "job-1" || userID != "user-1" {
			t.Fatalf("unexpected resource policy lookup: project=%s type=%s id=%s user=%s", projectID, resourceType, resourceID, userID)
		}
		return nil, nil
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := userCtx(httptest.NewRequest(http.MethodPatch, "/v1/jobs/job-1", nil), "proj-1", "user-1")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", "job-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	if len(ms.GetResourcePoliciesCalls()) != 1 {
		t.Fatalf("resource policy lookups = %d, want 1", len(ms.GetResourcePoliciesCalls()))
	}
}

func TestRequirePermission_User_ExplicitResourcePolicyGrantsAccess(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeJobsRead}, nil
	}
	ms.GetResourcePoliciesFunc = func(context.Context, string, string, string, string) ([]string, error) {
		return []string{domain.ScopeJobsWrite}, nil
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := userCtx(httptest.NewRequest(http.MethodPatch, "/v1/jobs/job-1", nil), "proj-1", "user-1")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", "job-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequirePermission_User_ResourcePolicyIgnoredBelowAdvancedRBAC(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeJobsRead}, nil
	}
	ms.GetResourcePoliciesFunc = func(context.Context, string, string, string, string) ([]string, error) {
		t.Fatal("resource policy lookup must not run below Advanced RBAC")
		return []string{domain.ScopeJobsWrite}, nil
	}
	enforcer := &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanPro)}
	srv := newServerWithEnforcer(t, ms, nil, enforcer)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := userCtx(httptest.NewRequest(http.MethodPatch, "/v1/jobs/job-1", nil), "proj-1", "user-1")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", "job-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_User_NoRole(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return nil, nil
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := userCtx(httptest.NewRequest(http.MethodGet, "/", nil), "proj-1", "user-1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (no role = forbidden)", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_User_DBError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return nil, context.DeadlineExceeded
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := userCtx(httptest.NewRequest(http.MethodGet, "/", nil), "proj-1", "user-1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestRequirePermission_User_MissingProjectContext(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{}) // non-nil, empty = DB path
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	// No project ID
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (missing project context)", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_User_CacheHit(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		callCount.Add(1)
		return []string{domain.ScopeJobsRead}, nil
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	makeReq := func() *httptest.ResponseRecorder {
		r := userCtx(httptest.NewRequest(http.MethodGet, "/", nil), "proj-1", "user-1")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}

	// First call — cache miss, hits DB
	w1 := makeReq()
	if w1.Code != http.StatusOK {
		t.Fatalf("first call status = %d, want %d", w1.Code, http.StatusOK)
	}
	if c := callCount.Load(); c != 1 {
		t.Fatalf("DB calls = %d, want 1", c)
	}

	// Second call — cache hit, no DB call
	w2 := makeReq()
	if w2.Code != http.StatusOK {
		t.Fatalf("second call status = %d, want %d", w2.Code, http.StatusOK)
	}
	if c := callCount.Load(); c != 1 {
		t.Fatalf("DB calls after cache hit = %d, want 1", c)
	}
}

func TestRequirePermission_User_MissingActorID(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	// Deliberately NO actorID.
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (missing actor ID)", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_User_WildcardPermission(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{"*"}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	// User with wildcard should access ANY scope.
	for _, scope := range []string{domain.ScopeJobsWrite, domain.ScopeRBACManage, domain.ScopeSecretsWrite, "anything"} {
		handler := srv.requirePermission(scope)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		r := userCtx(httptest.NewRequest(http.MethodGet, "/", nil), "proj-1", "user-1")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("scope %q: status = %d, want %d", scope, w.Code, http.StatusOK)
		}
	}
}

func TestRequirePermission_User_CacheInvalidationReloads(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		callCount.Add(1)
		return []string{domain.ScopeJobsRead}, nil
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	makeReq := func() {
		r := userCtx(httptest.NewRequest(http.MethodGet, "/", nil), "proj-1", "user-1")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
	}

	// First call populates cache.
	makeReq()
	if c := callCount.Load(); c != 1 {
		t.Fatalf("DB calls = %d, want 1", c)
	}

	// Invalidate cache.
	srv.permCache.Invalidate("proj-1", "user-1")

	// Next call should hit DB again.
	makeReq()
	if c := callCount.Load(); c != 2 {
		t.Fatalf("DB calls after invalidation = %d, want 2", c)
	}
}

func TestRequirePermission_ChainedMiddleware(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeJobsRead}, nil // Has read but NOT write.
	}
	srv := newTestServer(t, ms, nil, nil)

	// Chain: first requires read (pass), second requires write (fail).
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	chained := srv.requirePermission(domain.ScopeJobsRead)(
		srv.requirePermission(domain.ScopeJobsWrite)(inner),
	)

	r := userCtx(httptest.NewRequest(http.MethodGet, "/", nil), "proj-1", "user-1")
	w := httptest.NewRecorder()
	chained.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("chained status = %d, want %d (second middleware should block)", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_User_TokenScopesEnforced(t *testing.T) {
	t.Parallel()

	// When a user has explicit token scopes (from OAuth consent), those
	// scopes are enforced directly — the DB permission lookup is skipped.
	// This is the principle of least privilege: the token restricts what
	// the user can do, even if their database role would allow more.
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{domain.ScopeJobsRead}) // only read
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	// Token only has jobs:read, so jobs:write should be denied.
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (token scopes should restrict permissions)", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_User_EmptyTokenScopesDenyEvenWithProjectRBAC(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeJobsRead}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), ctxScopesKey, []string{})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	ctx = context.WithValue(ctx, ctxOIDCScopeClaimPresentKey, true)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	if len(ms.GetUserPermissionsCalls()) != 0 {
		t.Fatal("explicit empty OIDC scopes must deny before project RBAC lookup")
	}
}

func TestRequirePermission_User_TokenScopesAllow(t *testing.T) {
	t.Parallel()

	// When the token scope and the project role both include the required
	// permission, allow through.
	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeJobsRead}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{domain.ScopeJobsRead, domain.ScopeRunsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (token has required scope)", w.Code, http.StatusOK)
	}
}
