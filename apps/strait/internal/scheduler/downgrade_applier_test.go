package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"strait/internal/billing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type mockDowngradeStore struct {
	pendingOrgs   []billing.OrgSubscription
	appliedOrgIDs []string
	applyErrors   map[string]error
	listErr       error

	// HTTP pause tracking.
	pauseHTTPCalls []pauseHTTPCall
	pauseHTTPCount int64
	pauseHTTPErr   error
}

type pauseHTTPCall struct {
	orgID  string
	reason string
}

func (m *mockDowngradeStore) ListOrgsWithPendingDowngrade(_ context.Context) ([]billing.OrgSubscription, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.pendingOrgs, nil
}

func (m *mockDowngradeStore) ApplyPendingDowngrade(_ context.Context, orgID string) error {
	if m.applyErrors != nil {
		if err, ok := m.applyErrors[orgID]; ok {
			return err
		}
	}
	m.appliedOrgIDs = append(m.appliedOrgIDs, orgID)
	return nil
}

func (m *mockDowngradeStore) SuspendExcessProjects(_ context.Context, _ string, _ int) (int, error) {
	return 0, nil
}

func (m *mockDowngradeStore) DeactivateExcessCronJobs(_ context.Context, _ string, _ int) (int64, error) {
	return 0, nil
}

func (m *mockDowngradeStore) DeactivateExcessWebhookSubscriptions(_ context.Context, _ string, _ int) (int64, error) {
	return 0, nil
}

func (m *mockDowngradeStore) DeactivateExcessEnvironments(_ context.Context, _ string, _ int) (int64, error) {
	return 0, nil
}

func (m *mockDowngradeStore) ListProjectsByOrg(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (m *mockDowngradeStore) PauseHTTPJobsByOrg(_ context.Context, orgID string, reason string) (int64, error) {
	m.pauseHTTPCalls = append(m.pauseHTTPCalls, pauseHTTPCall{orgID: orgID, reason: reason})
	if m.pauseHTTPErr != nil {
		return 0, m.pauseHTTPErr
	}
	return m.pauseHTTPCount, nil
}

func newTestEnforcer(t *testing.T) *billing.Enforcer {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	store := &mockEnforcerStore{}
	return billing.NewEnforcer(store, rdb, slog.Default())
}

// mockEnforcerStore satisfies billing.Store for the enforcer.
type mockEnforcerStore struct{}

func (m *mockEnforcerStore) EnsureOrgSubscription(_ context.Context, _ string) error { return nil }
func (m *mockEnforcerStore) GetOrgSubscription(_ context.Context, _ string) (*billing.OrgSubscription, error) {
	return nil, billing.ErrSubscriptionNotFound
}
func (m *mockEnforcerStore) UpsertOrgSubscription(_ context.Context, _ *billing.OrgSubscription) error {
	return nil
}
func (m *mockEnforcerStore) UpdateOrgSubscriptionPlan(_ context.Context, _, _, _ string) error {
	return nil
}
func (m *mockEnforcerStore) UpdateOrgSubscriptionFull(_ context.Context, _, _, _ string, _, _ *time.Time) error {
	return nil
}
func (m *mockEnforcerStore) UpdateSpendingLimit(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}
func (m *mockEnforcerStore) SetPendingPlanTier(_ context.Context, _, _ string) error { return nil }
func (m *mockEnforcerStore) SetPendingDowngrade(_ context.Context, _, _ string, _, _ *time.Time) error {
	return nil
}
func (m *mockEnforcerStore) ClearPendingPlanTier(_ context.Context, _ string) error  { return nil }
func (m *mockEnforcerStore) ApplyPendingDowngrade(_ context.Context, _ string) error { return nil }
func (m *mockEnforcerStore) ListOrgsWithPendingDowngrade(_ context.Context) ([]billing.OrgSubscription, error) {
	return nil, nil
}
func (m *mockEnforcerStore) GetProjectOrgID(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockEnforcerStore) GetActiveProjectOrgID(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockEnforcerStore) ListProjectsByOrg(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (m *mockEnforcerStore) CountProjectsByOrg(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *mockEnforcerStore) CountMembersByOrg(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *mockEnforcerStore) CountExecutingRunsByOrg(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *mockEnforcerStore) BulkCountExecutingRunsByOrg(_ context.Context, orgIDs []string) (map[string]int, error) {
	return make(map[string]int, len(orgIDs)), nil
}
func (m *mockEnforcerStore) CountAIModelCallsByOrg(_ context.Context, _ string, _, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *mockEnforcerStore) SetProjectOrgID(_ context.Context, _, _ string) error { return nil }
func (m *mockEnforcerStore) UpsertUsageRecord(_ context.Context, _ *billing.UsageRecord) error {
	return nil
}
func (m *mockEnforcerStore) GetOrgUsageForPeriod(_ context.Context, _ string, _, _ time.Time) ([]billing.UsageRecord, error) {
	return nil, nil
}
func (m *mockEnforcerStore) GetProjectUsageForPeriod(_ context.Context, _ string, _, _ time.Time) ([]billing.UsageRecord, error) {
	return nil, nil
}
func (m *mockEnforcerStore) GetOrgDailyUsage(_ context.Context, _ string, _ time.Time) ([]billing.UsageRecord, error) {
	return nil, nil
}
func (m *mockEnforcerStore) SumOrgPeriodSpend(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *mockEnforcerStore) GetProjectBudget(_ context.Context, _ string) (int64, string, error) {
	return 0, "", nil
}
func (m *mockEnforcerStore) SetProjectBudget(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}
func (m *mockEnforcerStore) GetProjectPeriodSpend(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *mockEnforcerStore) UpdateAnomalyThresholds(_ context.Context, _ string, _, _ float64) error {
	return nil
}
func (m *mockEnforcerStore) CountOrgsByUser(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *mockEnforcerStore) UpdatePaymentStatus(_ context.Context, _ string, _ string, _ *time.Time) error {
	return nil
}
func (m *mockEnforcerStore) ListOrgsInGracePeriod(_ context.Context) ([]billing.OrgSubscription, error) {
	return nil, nil
}
func (m *mockEnforcerStore) ListAllSubscribedOrgIDs(_ context.Context) ([]string, error) {
	return nil, nil
}
func (m *mockEnforcerStore) ListStaleSubscriptions(_ context.Context) ([]billing.OrgSubscription, error) {
	return nil, nil
}
func (m *mockEnforcerStore) IsProjectSuspended(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *mockEnforcerStore) SuspendExcessProjects(_ context.Context, _ string, _ int) (int, error) {
	return 0, nil
}

func (m *mockEnforcerStore) ListOrgAdminEmails(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (m *mockEnforcerStore) HasSentUsageReport(_ context.Context, _ string, _ time.Time) (bool, error) {
	return false, nil
}

func (m *mockEnforcerStore) RecordSentUsageReport(_ context.Context, _ string, _ time.Time) error {
	return nil
}

func (m *mockEnforcerStore) UpdateMonthlyUsageEmail(_ context.Context, _ string, _ bool) error {
	return nil
}

func (m *mockEnforcerStore) ListActiveAddons(_ context.Context, _ string) ([]billing.Addon, error) {
	return nil, nil
}

func (m *mockEnforcerStore) CreateAddon(_ context.Context, _ *billing.Addon) error {
	return nil
}

func (m *mockEnforcerStore) DeactivateAddon(_ context.Context, _ string) error {
	return nil
}

func (m *mockEnforcerStore) CountActiveAddonsByType(_ context.Context, _ string, _ billing.AddonType) (int, error) {
	return 0, nil
}

func (m *mockEnforcerStore) RecordProcessedWebhook(_ context.Context, _ string) error {
	return nil
}

func (m *mockEnforcerStore) IsWebhookProcessed(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *mockEnforcerStore) DeleteOldWebhookMessages(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (m *mockEnforcerStore) GetEnterpriseContract(_ context.Context, _ string) (*billing.EnterpriseContract, error) {
	return nil, billing.ErrContractNotFound
}

func (m *mockEnforcerStore) UpsertEnterpriseContract(_ context.Context, _ *billing.EnterpriseContract) error {
	return nil
}

func (m *mockEnforcerStore) ListExpiringContracts(_ context.Context, _ int) ([]billing.EnterpriseContract, error) {
	return nil, nil
}

func (m *mockEnforcerStore) PauseHTTPJobsByOrg(context.Context, string, string) (int64, error) {
	return 0, nil
}

func (m *mockEnforcerStore) UnpauseJobsByPauseReason(context.Context, string, string) (int64, error) {
	return 0, nil
}

func (m *mockEnforcerStore) CountHTTPJobsByOrg(context.Context, string) (int, error) {
	return 0, nil
}

func TestDowngradeApplier_AppliesPastDueDowngrades(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-1", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
			{OrgID: "org-2", PlanTier: "starter", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
	}

	enforcer := newTestEnforcer(t)
	applier := NewDowngradeApplier(store, enforcer, time.Minute)
	applier.apply(context.Background())

	if len(store.appliedOrgIDs) != 2 {
		t.Fatalf("expected 2 downgrades applied, got %d", len(store.appliedOrgIDs))
	}
	if store.appliedOrgIDs[0] != "org-1" || store.appliedOrgIDs[1] != "org-2" {
		t.Errorf("unexpected org IDs: %v", store.appliedOrgIDs)
	}
}

func TestDowngradeApplier_SkipsOrgsNotYetDue(t *testing.T) {
	t.Parallel()

	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{}, // store returns empty (no past-due orgs)
	}

	applier := NewDowngradeApplier(store, nil, time.Minute)
	applier.apply(context.Background())

	if len(store.appliedOrgIDs) != 0 {
		t.Fatalf("expected 0 downgrades applied, got %d", len(store.appliedOrgIDs))
	}
}

func TestDowngradeApplier_ContinuesOnSingleOrgError(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-fail", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
			{OrgID: "org-ok", PlanTier: "starter", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
		applyErrors: map[string]error{
			"org-fail": fmt.Errorf("database error"),
		},
	}

	applier := NewDowngradeApplier(store, nil, time.Minute)
	applier.apply(context.Background())

	if len(store.appliedOrgIDs) != 1 {
		t.Fatalf("expected 1 successful downgrade, got %d", len(store.appliedOrgIDs))
	}
	if store.appliedOrgIDs[0] != "org-ok" {
		t.Errorf("expected org-ok to succeed, got %q", store.appliedOrgIDs[0])
	}
}

func TestDowngradeApplier_NilEnforcer(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-1", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
	}

	applier := NewDowngradeApplier(store, nil, time.Minute)
	applier.apply(context.Background()) // should not panic

	if len(store.appliedOrgIDs) != 1 {
		t.Fatalf("expected 1 downgrade, got %d", len(store.appliedOrgIDs))
	}
}

func TestDowngradeApplier_SkipsHTTPPauseWhenNewPlanAllows(t *testing.T) {
	t.Parallel()

	// Enterprise -> Pro: both allow HTTP mode, so no pause should happen.
	pro := "pro"
	pastEnd := time.Now().Add(-1 * time.Hour)
	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-keep", PlanTier: "enterprise", PendingPlanTier: &pro, CurrentPeriodEnd: &pastEnd},
		},
	}

	enforcer := newTestEnforcer(t)
	applier := NewDowngradeApplier(store, enforcer, time.Minute)
	applier.apply(context.Background())

	if len(store.appliedOrgIDs) != 1 {
		t.Fatalf("expected 1 downgrade applied, got %d", len(store.appliedOrgIDs))
	}
	if len(store.pauseHTTPCalls) != 0 {
		t.Errorf("expected 0 PauseHTTPJobsByOrg calls, got %d", len(store.pauseHTTPCalls))
	}
}
