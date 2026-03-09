package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
)

func TestRequirePermission_AdminAllowsAll(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	ms.getUserPermissionsFn = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{"*"}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user_1")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj_1")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequirePermission_ViewerBlocksWrite(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	ms.getUserPermissionsFn = func(_ context.Context, _, _ string) ([]string, error) {
		return domain.SystemRolePermissions["viewer"], nil
	}
	srv := newTestServer(t, ms, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user_1")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj_1")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_OperatorCanTrigger(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	ms.getUserPermissionsFn = func(_ context.Context, _, _ string) ([]string, error) {
		return domain.SystemRolePermissions["operator"], nil
	}
	srv := newTestServer(t, ms, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsTrigger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user_1")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj_1")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequirePermission_APIKeyUsesScopes(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	srv := newTestServer(t, ms, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsRead})
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequirePermission_UnknownUserDenied(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	// GetUserPermissions returns nil (no role assigned)
	ms.getUserPermissionsFn = func(_ context.Context, _, _ string) ([]string, error) {
		return nil, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user_unknown")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj_1")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_InternalSecretAllowed(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	srv := newTestServer(t, ms, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No actor type = internal auth
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (internal secret should pass)", w.Code, http.StatusOK)
	}
}
