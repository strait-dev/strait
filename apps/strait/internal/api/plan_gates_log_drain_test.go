package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// tunableLimitsEnforcer allows individual tests to override the plan limits
// returned by the enforcer. Used by all resource-cap gate tests in this file
// and its companions (notification channels, alert rules) so we don't need to
// fixture a real PlanRegistry.
type tunableLimitsEnforcer struct {
	mockBillingEnforcer
	limits    billing.OrgPlanLimits
	limitsErr error
	orgID     string
	orgErr    error
	emptyOrg  bool
	getCall   atomic.Int64
}

func (t *tunableLimitsEnforcer) GetProjectOrgID(_ context.Context, _ string) (string, error) {
	if t.orgErr != nil {
		return "", t.orgErr
	}
	if t.emptyOrg {
		return "", nil
	}
	if t.orgID == "" {
		return "org-1", nil
	}
	return t.orgID, nil
}

func (t *tunableLimitsEnforcer) GetActiveProjectOrgID(ctx context.Context, projectID string) (string, error) {
	return t.GetProjectOrgID(ctx, projectID)
}

func (t *tunableLimitsEnforcer) GetOrgPlanLimits(_ context.Context, _ string) (billing.OrgPlanLimits, error) {
	t.getCall.Add(1)
	if t.limitsErr != nil {
		return billing.OrgPlanLimits{}, t.limitsErr
	}
	return t.limits, nil
}

func freeLimits() billing.OrgPlanLimits {
	return billing.GetPlanLimits(domain.PlanFree)
}

func proLimits() billing.OrgPlanLimits {
	return billing.GetPlanLimits(domain.PlanPro)
}

func enterpriseLimits() billing.OrgPlanLimits {
	return billing.GetPlanLimits(domain.PlanEnterprise)
}

func validLogDrainBody() string {
	return `{
		"project_id": "proj-1",
		"name": "drain-1",
		"drain_type": "http",
		"endpoint_url": "https://example.com/logs",
		"auth_type": "bearer",
		"auth_config": {"token": "abc"}
	}`
}

// TestCreateLogDrain_FreeTier_RejectsZeroCap proves Free (cap=0) rejects every
// log-drain create with the "not available on the X plan" message, before any
// store call happens.
func TestCreateLogDrain_FreeTier_RejectsZeroCap(t *testing.T) {
	t.Parallel()

	createCalls := atomic.Int64{}
	ms := &APIStoreMock{
		CreateLogDrainFunc: func(_ context.Context, _ *domain.LogDrain) error {
			createCalls.Add(1)
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: freeLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", validLogDrainBody()))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not available") {
		t.Errorf("error message must say feature is not available, got: %s", w.Body.String())
	}
	if got := createCalls.Load(); got != 0 {
		t.Errorf("CreateLogDrain called %d times before plan-gate rejection; want 0", got)
	}
}

// TestCreateLogDrain_ProTier_BlocksAtCap verifies that on Pro (cap=5) the 6th
// log drain is rejected with a message naming the cap and current count.
func TestCreateLogDrain_ProTier_BlocksAtCap(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CountLogDrainsByOrgFunc: func(_ context.Context, _ string) (int, error) {
			return 5, nil
		},
		CreateLogDrainFunc: func(_ context.Context, _ *domain.LogDrain) error {
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: proLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", validLogDrainBody()))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "5 log drains") || !strings.Contains(body, "have 5") {
		t.Errorf("error message must report cap and current count, got: %s", body)
	}
}

// TestCreateLogDrain_ProTier_BelowCap_Succeeds verifies that on Pro (cap=5)
// with 4 existing drains, the 5th create succeeds (cap is exclusive on the
// new entry; >= triggers the gate, so 4 < 5 must pass).
func TestCreateLogDrain_ProTier_BelowCap_Succeeds(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CountLogDrainsByOrgFunc: func(_ context.Context, _ string) (int, error) {
			return 4, nil
		},
		CreateLogDrainFunc: func(_ context.Context, _ *domain.LogDrain) error {
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: proLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", validLogDrainBody()))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCreateLogDrain_EnterpriseUnlimited_NoCountLookup verifies that an
// unlimited tier (cap=-1) skips the count query entirely so we don't burn a
// SQL roundtrip per create on the largest customers.
func TestCreateLogDrain_EnterpriseUnlimited_NoCountLookup(t *testing.T) {
	t.Parallel()

	countCalls := atomic.Int64{}
	ms := &APIStoreMock{
		CountLogDrainsByOrgFunc: func(_ context.Context, _ string) (int, error) {
			countCalls.Add(1)
			return 9999, nil
		},
		CreateLogDrainFunc: func(_ context.Context, _ *domain.LogDrain) error {
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: enterpriseLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", validLogDrainBody()))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if got := countCalls.Load(); got != 0 {
		t.Errorf("CountLogDrainsByOrg called %d times for unlimited tier; want 0", got)
	}
}

// TestCreateLogDrain_NilEnforcer_FailsOpen confirms that community-edition
// behavior (no billing enforcer) does not block log drain creation.
func TestCreateLogDrain_NilEnforcer_FailsOpen(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateLogDrainFunc: func(_ context.Context, _ *domain.LogDrain) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.edition = domain.EditionCommunity

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", validLogDrainBody()))

	if w.Code != http.StatusCreated {
		t.Fatalf("nil enforcer must fail open; got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateLogDrain_CloudEmptyOrgLookupFailsClosed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CountLogDrainsByOrgFunc: func(_ context.Context, _ string) (int, error) {
			t.Fatal("empty org lookup must fail before the count query")
			return 0, nil
		},
		CreateLogDrainFunc: func(_ context.Context, _ *domain.LogDrain) error {
			t.Fatal("empty org lookup must fail before create")
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{
		limits:   proLimits(),
		emptyOrg: true,
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", validLogDrainBody()))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("empty org lookup must fail closed; got %d: %s", w.Code, w.Body.String())
	}
}

// TestCreateLogDrain_CountQueryFails_FailsClosed ensures a transient store
// failure on the cap check does not bypass the paid log-drain cap.
func TestCreateLogDrain_CountQueryFails_FailsClosed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CountLogDrainsByOrgFunc: func(_ context.Context, _ string) (int, error) {
			return 0, fmt.Errorf("transient db failure")
		},
		CreateLogDrainFunc: func(_ context.Context, _ *domain.LogDrain) error {
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: proLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", validLogDrainBody()))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("count failure must fail closed; got %d: %s", w.Code, w.Body.String())
	}
}
