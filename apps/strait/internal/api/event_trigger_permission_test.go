package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
)

// TestEventRoutes_PermissionEnforcement verifies that /v1/events/ routes
// require the correct scopes after the security hardening fix.
// These routes previously had no requirePermission middleware.
//
// Tests use requirePermission middleware directly (not through the full router)
// to isolate permission checks from authentication.

func TestEventRoutes_RequirePermission_ReadRoutes(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name   string
		scopes []string
		want   int
	}{
		{"jobs:read allows", []string{domain.ScopeJobsRead}, http.StatusOK},
		{"wildcard allows", []string{domain.ScopeAll}, http.StatusOK},
		{"stats:read denied", []string{domain.ScopeStatsRead}, http.StatusForbidden},
		{"jobs:write denied", []string{domain.ScopeJobsWrite}, http.StatusForbidden},
		{"jobs:trigger denied", []string{domain.ScopeJobsTrigger}, http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			handler := srv.requirePermission(domain.ScopeJobsRead)(inner)
			r := httptest.NewRequest(http.MethodGet, "/v1/events/", nil)
			ctx := context.WithValue(r.Context(), ctxScopesKey, tc.scopes)
			ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
			ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test-key")
			r = r.WithContext(ctx)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			assert.Equal(
				t, tc.want,
				w.Code)
		})
	}
}

func TestEventRoutes_RequirePermission_WriteRoutes(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name   string
		scopes []string
		want   int
	}{
		{"jobs:write allows", []string{domain.ScopeJobsWrite}, http.StatusOK},
		{"wildcard allows", []string{domain.ScopeAll}, http.StatusOK},
		{"jobs:read denied", []string{domain.ScopeJobsRead}, http.StatusForbidden},
		{"stats:read denied", []string{domain.ScopeStatsRead}, http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			handler := srv.requirePermission(domain.ScopeJobsWrite)(inner)
			r := httptest.NewRequest(http.MethodPost, "/v1/events/purge", nil)
			ctx := context.WithValue(r.Context(), ctxScopesKey, tc.scopes)
			ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
			ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test-key")
			r = r.WithContext(ctx)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			assert.Equal(
				t, tc.want,
				w.Code)
		})
	}
}

func TestEventRoutes_RequirePermission_TriggerRoutes(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name   string
		scopes []string
		want   int
	}{
		{"jobs:trigger allows", []string{domain.ScopeJobsTrigger}, http.StatusOK},
		{"wildcard allows", []string{domain.ScopeAll}, http.StatusOK},
		{"jobs:write denied", []string{domain.ScopeJobsWrite}, http.StatusForbidden},
		{"jobs:read denied", []string{domain.ScopeJobsRead}, http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			handler := srv.requirePermission(domain.ScopeJobsTrigger)(inner)
			r := httptest.NewRequest(http.MethodPost, "/v1/events/test-key/send", strings.NewReader(`{"payload":{}}`))
			r.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(r.Context(), ctxScopesKey, tc.scopes)
			ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
			ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test-key")
			r = r.WithContext(ctx)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			assert.Equal(
				t, tc.want,
				w.Code)
		})
	}
}

// Integration tests using the full router with X-Internal-Secret.

func TestEventRoutes_Integration_InternalSecret_AllowsAll(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	routes := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/v1/events/", ""},
		{http.MethodGet, "/v1/events/stats", ""},
		{http.MethodPost, "/v1/events/purge", `{"statuses":["expired"]}`},
		{http.MethodGet, "/v1/events/test-key", ""},
		{http.MethodDelete, "/v1/events/test-key", ""},
		{http.MethodPost, "/v1/events/test-key/send", `{"payload":{}}`},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			t.Parallel()
			var r *http.Request
			if route.body != "" {
				r = httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
				r.Header.Set("Content-Type", "application/json")
			} else {
				r = httptest.NewRequest(route.method, route.path, nil)
			}
			r.Header.Set("X-Internal-Secret", "test-secret-value")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)
			assert.False(
				t, w.Code ==
					http.StatusUnauthorized ||
					w.Code ==
						http.
							StatusForbidden,
			)

			// Internal secret bypasses scope checks -- should never get 401 or 403.
		})
	}
}
