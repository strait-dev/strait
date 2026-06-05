package scheduler

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"strait/internal/billing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	cleanupOps       []string
	projectIDs       []string
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

func (m *mockGraceEnforcerStore) SuspendExcessProjects(_ context.Context, _ string, _ int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupOps = append(m.cleanupOps, "projects")
	return 0, nil
}

func (m *mockGraceEnforcerStore) DeactivateExcessCronJobs(_ context.Context, _ string, _ int) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupOps = append(m.cleanupOps, "cron")
	return nil, nil
}

func (m *mockGraceEnforcerStore) DeactivateExcessWebhookSubscriptions(_ context.Context, _ string, _ int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupOps = append(m.cleanupOps, "webhook")
	return 0, nil
}

func (m *mockGraceEnforcerStore) DeactivateExcessEnvironments(_ context.Context, _ string, _ int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupOps = append(m.cleanupOps, "environment")
	return 0, nil
}

func (m *mockGraceEnforcerStore) DeactivateExcessLogDrains(_ context.Context, _ string, _ int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupOps = append(m.cleanupOps, "log_drain")
	return 0, nil
}

func (m *mockGraceEnforcerStore) DeactivateExcessNotificationChannelsByProject(_ context.Context, projectID string, _ int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupOps = append(m.cleanupOps, "notification:"+projectID)
	return 0, nil
}

func (m *mockGraceEnforcerStore) ListProjectsByOrg(_ context.Context, _ string) ([]string, error) {
	if m.projectIDs != nil {
		return m.projectIDs, nil
	}
	return []string{"project-1"}, nil
}

func (m *mockGraceEnforcerStore) PauseHTTPJobsByOrg(_ context.Context, _ string, _ string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupOps = append(m.cleanupOps, "pause_http")
	return nil, nil
}

func (m *mockGraceEnforcerStore) CountMembersByOrg(_ context.Context, _ string) (int, error) {
	return 0, nil
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
	assert.Equal(t, "restricted",

		store.
			updatedStatuses["org-expired"])
	assert.Equal(t, "free",
		store.
			updatedPlans["org-expired"])

}

func TestGraceEnforcer_PastGrace_EnforcesFreeTierResourceCleanup(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	store := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{
				OrgID:          "org-expired-cleanup",
				PlanTier:       "pro",
				PaymentStatus:  "grace",
				GracePeriodEnd: &pastGrace,
			},
		},
		projectIDs: []string{"project-a", "project-b"},
	}

	g := NewGracePeriodEnforcer(store, nil, time.Hour)
	g.enforce(context.Background())
	require.Equal(t, "restricted",

		store.
			updatedStatuses["org-expired-cleanup"])
	require.Equal(t, "free",
		store.
			updatedPlans["org-expired-cleanup"])

	got := make(map[string]bool)
	for _, op := range store.cleanupOps {
		got[op] = true
	}
	for _, want := range []string{
		"projects",
		"cron",
		"webhook",
		"environment",
		"log_drain",
		"notification:project-a",
		"notification:project-b",
	} {
		require.True(t, got[want])

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
	assert.Len(t, store.
		updatedStatuses,

		0)
	assert.Len(t, store.
		updatedPlans,

		0)

}

func TestGraceEnforcer_NoOrgsInGrace_NoOp(t *testing.T) {
	t.Parallel()

	store := &mockGraceEnforcerStore{
		graceOrgs: nil,
	}

	g := NewGracePeriodEnforcer(store, nil, time.Hour)
	g.enforce(context.Background())
	assert.Len(t, store.
		updatedStatuses,

		0)

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
		assert.Fail(t,

			"expected org-a status update to be skipped due to error")
	}
	assert.Equal(t, "restricted",

		store.
			updatedStatuses["org-b"])
	assert.Equal(t, "free",
		store.
			updatedPlans["org-b"])

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
	require.Len(t, store.
		updatedStatuses,

		0)
	require.Len(t, store.
		updatedPlans,

		0)

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
	assert.Len(t, store.
		updatedStatuses,

		0)
	assert.Len(t, store.
		updatedPlans,

		0)

	// Already restricted orgs should be skipped entirely.

}
