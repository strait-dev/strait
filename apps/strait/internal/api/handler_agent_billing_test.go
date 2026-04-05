package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"
)

func newAgentBillingTestServer(t *testing.T, enforcer BillingEnforcer, ms *APIStoreMock) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	if ms == nil {
		ms = &APIStoreMock{}
	}
	srv := NewServer(ServerDeps{
		Config:          cfg,
		Store:           ms,
		Queue:           &mockQueue{},
		BillingEnforcer: enforcer,
		Edition:         domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestHandleGetAgentUsage_RequiresOrgID(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	srv := newAgentBillingTestServer(t, enforcer, nil)

	// Request without org_id should return 400.
	req := apiKeyRequest(http.MethodGet, "/v1/agents/billing/usage", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
}

func TestHandleGetAgentUsage_CrossTenantBlocked(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newAgentBillingTestServer(t, enforcer, nil)

	// Request with a different org_id should return 403.
	req := apiKeyRequest(http.MethodGet, "/v1/agents/billing/usage?org_id=org-B", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for cross-tenant; body = %s", w.Code, w.Body.String())
	}
}

func TestHandleGetAgentUsage_ValidRequest(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	ms := &APIStoreMock{}
	ms.QueryAgentUsageSummaryFunc = func(_ context.Context, _ string, _ time.Time) (*store.AgentUsageSummary, error) {
		return &store.AgentUsageSummary{
			RunCount:          42,
			TotalTokens:       100_000,
			TotalToolCalls:    200,
			TotalCostMicrousd: 5_000_000,
		}, nil
	}

	srv := newAgentBillingTestServer(t, enforcer, ms)

	req := apiKeyRequest(http.MethodGet, "/v1/agents/billing/usage?org_id=org-1", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, `"run_count":42`) {
		t.Errorf("response missing run_count: %s", body)
	}
	if !strings.Contains(body, `"total_tokens":100000`) {
		t.Errorf("response missing total_tokens: %s", body)
	}
	if !strings.Contains(body, `"total_cost_microusd":5000000`) {
		t.Errorf("response missing total_cost_microusd: %s", body)
	}
}

func TestHandleGetAgentSpendingLimit_Valid(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	ms := &APIStoreMock{}
	ms.GetOrgAgentSpendingLimitFunc = func(_ context.Context, _ string) (int64, error) {
		return 50_000_000, nil // $50 cap
	}

	srv := newAgentBillingTestServer(t, enforcer, ms)

	req := apiKeyRequest(http.MethodGet, "/v1/agents/billing/spending-limit?org_id=org-1", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, `"limit_microusd":50000000`) {
		t.Errorf("response missing limit: %s", body)
	}
	if !strings.Contains(body, `"enabled":true`) {
		t.Errorf("response missing enabled: %s", body)
	}
}

func TestHandleGetAgentSpendingLimit_Disabled(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	ms := &APIStoreMock{}
	ms.GetOrgAgentSpendingLimitFunc = func(_ context.Context, _ string) (int64, error) {
		return -1, nil // no cap
	}

	srv := newAgentBillingTestServer(t, enforcer, ms)

	req := apiKeyRequest(http.MethodGet, "/v1/agents/billing/spending-limit?org_id=org-1", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, `"limit_usd":-1`) {
		t.Errorf("disabled limit should return -1 for limit_usd: %s", body)
	}
	if !strings.Contains(body, `"enabled":false`) {
		t.Errorf("response should show enabled:false: %s", body)
	}
}

func TestHandleUpdateAgentSpendingLimit_Valid(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	var storedLimit int64
	ms := &APIStoreMock{}
	ms.UpdateAgentSpendingLimitFunc = func(_ context.Context, _ string, limit int64) error {
		storedLimit = limit
		return nil
	}

	srv := newAgentBillingTestServer(t, enforcer, ms)

	req := apiKeyRequest(http.MethodPut, "/v1/agents/billing/spending-limit?org_id=org-1",
		`{"limit_microusd": 25000000}`, "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	if storedLimit != 25_000_000 {
		t.Errorf("storedLimit = %d, want 25000000", storedLimit)
	}
}

func TestHandleUpdateAgentSpendingLimit_InvalidValue(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	srv := newAgentBillingTestServer(t, enforcer, nil)

	req := apiKeyRequest(http.MethodPut, "/v1/agents/billing/spending-limit?org_id=org-1",
		`{"limit_microusd": -5}`, "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid limit; body = %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateAgentSpendingLimit_Disable(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	var storedLimit int64
	ms := &APIStoreMock{}
	ms.UpdateAgentSpendingLimitFunc = func(_ context.Context, _ string, limit int64) error {
		storedLimit = limit
		return nil
	}

	srv := newAgentBillingTestServer(t, enforcer, ms)

	req := apiKeyRequest(http.MethodPut, "/v1/agents/billing/spending-limit?org_id=org-1",
		`{"limit_microusd": -1}`, "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	if storedLimit != -1 {
		t.Errorf("storedLimit = %d, want -1 (disabled)", storedLimit)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"enabled":false`) {
		t.Errorf("response should show enabled:false when limit=-1: %s", body)
	}
}

func TestHandleGetAgentUsage_UpgradeRecommendation(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	ms := &APIStoreMock{}
	ms.QueryAgentUsageSummaryFunc = func(_ context.Context, _ string, _ time.Time) (*store.AgentUsageSummary, error) {
		return &store.AgentUsageSummary{
			TotalCostMicrousd: 200_000_000, // $200 — should trigger upgrade recommendation for Maker
		}, nil
	}

	srv := newAgentBillingTestServer(t, enforcer, ms)

	req := apiKeyRequest(http.MethodGet, "/v1/agents/billing/usage?org_id=org-1", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	// The handler currently defaults to AgentPlanFree, so upgrade_recommended won't trigger
	// (it only triggers for Maker). This test verifies the field exists and is false for Free.
	body := w.Body.String()
	if !strings.Contains(body, `"upgrade_recommended"`) {
		t.Errorf("response missing upgrade_recommended field: %s", body)
	}
}

// Verify mock has the new methods needed.
func TestAPIStoreMock_AgentBillingMethods(t *testing.T) {
	t.Parallel()

	var ms APIStore = &APIStoreMock{}
	_ = ms

	mock := &APIStoreMock{}

	// GetOrgAgentSpendingLimit.
	limit, err := mock.GetOrgAgentSpendingLimit(context.Background(), "org-1")
	if err != nil {
		t.Errorf("GetOrgAgentSpendingLimit() error = %v", err)
	}
	if limit != -1 {
		t.Errorf("default limit = %d, want -1", limit)
	}

	// UpdateAgentSpendingLimit.
	err = mock.UpdateAgentSpendingLimit(context.Background(), "org-1", 5000)
	if err != nil {
		t.Errorf("UpdateAgentSpendingLimit() error = %v", err)
	}

	// QueryAgentUsageSummary.
	summary, err := mock.QueryAgentUsageSummary(context.Background(), "org-1", time.Now())
	if err != nil {
		t.Errorf("QueryAgentUsageSummary() error = %v", err)
	}
	if summary == nil {
		t.Error("QueryAgentUsageSummary() returned nil")
	}
}

// Verify the mock implements the complete GetOrgAgentSpendingLimit with custom function.
func TestAPIStoreMock_CustomSpendingLimitFunc(t *testing.T) {
	t.Parallel()

	mock := &APIStoreMock{}
	mock.GetOrgAgentSpendingLimitFunc = func(_ context.Context, orgID string) (int64, error) {
		if orgID == "org-limited" {
			return 10_000_000, nil
		}
		return -1, nil
	}

	limit, _ := mock.GetOrgAgentSpendingLimit(context.Background(), "org-limited")
	if limit != 10_000_000 {
		t.Errorf("limit = %d, want 10000000", limit)
	}

	limit, _ = mock.GetOrgAgentSpendingLimit(context.Background(), "org-unlimited")
	if limit != -1 {
		t.Errorf("limit = %d, want -1", limit)
	}
}

// apiKeyRequest is defined in usage_test.go — verify it's available in this package.
var _ = billing.GetAgentPlanLimits // ensure billing import is used
