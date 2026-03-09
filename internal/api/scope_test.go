package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
)

func TestRequirePermission_APIKey_AllowsWildcard(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &mockAPIStore{}, nil, nil)
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

	srv := newTestServer(t, &mockAPIStore{}, nil, nil)
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

	srv := newTestServer(t, &mockAPIStore{}, nil, nil)
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

	srv := newTestServer(t, &mockAPIStore{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	// Empty scopes = backwards compatible full access
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

	srv := newTestServer(t, &mockAPIStore{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	// nil scopes + api_key type — allow (backwards compat)
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

	srv := newTestServer(t, &mockAPIStore{}, nil, nil)
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

func TestRequirePermission_InternalSecret_AllowsAll(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &mockAPIStore{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No actor type = internal secret auth
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (internal secret should pass)", w.Code, http.StatusOK)
	}
}

func TestRequirePermission_UnknownActorType_Rejected(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &mockAPIStore{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxActorTypeKey, "bogus")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (unknown actor type should be rejected)", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_User_WithMatchingPermission(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	ms.getUserPermissionsFn = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeJobsRead, domain.ScopeJobsWrite}, nil
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxActorTypeKey, "user")
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

	ms := &mockAPIStore{}
	ms.getUserPermissionsFn = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeJobsRead}, nil
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_User_NoRole(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	ms.getUserPermissionsFn = func(_ context.Context, _, _ string) ([]string, error) {
		return nil, nil
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (no role = forbidden)", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_User_DBError(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	ms.getUserPermissionsFn = func(_ context.Context, _, _ string) ([]string, error) {
		return nil, context.DeadlineExceeded
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestRequirePermission_User_MissingProjectContext(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &mockAPIStore{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxActorTypeKey, "user")
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

	callCount := 0
	ms := &mockAPIStore{}
	ms.getUserPermissionsFn = func(_ context.Context, _, _ string) ([]string, error) {
		callCount++
		return []string{domain.ScopeJobsRead}, nil
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	makeReq := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := context.WithValue(r.Context(), ctxActorTypeKey, "user")
		ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
		ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
		r = r.WithContext(ctx)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}

	// First call — cache miss, hits DB
	w1 := makeReq()
	if w1.Code != http.StatusOK {
		t.Fatalf("first call status = %d, want %d", w1.Code, http.StatusOK)
	}
	if callCount != 1 {
		t.Fatalf("DB calls = %d, want 1", callCount)
	}

	// Second call — cache hit, no DB call
	w2 := makeReq()
	if w2.Code != http.StatusOK {
		t.Fatalf("second call status = %d, want %d", w2.Code, http.StatusOK)
	}
	if callCount != 1 {
		t.Fatalf("DB calls after cache hit = %d, want 1", callCount)
	}
}
