package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func (m *mockUsageService) GetCurrentUsage(_ context.Context, orgID string, _, _ int) (*billing.CurrentUsageResponse, error) {
	return &billing.CurrentUsageResponse{OrgID: orgID, Plan: "starter"}, nil
}

func (m *mockUsageService) GetUsageHistory(_ context.Context, _ string, _, _ time.Time) ([]billing.UsageHistoryEntry, error) {
	return []billing.UsageHistoryEntry{}, nil
}

func (m *mockUsageService) GetUsageForecast(_ context.Context, _ string) (*billing.UsageForecastResponse, error) {
	return &billing.UsageForecastResponse{}, nil
}

func (m *mockUsageService) GetProjectCosts(_ context.Context, _ string, _, _ time.Time) ([]billing.ProjectCostEntry, error) {
	return []billing.ProjectCostEntry{}, nil
}

func (m *mockUsageService) ExportUsageCSV(_ context.Context, _ string, _, _ time.Time) ([]byte, error) {
	return []byte("date,project,runs\n"), nil
}

func (m *mockUsageService) GetSpendingLimit(_ context.Context, orgID string) (*billing.SpendingLimitResponse, error) {
	return &billing.SpendingLimitResponse{OrgID: orgID, PlanTier: "starter"}, nil
}

func (m *mockUsageService) SetSpendingLimit(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}

func (m *mockUsageService) PreviewDowngrade(_ context.Context, _ string, targetTier domain.PlanTier) (*billing.DowngradeImpact, error) {
	return &billing.DowngradeImpact{TargetTier: string(targetTier)}, nil
}

func (m *mockUsageService) DetectAnomalies(_ context.Context, _ string) ([]billing.AnomalyAlert, error) {
	return []billing.AnomalyAlert{}, nil
}

type mockReferralService struct{}

func (m *mockReferralService) GenerateCode(_ context.Context, orgID string) (*billing.Referral, error) {
	return &billing.Referral{ID: "ref-1", ReferrerOrgID: orgID, ReferralCode: "abc123", Status: "pending"}, nil
}

func (m *mockReferralService) ActivateReferral(_ context.Context, _ string, referredOrgID string) (*billing.Referral, error) {
	return &billing.Referral{ID: "ref-1", ReferredOrgID: referredOrgID, Status: "activated"}, nil
}

func (m *mockReferralService) ListReferrals(_ context.Context, _ string) ([]billing.Referral, error) {
	return []billing.Referral{}, nil
}

type usageTestServerOpts struct {
	enforcer    BillingEnforcer
	usageSvc    UsageService
	referralSvc ReferralService
}

func newUsageTestServer(t *testing.T, enforcer BillingEnforcer, usageSvc UsageService) *Server {
	t.Helper()
	return newUsageTestServerFull(t, usageTestServerOpts{
		enforcer: enforcer,
		usageSvc: usageSvc,
	})
}

func newUsageTestServerFull(t *testing.T, opts usageTestServerOpts) *Server {
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
		BillingEnforcer: opts.enforcer,
		UsageService:    opts.usageSvc,
		ReferralService: opts.referralSvc,
	})
	t.Cleanup(srv.Close)
	return srv
}

// apiKeyRequest creates an HTTP request with API key context (scopes + project).
func apiKeyRequest(method, url, body, projectID string) *http.Request {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	req.Header.Set("X-Internal-Secret", "test-secret")
	ctx := context.WithValue(req.Context(), ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxProjectIDKey, projectID)
	return req.WithContext(ctx)
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

func TestGetUsageHistory_Success(t *testing.T) {
	t.Parallel()

	srv := newUsageTestServer(t, &mockBillingEnforcer{}, &mockUsageService{})
	req := authedRequest(http.MethodGet, "/v1/usage/history?org_id=org-1&from=2025-01-01&to=2025-01-31", "")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetUsageHistory_MissingParams(t *testing.T) {
	t.Parallel()

	srv := newUsageTestServer(t, &mockBillingEnforcer{}, &mockUsageService{})

	tests := []struct {
		name string
		url  string
	}{
		{"missing_from", "/v1/usage/history?org_id=org-1&to=2025-01-31"},
		{"missing_to", "/v1/usage/history?org_id=org-1&from=2025-01-01"},
		{"missing_org_id", "/v1/usage/history?from=2025-01-01&to=2025-01-31"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodGet, tc.url, ""))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", tc.name, w.Code)
			}
		})
	}
}

func TestGetUsageForecast_Success(t *testing.T) {
	t.Parallel()

	srv := newUsageTestServer(t, &mockBillingEnforcer{}, &mockUsageService{})
	req := authedRequest(http.MethodGet, "/v1/usage/forecast?org_id=org-1", "")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetProjectCosts_Success(t *testing.T) {
	t.Parallel()

	srv := newUsageTestServer(t, &mockBillingEnforcer{}, &mockUsageService{})
	req := authedRequest(http.MethodGet, "/v1/usage/projects?org_id=org-1&from=2025-01-01&to=2025-01-31", "")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetCostEstimate_InvalidPreset_400(t *testing.T) {
	t.Parallel()

	srv := newUsageTestServer(t, &mockBillingEnforcer{}, &mockUsageService{})
	req := authedRequest(http.MethodGet, "/v1/cost-estimate?preset=nonexistent&timeout_secs=60", "")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid preset, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetDowngradePreview_InvalidTier_400(t *testing.T) {
	t.Parallel()

	srv := newUsageTestServer(t, &mockBillingEnforcer{}, &mockUsageService{})
	req := authedRequest(http.MethodGet, "/v1/downgrade-preview?org_id=org-1&target_tier=nonexistent", "")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid target tier, got %d", w.Code)
	}
}

func TestExportCSV_ReturnsValidCSV(t *testing.T) {
	t.Parallel()

	srv := newUsageTestServer(t, &mockBillingEnforcer{}, &mockUsageService{})
	req := authedRequest(http.MethodGet, "/v1/usage/export?org_id=org-1&from=2025-01-01&to=2025-01-31", "")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/csv" {
		t.Errorf("expected Content-Type text/csv, got %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd == "" {
		t.Error("expected Content-Disposition header")
	}
}

func TestAnomalyAlerts_NoHistoryReturnsEmpty(t *testing.T) {
	t.Parallel()

	srv := newUsageTestServer(t, &mockBillingEnforcer{}, &mockUsageService{})
	req := authedRequest(http.MethodGet, "/v1/usage/anomalies?org_id=org-1", "")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var alerts []billing.AnomalyAlert
	if err := json.Unmarshal(w.Body.Bytes(), &alerts); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected empty alerts, got %d", len(alerts))
	}
}

func TestGetUsageHistory_APIKey_CrossTenantForbidden(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})

	req := httptest.NewRequest(http.MethodGet, "/v1/usage/history?org_id=org-B&from=2025-01-01&to=2025-01-31", nil)
	req.Header.Set("X-Internal-Secret", "test-secret")
	ctx := context.WithValue(req.Context(), ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-tenant usage history, got %d", w.Code)
	}
}

// Cost estimate cross-tenant tests.

func TestCostEstimate_APIKey_CrossTenantForbidden(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer: enforcer,
		usageSvc: &mockUsageService{},
	})

	req := apiKeyRequest(http.MethodGet, "/v1/cost-estimate?org_id=org-B&preset=micro&timeout_secs=60", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-tenant cost estimate, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCostEstimate_APIKey_SameTenantAllowed(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer: enforcer,
		usageSvc: &mockUsageService{},
	})

	req := apiKeyRequest(http.MethodGet, "/v1/cost-estimate?org_id=org-A&preset=micro&timeout_secs=60", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for same-tenant cost estimate, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCostEstimate_APIKey_NoOrgID_Allowed(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer: enforcer,
		usageSvc: &mockUsageService{},
	})

	req := apiKeyRequest(http.MethodGet, "/v1/cost-estimate?preset=micro&timeout_secs=60", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 without org_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCostEstimate_InternalSecret_AllowsAnyOrg(t *testing.T) {
	t.Parallel()

	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer: &mockBillingEnforcer{},
		usageSvc: &mockUsageService{},
	})

	req := authedRequest(http.MethodGet, "/v1/cost-estimate?org_id=any-org&preset=micro&timeout_secs=60", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for internal secret cost estimate, got %d: %s", w.Code, w.Body.String())
	}
}

// List referrals cross-tenant tests.

func TestListReferrals_APIKey_CrossTenantForbidden(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer:    enforcer,
		referralSvc: &mockReferralService{},
	})

	req := apiKeyRequest(http.MethodGet, "/v1/referrals?org_id=org-B", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListReferrals_APIKey_SameTenantAllowed(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer:    enforcer,
		referralSvc: &mockReferralService{},
	})

	req := apiKeyRequest(http.MethodGet, "/v1/referrals?org_id=org-A", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListReferrals_InternalSecret_AllowsAnyOrg(t *testing.T) {
	t.Parallel()

	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer:    &mockBillingEnforcer{},
		referralSvc: &mockReferralService{},
	})

	req := authedRequest(http.MethodGet, "/v1/referrals?org_id=any-org", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// Create referral cross-tenant tests.

func TestCreateReferral_APIKey_CrossTenantForbidden(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer:    enforcer,
		referralSvc: &mockReferralService{},
	})

	req := apiKeyRequest(http.MethodPost, "/v1/referrals", `{"org_id":"org-B"}`, "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateReferral_APIKey_SameTenantAllowed(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer:    enforcer,
		referralSvc: &mockReferralService{},
	})

	req := apiKeyRequest(http.MethodPost, "/v1/referrals", `{"org_id":"org-A"}`, "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateReferral_InternalSecret_AllowsAnyOrg(t *testing.T) {
	t.Parallel()

	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer:    &mockBillingEnforcer{},
		referralSvc: &mockReferralService{},
	})

	req := authedRequest(http.MethodPost, "/v1/referrals", `{"org_id":"any-org"}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// Activate referral cross-tenant tests.

func TestActivateReferral_APIKey_CrossTenantForbidden(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer:    enforcer,
		referralSvc: &mockReferralService{},
	})

	req := apiKeyRequest(http.MethodPost, "/v1/referrals/activate", `{"code":"xxx","referred_org_id":"org-B"}`, "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestActivateReferral_APIKey_SameTenantAllowed(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer:    enforcer,
		referralSvc: &mockReferralService{},
	})

	req := apiKeyRequest(http.MethodPost, "/v1/referrals/activate", `{"code":"xxx","referred_org_id":"org-A"}`, "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
