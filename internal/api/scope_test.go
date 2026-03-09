package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
)

func TestRequireScope_AllowsWildcard(t *testing.T) {
	t.Parallel()

	handler := requireScope(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{"*"})
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequireScope_AllowsMatchingScope(t *testing.T) {
	t.Parallel()

	handler := requireScope(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{domain.ScopeJobsRead})
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequireScope_BlocksMissingScope(t *testing.T) {
	t.Parallel()

	handler := requireScope(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{domain.ScopeJobsRead})
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireScope_EmptyScopesAllowAll(t *testing.T) {
	t.Parallel()

	handler := requireScope(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	// Empty scopes = backwards compatible full access
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{})
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (empty scopes should allow all)", w.Code, http.StatusOK)
	}
}

func TestRequireScope_NilScopesAllowAll(t *testing.T) {
	t.Parallel()

	handler := requireScope(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No scopes in context at all (internal secret auth)
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (nil scopes = internal auth)", w.Code, http.StatusOK)
	}
}

func TestRequireScope_MultipleScopesOnKey(t *testing.T) {
	t.Parallel()

	handler := requireScope(domain.ScopeRunsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{domain.ScopeJobsRead, domain.ScopeRunsRead})
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
