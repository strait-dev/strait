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

	"github.com/golang-jwt/jwt/v5"
)

type mockBillingEnforcer struct {
	projectOrgMap       map[string]string
	activeProjectOrgMap map[string]string
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

func (m *mockBillingEnforcer) GetActiveProjectOrgID(_ context.Context, projectID string) (string, error) {
	if m.activeProjectOrgMap != nil {
		return m.activeProjectOrgMap[projectID], nil
	}
	if m.projectOrgMap != nil {
		return m.projectOrgMap[projectID], nil
	}
	return "", nil
}

type mockUsageService struct {
	currentUsage    *billing.CurrentUsageResponse
	spendingLimit   *billing.SpendingLimitResponse
	projectCosts    []billing.ProjectCostEntry
	usageHistory    []billing.UsageHistoryEntry
	anomalyAlerts   []billing.AnomalyAlert
	exportData      []byte
	forecast        *billing.UsageForecastResponse
	downgrade       *billing.DowngradeImpact
	currentUsageErr error
}

func (m *mockUsageService) GetCurrentUsage(_ context.Context, orgID string) (*billing.CurrentUsageResponse, error) {
	if m.currentUsageErr != nil {
		return nil, m.currentUsageErr
	}
	if m.currentUsage != nil {
		resp := *m.currentUsage
		if resp.OrgID == "" {
			resp.OrgID = orgID
		}
		return &resp, nil
	}
	return &billing.CurrentUsageResponse{OrgID: orgID, Plan: "starter"}, nil
}

func (m *mockUsageService) GetUsageHistory(_ context.Context, _ string, _, _ time.Time) ([]billing.UsageHistoryEntry, error) {
	if m.usageHistory != nil {
		return m.usageHistory, nil
	}
	return []billing.UsageHistoryEntry{}, nil
}

func (m *mockUsageService) GetUsageForecast(_ context.Context, _ string) (*billing.UsageForecastResponse, error) {
	if m.forecast != nil {
		return m.forecast, nil
	}
	return &billing.UsageForecastResponse{}, nil
}

func (m *mockUsageService) GetProjectCosts(_ context.Context, _ string, _, _ time.Time) ([]billing.ProjectCostEntry, error) {
	if m.projectCosts != nil {
		return m.projectCosts, nil
	}
	return []billing.ProjectCostEntry{}, nil
}

func (m *mockUsageService) ExportUsageCSV(_ context.Context, _ string, _, _ time.Time) ([]byte, error) {
	if m.exportData != nil {
		return m.exportData, nil
	}
	return []byte("date,project,runs\n"), nil
}

func (m *mockUsageService) GetSpendingLimit(_ context.Context, orgID string) (*billing.SpendingLimitResponse, error) {
	if m.spendingLimit != nil {
		resp := *m.spendingLimit
		if resp.OrgID == "" {
			resp.OrgID = orgID
		}
		return &resp, nil
	}
	return &billing.SpendingLimitResponse{OrgID: orgID, PlanTier: "starter"}, nil
}

func (m *mockUsageService) SetSpendingLimit(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}

func (m *mockUsageService) PreviewDowngrade(_ context.Context, _ string, targetTier domain.PlanTier) (*billing.DowngradeImpact, error) {
	if m.downgrade != nil {
		return m.downgrade, nil
	}
	return &billing.DowngradeImpact{TargetTier: string(targetTier)}, nil
}

func (m *mockUsageService) DetectAnomalies(_ context.Context, _ string) ([]billing.AnomalyAlert, error) {
	if m.anomalyAlerts != nil {
		return m.anomalyAlerts, nil
	}
	return []billing.AnomalyAlert{}, nil
}

func (m *mockUsageService) GetProjectBudget(_ context.Context, projectID string) (*billing.ProjectBudgetResponse, error) {
	return &billing.ProjectBudgetResponse{ProjectID: projectID, MonthlyBudgetMicro: -1, BudgetAction: "notify"}, nil
}

func (m *mockUsageService) SetProjectBudget(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}

func (m *mockUsageService) GetAnomalyConfig(_ context.Context, _ string) (*billing.AnomalyConfigResponse, error) {
	return &billing.AnomalyConfigResponse{WarningThreshold: 3.0, CriticalThreshold: 10.0}, nil
}

func (m *mockUsageService) SetAnomalyConfig(_ context.Context, _ string, _, _ float64) error {
	return nil
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
	store       *mockAPIStore
	config      *config.Config
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
	cfg := opts.config
	if cfg == nil {
		cfg = &config.Config{
			InternalSecret:      "test-secret",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       "01234567890123456789012345678901",
		}
	}
	ms := opts.store
	if ms == nil {
		ms = &mockAPIStore{}
	}
	if ms.listProjectsByOrgFn == nil {
		ms.listProjectsByOrgFn = func(_ context.Context, _ string) ([]domain.Project, error) {
			return nil, nil
		}
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

const usageTestOIDCUserID = "user-oidc-1"

// apiKeyRequest creates an HTTP request with API key context (scopes + project).
func apiKeyRequest(method, url, body, projectID string) *http.Request {
	return apiKeyRequestWithScopes(method, url, body, projectID, []string{"*"})
}

func apiKeyRequestWithScopes(method, url, body, projectID string, scopes []string) *http.Request {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	req.Header.Set("X-Internal-Secret", "test-secret")
	ctx := context.WithValue(req.Context(), ctxScopesKey, scopes)
	ctx = context.WithValue(ctx, ctxProjectIDKey, projectID)
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test-key")
	return req.WithContext(ctx)
}

func newOIDCUsageTestServer(
	t *testing.T,
	opts usageTestServerOpts,
	getUserPermissions func(context.Context, string, string) ([]string, error),
) (*Server, string) {
	t.Helper()

	key, pubPEM := mustOIDCKeyPair(t)
	token := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   usageTestOIDCUserID,
		Issuer:    "https://issuer.example",
		Audience:  []string{"strait-api"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	store := opts.store
	if store == nil {
		store = &mockAPIStore{}
	}
	if store.listProjectsByOrgFn == nil {
		store.listProjectsByOrgFn = func(_ context.Context, _ string) ([]domain.Project, error) {
			return nil, nil
		}
	}
	store.getUserPermissionsFn = getUserPermissions

	opts.store = store
	opts.config = &config.Config{
		InternalSecret:      "test-secret",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       "01234567890123456789012345678901",
		OIDCEnabled:         true,
		OIDCIssuer:          "https://issuer.example",
		OIDCAudience:        "strait-api",
		OIDCPublicKeyPEM:    string(pubPEM),
	}

	return newUsageTestServerFull(t, opts), token
}

func oidcRequest(method, url, body, token, projectID string) *http.Request {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if projectID != "" {
		req.Header.Set("X-Project-Id", projectID)
	}
	return req
}

func TestUsageEndpoint_APIKey_CrossTenantForbidden(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{
			"proj-1": "org-A",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})

	req := apiKeyRequest(http.MethodGet, "/v1/usage/current?org_id=org-B", "", "proj-1")

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

	req := apiKeyRequest(http.MethodGet, "/v1/usage/current?org_id=org-A", "", "proj-1")

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

func TestGetSpendingLimit_FreeTierReturns200(t *testing.T) {
	t.Parallel()

	srv := newUsageTestServer(t, &mockBillingEnforcer{}, &mockUsageService{
		spendingLimit: &billing.SpendingLimitResponse{
			PlanTier:          "free",
			LimitAction:       "reject",
			IsHardCapped:      true,
			IncludedCreditUsd: 0,
		},
	})
	req := authedRequest(http.MethodGet, "/v1/spending-limit?org_id=org-free", "")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp billing.SpendingLimitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if resp.PlanTier != "free" {
		t.Fatalf("plan tier = %q, want free", resp.PlanTier)
	}
	if !resp.IsHardCapped {
		t.Fatal("expected free tier spending limit response to be hard capped")
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

	req := apiKeyRequest(http.MethodGet, "/v1/usage/history?org_id=org-B&from=2025-01-01&to=2025-01-31", "", "proj-1")

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

func TestUsageEndpoint_OIDC_NoRoleForbidden(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv, token := newOIDCUsageTestServer(t, usageTestServerOpts{
		enforcer: enforcer,
		usageSvc: &mockUsageService{},
	}, func(_ context.Context, _, _ string) ([]string, error) {
		return nil, nil
	})

	req := oidcRequest(http.MethodGet, "/v1/usage/current?org_id=org-A", "", token, "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without a project role, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUsageEndpoint_OIDC_ReadRoleAllowed(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv, token := newOIDCUsageTestServer(t, usageTestServerOpts{
		enforcer: enforcer,
		usageSvc: &mockUsageService{},
	}, func(_ context.Context, projectID, userID string) ([]string, error) {
		if projectID != "proj-1" || userID != usageTestOIDCUserID {
			t.Fatalf("permission lookup args = (%s,%s), want (proj-1,%s)", projectID, userID, usageTestOIDCUserID)
		}
		return []string{domain.ScopeProjectsRead}, nil
	})

	req := oidcRequest(http.MethodGet, "/v1/usage/current?org_id=org-A", "", token, "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with projects:read, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUsageEndpoint_OIDC_MismatchedOrgForbidden(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv, token := newOIDCUsageTestServer(t, usageTestServerOpts{
		enforcer: enforcer,
		usageSvc: &mockUsageService{},
	}, func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeProjectsRead}, nil
	})

	req := oidcRequest(http.MethodGet, "/v1/usage/current?org_id=org-B", "", token, "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for mismatched org_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUsageEndpoint_OIDC_MissingProjectContextForbidden(t *testing.T) {
	t.Parallel()

	srv, token := newOIDCUsageTestServer(t, usageTestServerOpts{
		enforcer: &mockBillingEnforcer{},
		usageSvc: &mockUsageService{},
	}, func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeProjectsRead}, nil
	})

	req := oidcRequest(http.MethodGet, "/v1/usage/current?org_id=org-A", "", token, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without X-Project-Id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUsageEndpoint_OIDC_DeletedProjectContextForbidden(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
		activeProjectOrgMap: map[string]string{
			"proj-1": "",
		},
	}
	srv, token := newOIDCUsageTestServer(t, usageTestServerOpts{
		enforcer: enforcer,
		usageSvc: &mockUsageService{},
	}, func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeProjectsRead}, nil
	})

	req := oidcRequest(http.MethodGet, "/v1/usage/current?org_id=org-A", "", token, "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for deleted project context, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSpendingLimit_OIDC_ReadRoleCannotMutate(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv, token := newOIDCUsageTestServer(t, usageTestServerOpts{
		enforcer: enforcer,
		usageSvc: &mockUsageService{},
	}, func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeProjectsRead}, nil
	})

	req := oidcRequest(http.MethodPut, "/v1/spending-limit?org_id=org-A", `{"limit_microusd":1000,"action":"reject"}`, token, "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 with projects:read on spending limit update, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateReferral_OIDC_ManageRoleAllowed(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv, token := newOIDCUsageTestServer(t, usageTestServerOpts{
		enforcer:    enforcer,
		referralSvc: &mockReferralService{},
	}, func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeProjectsManage}, nil
	})

	req := oidcRequest(http.MethodPost, "/v1/referrals", `{"org_id":"org-A"}`, token, "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 with projects:manage, got %d: %s", w.Code, w.Body.String())
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

func TestUsageEndpoint_APIKey_ReadScopeRequired(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})

	req := apiKeyRequestWithScopes(http.MethodGet, "/v1/usage/current?org_id=org-A", "", "proj-1", []string{domain.ScopeJobsRead})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without projects:read, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUsageEndpoint_APIKey_ReadScopeAllowed(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})

	req := apiKeyRequestWithScopes(http.MethodGet, "/v1/usage/current?org_id=org-A", "", "proj-1", []string{domain.ScopeProjectsRead})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with projects:read, got %d: %s", w.Code, w.Body.String())
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

func TestCreateReferral_APIKey_ManageScopeRequired(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer:    enforcer,
		referralSvc: &mockReferralService{},
	})

	req := apiKeyRequestWithScopes(http.MethodPost, "/v1/referrals", `{"org_id":"org-A"}`, "proj-1", []string{domain.ScopeProjectsRead})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without projects:manage, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateReferral_APIKey_ManageScopeAllowed(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		projectOrgMap: map[string]string{"proj-1": "org-A"},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer:    enforcer,
		referralSvc: &mockReferralService{},
	})

	req := apiKeyRequestWithScopes(http.MethodPost, "/v1/referrals", `{"org_id":"org-A"}`, "proj-1", []string{domain.ScopeProjectsManage})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 with projects:manage, got %d: %s", w.Code, w.Body.String())
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

// internalSecretRequestWithProject creates an internal-secret request with a
// project ID in the context, simulating an internal caller with project scope.
// Uses the X-Project-Id header so the auth middleware sets the correct context.
func internalSecretRequestWithProject(method, url, body, projectID string) *http.Request {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	req.Header.Set("X-Internal-Secret", "test-secret")
	if projectID != "" {
		req.Header.Set("X-Project-Id", projectID)
	}
	return req
}

// Project budget cross-tenant tests.

func TestGetProjectBudget_InternalSecret_CrossOrgForbidden(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
			"proj-B": "org-B",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	req := internalSecretRequestWithProject(http.MethodGet, "/v1/project-budget?project_id=proj-B", "", "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetProjectBudget_InternalSecret_SameOrgAllowed(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A":  "org-A",
			"proj-A2": "org-A",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	req := internalSecretRequestWithProject(http.MethodGet, "/v1/project-budget?project_id=proj-A2", "", "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetProjectBudget_APIKey_CrossOrgForbidden(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
			"proj-B": "org-B",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	// Use X-Project-Id header to ensure the caller's project context is proj-A,
	// not overwritten by the project_id query param in the auth middleware.
	req := apiKeyRequest(http.MethodGet, "/v1/project-budget?project_id=proj-B", "", "proj-A")
	req.Header.Set("X-Project-Id", "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetProjectBudget_APIKey_SameOrgAllowed(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	req := apiKeyRequest(http.MethodGet, "/v1/project-budget?project_id=proj-A", "", "proj-A")
	req.Header.Set("X-Project-Id", "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateProjectBudget_InternalSecret_CrossOrgForbidden(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
			"proj-B": "org-B",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	body := `{"project_id":"proj-B","budget_microusd":1000000,"action":"notify"}`
	req := internalSecretRequestWithProject(http.MethodPut, "/v1/project-budget", body, "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateProjectBudget_InternalSecret_SameOrgAllowed(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	body := `{"project_id":"proj-A","budget_microusd":1000000,"action":"notify"}`
	req := internalSecretRequestWithProject(http.MethodPut, "/v1/project-budget", body, "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// Edge cases.

func TestGetProjectBudget_NoProjectContext_BadRequest(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-A": "org-A"},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	// Internal secret with no project_id at all: should get 400 (missing param).
	req := authedRequest(http.MethodGet, "/v1/project-budget", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without project_id param, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetProjectBudget_NonexistentProject_Forbidden(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-A": "org-A"},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	req := internalSecretRequestWithProject(http.MethodGet, "/v1/project-budget?project_id=proj-nonexistent", "", "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for nonexistent project, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateProjectBudget_EmptyProjectID_BadRequest(t *testing.T) {
	t.Parallel()
	srv := newUsageTestServer(t, &mockBillingEnforcer{}, &mockUsageService{})
	body := `{"project_id":"","budget_microusd":1000000,"action":"notify"}`
	req := authedRequest(http.MethodPut, "/v1/project-budget", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateProjectBudget_MissingBody_BadRequest(t *testing.T) {
	t.Parallel()
	srv := newUsageTestServer(t, &mockBillingEnforcer{}, &mockUsageService{})
	req := authedRequest(http.MethodPut, "/v1/project-budget", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
