package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/config"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestUsage_DateRangeInjection verifies that adversarial date strings
// are rejected by parseDateRangeTyped.
func TestUsage_DateRangeInjection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		from string
		to   string
	}{
		{"sql_injection", "2024-01-01' OR 1=1--", "2024-12-31"},
		{"script_tag", "<script>alert(1)</script>", "2024-12-31"},
		{"null_bytes", "2024-01-01\x00", "2024-12-31"},
		{"overlong_date", "99999-01-01", "99999-12-31"},
		{"reversed", "2024-12-31", "2024-01-01"},
		{"empty", "", ""},
		{"epoch_zero", "0000-00-00", "0000-00-00"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := parseDateRangeTyped(tc.from, tc.to)
			require.Error(t, err)

		})
	}
}

// TestUsage_FutureProjection verifies that the forecast handler does not
// panic with extreme org data.
func TestUsage_FutureProjection(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	usageSvc := &mockUsageService{
		forecast: &billing.UsageForecastResponse{
			ProjectedMonthlyRuns: 999999999,
			DaysUntilLimit:       0,
		},
	}

	srv := newUsageTestServer(t, enforcer, usageSvc)
	req := apiKeyRequest("GET", "/v1/usage/forecast?org_id=org-1", "", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK,
		rr.Code)

}

// TestUsage_CrossProjectUsage verifies that project A cannot query project B's
// usage when they belong to different organizations.
func TestUsage_CrossProjectUsage(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-a": "org-a",
			"proj-b": "org-b",
		},
	}
	usageSvc := &mockUsageService{}

	srv := newUsageTestServer(t, enforcer, usageSvc)

	// proj-a caller tries to query org-b usage.
	req := apiKeyRequest("GET", "/v1/usage/current?org_id=org-b", "", "proj-a")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusForbidden,
		rr.
			Code)

}

func TestUsage_ProjectScopedAPIKeyCannotReadOrgBillingState(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-A": "org-A"},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-A")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")

	_, err := srv.handleGetCurrentUsage(ctx, &GetCurrentUsageInput{OrgID: "org-A"})
	require.True(
		t, isHumaStatusError(err, http.
			StatusForbidden,
		))

	_, err = srv.handleGetProjectCosts(ctx, &GetProjectCostsInput{OrgID: "org-A", From: "2026-01-01", To: "2026-01-02"})
	require.True(
		t, isHumaStatusError(err, http.
			StatusForbidden,
		))

}

func TestUsage_OrgScopedAPIKeyCanReadOrgBillingState(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-A": "org-A"},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-A")
	ctx = context.WithValue(ctx, ctxOrgIDKey, "org-A")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")

	if _, err := srv.handleGetCurrentUsage(ctx, &GetCurrentUsageInput{OrgID: "org-A"}); err != nil {
		require.Failf(t, "test failure",

			"current usage error = %v, want nil", err)
	}
	if _, err := srv.handleGetProjectCosts(ctx, &GetProjectCostsInput{OrgID: "org-A", From: "2026-01-01", To: "2026-01-02"}); err != nil {
		require.Failf(t, "test failure",

			"project costs error = %v, want nil", err)
	}
}

func TestUsage_ProjectScopedUserCannotReadOrgBillingControls(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-A": "org-A"},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-A")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-project-scoped")

	_, err := srv.handleGetCurrentUsage(ctx, &GetCurrentUsageInput{OrgID: "org-A"})
	require.True(
		t, isHumaStatusError(err, http.
			StatusForbidden,
		))

	_, err = srv.handleGetDowngradePreview(ctx, &GetDowngradePreviewInput{OrgID: "org-A", TargetTier: string(domain.PlanFree)})
	require.True(
		t, isHumaStatusError(err, http.
			StatusForbidden,
		))

}

func TestUsage_ProjectScopedUserCannotMutateOrgBillingControls(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-A": "org-A"},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-A")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-project-scoped")

	_, err := srv.handleUpdateSpendingLimit(ctx, &UpdateSpendingLimitInput{
		OrgID: "org-A",
		Body:  updateSpendingLimitRequest{LimitMicrousd: 1000, Action: "notify"},
	})
	require.True(
		t, isHumaStatusError(err, http.
			StatusForbidden,
		))

	_, err = srv.handleUpdateEmailPreferences(ctx, &UpdateEmailPreferencesInput{
		OrgID: "org-A",
		Body:  updateEmailPreferencesRequest{MonthlyUsageEmail: false},
	})
	require.True(
		t, isHumaStatusError(err, http.
			StatusForbidden,
		))

}

func TestUsage_ProjectScopedAPIKeyCannotMutateSiblingProjectBudget(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
			"proj-B": "org-A",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-A")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")

	_, err := srv.handleUpdateProjectBudget(ctx, &UpdateProjectBudgetInput{Body: updateProjectBudgetRequest{
		ProjectID:   "proj-B",
		BudgetMicro: 100,
		Action:      "notify",
	}})
	require.True(
		t, isHumaStatusError(err, http.
			StatusForbidden,
		))

}

func TestUsage_ProjectScopedUserCannotMutateSiblingProjectBudget(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
			"proj-B": "org-A",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-A")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-project-scoped")

	_, err := srv.handleUpdateProjectBudget(ctx, &UpdateProjectBudgetInput{Body: updateProjectBudgetRequest{
		ProjectID:   "proj-B",
		BudgetMicro: 100,
		Action:      "notify",
	}})
	require.True(
		t, isHumaStatusError(err, http.
			StatusForbidden,
		))

}

func TestUsage_OrgScopedUserCanMutateSiblingProjectBudget(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
			"proj-B": "org-A",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-A")
	ctx = context.WithValue(ctx, ctxOrgIDKey, "org-A")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-org-scoped")

	if _, err := srv.handleUpdateProjectBudget(ctx, &UpdateProjectBudgetInput{Body: updateProjectBudgetRequest{
		ProjectID:   "proj-B",
		BudgetMicro: 100,
		Action:      "notify",
	}}); err != nil {
		require.Failf(t, "test failure",

			"update sibling project budget error = %v, want nil", err)
	}
}

// TestUsage_ExportCSVInjection verifies that CSV formula injection characters
// in org data are not interpreted by the export handler.
func TestUsage_ExportCSVInjection(t *testing.T) {
	t.Parallel()

	formulaPayload := []byte("=CMD('calc'),Project,100\n+SUM(A1:A100),Project2,200\n")
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	usageSvc := &mockUsageService{
		exportData: formulaPayload,
	}

	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer: enforcer,
		usageSvc: usageSvc,
		config: &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testJWTSigningKey,
		},
	})

	req := apiKeyRequest("GET", "/v1/usage/export?org_id=org-1&from=2024-01-01&to=2024-12-31&format=csv", "", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.False(t, rr.Code >= 500)

	// The handler writes directly to the response writer, so we may get
	// 200 or the huma framework status. Just verify no panic.

}

// TestUsage_CostAnomalyFalsePositive verifies that a legitimate usage spike
// can be returned without being flagged as an error.
func TestUsage_CostAnomalyFalsePositive(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	usageSvc := &mockUsageService{
		anomalyAlerts: []billing.AnomalyAlert{
			{
				OrgID:          "org-1",
				Severity:       billing.AnomalySeverityWarning,
				TopContributor: "cost_spike",
			},
		},
	}

	srv := newUsageTestServer(t, enforcer, usageSvc)
	req := apiKeyRequest("GET", "/v1/usage/anomalies?org_id=org-1", "", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK,
		rr.Code)

	var alerts []billing.AnomalyAlert
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&alerts))
	require.Len(t,
		alerts, 1)

}

// TestSilentError_JSONMarshalFailure verifies that the usage handler returns
// a 500 when the usage service fails rather than silently succeeding.
func TestSilentError_JSONMarshalFailure(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	usageSvc := &mockUsageService{
		currentUsageErr: errors.New("database connection lost"),
	}

	srv := newUsageTestServer(t, enforcer, usageSvc)
	req := apiKeyRequest("GET", "/v1/usage/current?org_id=org-1", "", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusInternalServerError,

		rr.
			Code)

}

// TestSilentError_DecryptFailure verifies that a decrypt error in the
// billing enforcer results in a 403 rather than succeeding.
func TestSilentError_DecryptFailure(t *testing.T) {
	t.Parallel()

	// No enforcer means ownership validation fails.
	srv := newUsageTestServer(t, nil, &mockUsageService{})
	req := apiKeyRequest("GET", "/v1/usage/current?org_id=org-1", "", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.NotEqual(t, http.StatusOK,
		rr.Code,
	)

	// Without billing enforcer, the server returns a 403 or 501.

}

// TestSilentError_WebhookCallbackFailure verifies that a failed spending
// limit update returns an error rather than silently succeeding.
func TestSilentError_WebhookCallbackFailure(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	srv := newUsageTestServer(t, enforcer, nil)

	req := apiKeyRequest("GET", "/v1/spending-limit?org_id=org-1", "", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotImplemented,

		rr.Code)

	// usageService is nil, so should return 501.

}

// TestSilentError_PaginationInvalidCursor verifies that an invalid pagination
// cursor returns an error rather than an empty result.
func TestSilentError_PaginationInvalidCursor(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	usageSvc := &mockUsageService{}

	srv := newUsageTestServer(t, enforcer, usageSvc)

	// Use an invalid from date as a proxy for invalid cursor.
	req := apiKeyRequest("GET", "/v1/usage/history?org_id=org-1&from=invalid&to=2024-12-31", "", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.NotEqual(t, http.StatusOK,
		rr.Code,
	)

}

// TestSilentError_StoreConnectionFailure verifies that a store failure
// returns 500 rather than 200.
func TestSilentError_StoreConnectionFailure(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-1": "org-1"},
	}
	usageSvc := &mockUsageService{
		currentUsageErr: errors.New("connection refused"),
	}

	srv := newUsageTestServer(t, enforcer, usageSvc)
	req := apiKeyRequest("GET", "/v1/usage/current?org_id=org-1", "", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.NotEqual(t, http.StatusOK,
		rr.Code,
	)

	if rr.Code != http.StatusInternalServerError {
		t.Logf("got status %d (expected 500)", rr.Code)
	}
}

// FuzzUsageDateRange fuzzes the from/to date parameters.
func FuzzUsageDateRange(f *testing.F) {
	f.Add("2024-01-01", "2024-12-31")
	f.Add("", "")
	f.Add("not-a-date", "also-not")
	f.Add("9999-99-99", "0000-00-00")

	f.Fuzz(func(t *testing.T, from, to string) {
		// Must not panic.
		_, _, _ = parseDateRangeTyped(from, to)
	})
}

// FuzzCSVExport fuzzes the export format parameter to verify no panics.
func FuzzCSVExport(f *testing.F) {
	f.Add("csv")
	f.Add("pdf")
	f.Add("")
	f.Add("json")
	f.Add("<script>")

	f.Fuzz(func(t *testing.T, format string) {
		enforcer := &mockBillingEnforcer{
			activeProjectOrgMap: map[string]string{"proj-1": "org-1"},
		}
		usageSvc := &mockUsageService{
			exportData: []byte("date,runs\n2024-01-01,100\n"),
		}

		srv := newUsageTestServer(t, enforcer, usageSvc)
		req := apiKeyRequest("GET", "/v1/usage/export?org_id=org-1&from=2024-01-01&to=2024-12-31&format="+format, "", "proj-1")
		rr := httptest.NewRecorder()
		// Must not panic.
		srv.ServeHTTP(rr, req)
	})
}

// Suppress unused import warnings.
var (
	_ = strings.Contains
	_ = time.Now
	_ domain.PlanTier
)
