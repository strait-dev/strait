package scheduler

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"strait/internal/billing"
)

type mockGraceEnforcerStore struct {
	mu               sync.Mutex
	graceOrgs        []billing.OrgSubscription
	freshSubs        map[string]*billing.OrgSubscription
	listErr          error
	updatedStatuses  map[string]string
	updatedPlans     map[string]string
	updateStatusErrs map[string]error
	updatePlanErrs   map[string]error
	ineligibleOrgs   map[string]bool
}

func (m *mockGraceEnforcerStore) GetOrgSubscription(_ context.Context, orgID string) (*billing.OrgSubscription, error) {
	if m.freshSubs != nil {
		if sub, ok := m.freshSubs[orgID]; ok {
			return sub, nil
		}
	}
	// Default: return the sub from graceOrgs with matching OrgID.
	for i := range m.graceOrgs {
		if m.graceOrgs[i].OrgID == orgID {
			return &m.graceOrgs[i], nil
		}
	}
	return nil, fmt.Errorf("subscription not found")
}

func (m *mockGraceEnforcerStore) ListOrgsInGracePeriod(_ context.Context) ([]billing.OrgSubscription, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.graceOrgs, nil
}

func (m *mockGraceEnforcerStore) UpdatePaymentStatus(_ context.Context, orgID string, status string, _ *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateStatusErrs != nil {
		if err, ok := m.updateStatusErrs[orgID]; ok {
			return err
		}
	}
	if m.updatedStatuses == nil {
		m.updatedStatuses = make(map[string]string)
	}
	m.updatedStatuses[orgID] = status
	return nil
}

func (m *mockGraceEnforcerStore) UpdateOrgSubscriptionPlan(_ context.Context, orgID, planTier, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updatePlanErrs != nil {
		if err, ok := m.updatePlanErrs[orgID]; ok {
			return err
		}
	}
	if m.updatedPlans == nil {
		m.updatedPlans = make(map[string]string)
	}
	m.updatedPlans[orgID] = planTier
	return nil
}

func (m *mockGraceEnforcerStore) RestrictExpiredGracePeriod(_ context.Context, orgID string, _ *time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateStatusErrs != nil {
		if err, ok := m.updateStatusErrs[orgID]; ok {
			return false, err
		}
	}
	if m.updatePlanErrs != nil {
		if err, ok := m.updatePlanErrs[orgID]; ok {
			return false, err
		}
	}
	if m.ineligibleOrgs != nil && m.ineligibleOrgs[orgID] {
		return false, nil
	}
	if m.updatedStatuses == nil {
		m.updatedStatuses = make(map[string]string)
	}
	if m.updatedPlans == nil {
		m.updatedPlans = make(map[string]string)
	}
	m.updatedStatuses[orgID] = "restricted"
	m.updatedPlans[orgID] = "free"
	return true, nil
}

func TestGraceEnforcer_PastGrace_RestrictsToFree(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	store := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{
				OrgID:          "org-expired",
				PlanTier:       "pro",
				PaymentStatus:  "grace",
				GracePeriodEnd: &pastGrace,
			},
		},
	}

	enforcer := newTestEnforcer(t)
	g := NewGracePeriodEnforcer(store, enforcer, time.Hour)
	g.enforce(context.Background())

	if store.updatedStatuses["org-expired"] != "restricted" {
		t.Errorf("expected restricted status, got %q", store.updatedStatuses["org-expired"])
	}
	if store.updatedPlans["org-expired"] != "free" {
		t.Errorf("expected free plan, got %q", store.updatedPlans["org-expired"])
	}
}

func TestGraceEnforcer_WithinGrace_NoAction(t *testing.T) {
	t.Parallel()

	// ListOrgsInGracePeriod only returns orgs past grace, so an empty list
	// means no orgs need action.
	store := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{},
	}

	g := NewGracePeriodEnforcer(store, nil, time.Hour)
	g.enforce(context.Background())

	if len(store.updatedStatuses) != 0 {
		t.Errorf("expected no status updates, got %d", len(store.updatedStatuses))
	}
	if len(store.updatedPlans) != 0 {
		t.Errorf("expected no plan updates, got %d", len(store.updatedPlans))
	}
}

func TestGraceEnforcer_NoOrgsInGrace_NoOp(t *testing.T) {
	t.Parallel()

	store := &mockGraceEnforcerStore{
		graceOrgs: nil,
	}

	g := NewGracePeriodEnforcer(store, nil, time.Hour)
	g.enforce(context.Background())

	if len(store.updatedStatuses) != 0 {
		t.Errorf("expected no updates, got %d", len(store.updatedStatuses))
	}
}

func TestGraceEnforcer_MultipleOrgs_IndependentProcessing(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	store := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-a", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
			{OrgID: "org-b", PlanTier: "starter", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
		updateStatusErrs: map[string]error{
			"org-a": fmt.Errorf("database error"),
		},
	}

	g := NewGracePeriodEnforcer(store, nil, time.Hour)
	g.enforce(context.Background())

	// org-a should fail, org-b should succeed.
	if _, ok := store.updatedStatuses["org-a"]; ok {
		t.Error("expected org-a status update to be skipped due to error")
	}
	if store.updatedStatuses["org-b"] != "restricted" {
		t.Errorf("expected org-b restricted, got %q", store.updatedStatuses["org-b"])
	}
	if store.updatedPlans["org-b"] != "free" {
		t.Errorf("expected org-b free plan, got %q", store.updatedPlans["org-b"])
	}
}

func TestGraceEnforcer_AtomicRestrictionIneligibleSkipsCacheReset(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	store := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-race", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
		ineligibleOrgs: map[string]bool{"org-race": true},
	}

	enforcer := newTestEnforcer(t)
	g := NewGracePeriodEnforcer(store, enforcer, time.Hour)
	g.enforce(context.Background())

	if len(store.updatedStatuses) != 0 {
		t.Fatalf("expected no status updates when atomic restriction loses the race, got %d", len(store.updatedStatuses))
	}
	if len(store.updatedPlans) != 0 {
		t.Fatalf("expected no plan updates when atomic restriction loses the race, got %d", len(store.updatedPlans))
	}
}

func TestGraceEnforcer_AlreadyRestricted_Skipped(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	store := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{
				OrgID:          "org-already",
				PlanTier:       "free",
				PaymentStatus:  "restricted",
				GracePeriodEnd: &pastGrace,
			},
		},
	}

	g := NewGracePeriodEnforcer(store, nil, time.Hour)
	g.enforce(context.Background())

	// Already restricted orgs should be skipped entirely.
	if len(store.updatedStatuses) != 0 {
		t.Errorf("expected no status updates for already-restricted org, got %d", len(store.updatedStatuses))
	}
	if len(store.updatedPlans) != 0 {
		t.Errorf("expected no plan updates for already-restricted org, got %d", len(store.updatedPlans))
	}
}
