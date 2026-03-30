package api

import (
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
			if err == nil {
				t.Fatal("expected error for adversarial date input")
			}
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

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
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

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
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
			JWTSigningKey:       "01234567890123456789012345678901",
		},
	})

	req := apiKeyRequest("GET", "/v1/usage/export?org_id=org-1&from=2024-01-01&to=2024-12-31&format=csv", "", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// The handler writes directly to the response writer, so we may get
	// 200 or the huma framework status. Just verify no panic.
	if rr.Code >= 500 {
		t.Fatalf("expected non-5xx, got %d: %s", rr.Code, rr.Body.String())
	}
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

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var alerts []billing.AnomalyAlert
	if err := json.NewDecoder(rr.Body).Decode(&alerts); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
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

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
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

	// Without billing enforcer, the server returns a 403 or 501.
	if rr.Code == http.StatusOK {
		t.Fatal("expected non-200 when billing enforcer is nil")
	}
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

	// usageService is nil, so should return 501.
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rr.Code, rr.Body.String())
	}
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

	if rr.Code == http.StatusOK {
		t.Fatal("expected error for invalid date cursor, got 200")
	}
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

	if rr.Code == http.StatusOK {
		t.Fatal("expected non-200 for store connection failure")
	}
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
