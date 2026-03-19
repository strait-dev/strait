package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/billing"
	"strait/internal/config"
	"strait/internal/domain"
)

type mockBillingEnforcer struct {
	projectOrgMap map[string]string
}

func (m *mockBillingEnforcer) CheckProjectLimit(_ context.Context, _ string) error {
	return nil
}

func (m *mockBillingEnforcer) GetProjectOrgID(_ context.Context, projectID string) (string, error) {
	if m.projectOrgMap != nil {
		return m.projectOrgMap[projectID], nil
	}
	return "", nil
}

type mockUsageService struct{}

func (m *mockUsageService) GetCurrentUsage(_ context.Context, orgID string, projectCount, memberCount int) (*billing.CurrentUsageResponse, error) {
	return &billing.CurrentUsageResponse{
		OrgID: orgID,
		Plan:  "starter",
	}, nil
}

func newUsageTestServer(t *testing.T, enforcer BillingEnforcer, usageSvc UsageService) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       "01234567890123456789012345678901",
	}
	ms := &mockAPIStore{
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]domain.Project, error) {
			return nil, nil
		},
	}
	srv := NewServer(ServerDeps{
		Config:          cfg,
		Store:           ms,
		Queue:           &mockQueue{},
		BillingEnforcer: enforcer,
		UsageService:    usageSvc,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestUsageEndpoint_APIKey_CrossTenantForbidden(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{
			"proj-1": "org-A",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})

	// Simulate API key auth: set scopes and project in context.
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/current?org_id=org-B", nil)
	req.Header.Set("X-Internal-Secret", "test-secret")
	// Set API key context values to simulate API key auth.
	ctx := context.WithValue(req.Context(), ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-tenant access, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUsageEndpoint_APIKey_SameTenantAllowed(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{
			"proj-1": "org-A",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})

	req := httptest.NewRequest(http.MethodGet, "/v1/usage/current?org_id=org-A", nil)
	req.Header.Set("X-Internal-Secret", "test-secret")
	ctx := context.WithValue(req.Context(), ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for same-tenant access, got %d: %s", w.Code, w.Body.String())
	}

	var resp billing.CurrentUsageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if resp.OrgID != "org-A" {
		t.Fatalf("expected org_id=org-A, got %q", resp.OrgID)
	}
}

func TestUsageEndpoint_InternalSecret_AllowsAnyOrg(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})

	// Internal secret auth: no scopes in context (scopesFromContext returns nil).
	req := authedRequest(http.MethodGet, "/v1/usage/current?org_id=any-org", "")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for internal secret with any org_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUsageEndpoint_NotConfigured(t *testing.T) {
	t.Parallel()

	srv := newUsageTestServer(t, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/usage/current?org_id=org-1", "")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 when usage service is nil, got %d", w.Code)
	}
}

func TestUsageEndpoint_MissingOrgID(t *testing.T) {
	t.Parallel()

	srv := newUsageTestServer(t, &mockBillingEnforcer{}, &mockUsageService{})

	req := authedRequest(http.MethodGet, "/v1/usage/current", "")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when org_id is missing, got %d", w.Code)
	}
}
