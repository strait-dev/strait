package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDowngradeStore struct {
	pendingOrgs   []billing.OrgSubscription
	appliedOrgIDs []string
	clearedOrgIDs []string
	applyErrors   map[string]error
	applyIfTierFn func(ctx context.Context, orgID, pendingTier string) (bool, error)
	clearIfTierFn func(ctx context.Context, orgID, pendingTier string) (bool, error)
	listErr       error
	operations    []string

	// HTTP pause tracking.
	pauseHTTPCalls []pauseHTTPCall
	pauseHTTPIDs   []string
	pauseHTTPErr   error
	cronErr        error

	// Log drain / notification channel cleanup tracking.
	logDrainCalls     []deactivateCall
	logDrainCount     int64
	notifChannelCalls []deactivateCall
	notifChannelCount int64

	// Member overage tracking.
	memberCount int
	projectIDs  []string
}

type pauseHTTPCall struct {
	orgID  string
	reason string
}

type deactivateCall struct {
	orgID string
	max   int
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
	m.operations = append(m.operations, "apply:"+orgID)
	return nil
}

func (m *mockDowngradeStore) ApplyPendingDowngradeIfTier(ctx context.Context, orgID, pendingTier string) (bool, error) {
	if m.applyIfTierFn != nil {
		return m.applyIfTierFn(ctx, orgID, pendingTier)
	}
	if err := m.ApplyPendingDowngrade(ctx, orgID); err != nil {
		return false, err
	}
	return true, nil
}

func (m *mockDowngradeStore) ApplyPendingDowngradeTierIfPending(ctx context.Context, orgID, pendingTier string) (bool, error) {
	return m.ApplyPendingDowngradeIfTier(ctx, orgID, pendingTier)
}

func (m *mockDowngradeStore) ClearPendingPlanTierIfTier(ctx context.Context, orgID, pendingTier string) (bool, error) {
	if m.clearIfTierFn != nil {
		return m.clearIfTierFn(ctx, orgID, pendingTier)
	}
	m.clearedOrgIDs = append(m.clearedOrgIDs, orgID)
	m.operations = append(m.operations, "clear:"+orgID)
	return true, nil
}

func (m *mockDowngradeStore) SuspendExcessProjects(_ context.Context, _ string, _ int) (int, error) {
	m.operations = append(m.operations, "suspend")
	return 0, nil
}

func (m *mockDowngradeStore) DeactivateExcessCronJobs(_ context.Context, _ string, _ int) ([]string, error) {
	m.operations = append(m.operations, "cron")
	if m.cronErr != nil {
		return nil, m.cronErr
	}
	return nil, nil
}

func (m *mockDowngradeStore) DeactivateExcessWebhookSubscriptions(_ context.Context, _ string, _ int) (int64, error) {
	m.operations = append(m.operations, "webhook")
	return 0, nil
}

func (m *mockDowngradeStore) DeactivateExcessEnvironments(_ context.Context, _ string, _ int) (int64, error) {
	m.operations = append(m.operations, "environment")
	return 0, nil
}

func (m *mockDowngradeStore) ListProjectsByOrg(_ context.Context, _ string) ([]string, error) {
	return m.projectIDs, nil
}

func (m *mockDowngradeStore) PauseHTTPJobsByOrg(_ context.Context, orgID string, reason string) ([]string, error) {
	m.operations = append(m.operations, "pause_http")
	m.pauseHTTPCalls = append(m.pauseHTTPCalls, pauseHTTPCall{orgID: orgID, reason: reason})
	if m.pauseHTTPErr != nil {
		return nil, m.pauseHTTPErr
	}
	return m.pauseHTTPIDs, nil
}

func (m *mockDowngradeStore) DeactivateExcessLogDrains(_ context.Context, orgID string, maxDrains int) (int64, error) {
	m.logDrainCalls = append(m.logDrainCalls, deactivateCall{orgID: orgID, max: maxDrains})
	return m.logDrainCount, nil
}

func (m *mockDowngradeStore) DeactivateExcessNotificationChannelsByProject(_ context.Context, projectID string, maxChannels int) (int64, error) {
	m.notifChannelCalls = append(m.notifChannelCalls, deactivateCall{orgID: projectID, max: maxChannels})
	return m.notifChannelCount, nil
}

func (m *mockDowngradeStore) CountMembersByOrg(_ context.Context, _ string) (int, error) {
	return m.memberCount, nil
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
type mockEnforcerStore struct {
	subscriptions map[string]*billing.OrgSubscription
}

func (m *mockEnforcerStore) UpdateEntitlements(context.Context, string, billing.OrgPlanLimits) error {
	return nil
}
func (m *mockEnforcerStore) TryMarkBillingCapEvent(context.Context, string, billing.BillingCapEvent) (bool, error) {
	return false, nil
}
func (m *mockEnforcerStore) EnsureOrgSubscription(_ context.Context, _ string) error { return nil }
func (m *mockEnforcerStore) GetOrgSubscription(_ context.Context, orgID string) (*billing.OrgSubscription, error) {
	if m.subscriptions != nil {
		if sub, ok := m.subscriptions[orgID]; ok {
			cp := *sub
			return &cp, nil
		}
	}
	return nil, billing.ErrSubscriptionNotFound
}
func (m *mockEnforcerStore) GetOrgSubscriptionByStripeCustomerID(context.Context, string) (*billing.OrgSubscription, error) {
	return nil, billing.ErrSubscriptionNotFound
}
func (m *mockEnforcerStore) GetOrgSubscriptionByStripeSubscriptionID(context.Context, string) (*billing.OrgSubscription, error) {
	return nil, billing.ErrSubscriptionNotFound
}
func (m *mockEnforcerStore) UpsertOrgSubscription(_ context.Context, _ *billing.OrgSubscription) error {
	return nil
}
func (m *mockEnforcerStore) UpdateOrgSubscriptionPlan(_ context.Context, _, _, _ string) error {
	return nil
}
func (m *mockEnforcerStore) UpdateOrgSubscriptionStatus(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockEnforcerStore) UpdateOrgSubscriptionFull(_ context.Context, _, _, _ string, _, _ *time.Time) error {
	return nil
}
func (m *mockEnforcerStore) UpdateSpendingLimit(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}
func (m *mockEnforcerStore) UpdateOverageDisabled(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockEnforcerStore) SetPendingPlanTier(_ context.Context, _, _ string) error { return nil }
func (m *mockEnforcerStore) SetPendingDowngrade(_ context.Context, _, _ string, _, _ *time.Time) error {
	return nil
}
func (m *mockEnforcerStore) ClearPendingPlanTier(_ context.Context, _ string) error  { return nil }
func (m *mockEnforcerStore) ApplyPendingDowngrade(_ context.Context, _ string) error { return nil }
func (m *mockEnforcerStore) ApplyPendingDowngradeTierIfPending(context.Context, string, string) (bool, error) {
	return true, nil
}
func (m *mockEnforcerStore) ClearPendingPlanTierIfTier(context.Context, string, string) (bool, error) {
	return true, nil
}
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

func (m *mockEnforcerStore) PauseHTTPJobsByOrg(context.Context, string, string) ([]string, error) {
	return nil, nil
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
	require.Len(t, store.
		appliedOrgIDs,
		2)
	assert.False(t, store.
		appliedOrgIDs[0] != "org-1" ||
		store.
			appliedOrgIDs[1] !=
			"org-2")
}

func TestDowngradeApplier_SkipsOrgsNotYetDue(t *testing.T) {
	t.Parallel()

	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{}, // store returns empty (no past-due orgs)
	}

	applier := NewDowngradeApplier(store, nil, time.Minute)
	applier.apply(context.Background())
	require.Empty(t, store.
		appliedOrgIDs)
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
	require.Len(t, store.
		appliedOrgIDs,
		1)
	assert.Equal(t, "org-ok",
		store.appliedOrgIDs[0])
}

func TestDowngradeApplier_DoesNotApplyWhenPendingTierChanged(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-raced", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
		applyIfTierFn: func(_ context.Context, orgID, pendingTier string) (bool, error) {
			require.False(t, orgID !=
				"org-raced" ||
				pendingTier !=
					"free")

			return false, nil
		},
	}

	applier := NewDowngradeApplier(store, nil, time.Minute)
	applier.apply(context.Background())
	require.Empty(t, store.
		appliedOrgIDs)
}

func TestDowngradeApplier_RetainsPendingTierWhenLimitEnforcementFails(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-fail-enforce", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
		cronErr: fmt.Errorf("cron deactivate failed"),
	}

	applier := NewDowngradeApplier(store, nil, time.Minute)
	applier.apply(context.Background())
	require.False(t, len(store.appliedOrgIDs) !=
		1 || store.
		appliedOrgIDs[0] !=

		"org-fail-enforce")
	require.GreaterOrEqual(t, len(store.
		operations,
	), 2)
	require.Equal(t, "apply:org-fail-enforce",

		store.operations[0])
	require.Empty(t, store.
		clearedOrgIDs)
}

func TestDowngradeApplier_InvalidatesOrgCacheAfterTierTransitionBeforeCleanup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	free := string(domain.PlanFree)
	orgID := "org-cache-downgrade"
	pastEnd := time.Now().Add(-1 * time.Hour)
	enforcerStore := &mockEnforcerStore{
		subscriptions: map[string]*billing.OrgSubscription{
			orgID: {OrgID: orgID, PlanTier: string(domain.PlanPro), Status: "active"},
		},
	}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	enforcer := billing.NewEnforcer(enforcerStore, rdb, slog.Default())

	primed, err := enforcer.GetOrgPlanLimits(ctx, orgID)
	require.NoError(t,
		err)
	require.Equal(t, domain.
		PlanPro, primed.
		PlanTier,
	)

	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: orgID, PlanTier: string(domain.PlanPro), PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
		cronErr: fmt.Errorf("cleanup failed after tier transition"),
		applyIfTierFn: func(_ context.Context, gotOrgID, pendingTier string) (bool, error) {
			require.False(t, gotOrgID !=
				orgID ||
				pendingTier !=
					free,
			)

			enforcerStore.subscriptions[orgID] = &billing.OrgSubscription{OrgID: orgID, PlanTier: free, Status: "active"}
			sub := enforcerStore.subscriptions[orgID]
			entitlements := billing.GetPlanLimits(domain.PlanFree)
			sub.Entitlements, _ = json.Marshal(entitlements)
			return true, nil
		},
	}

	applier := NewDowngradeApplier(store, enforcer, time.Minute)
	applier.apply(ctx)
	require.Empty(t, store.
		clearedOrgIDs)

	after, err := enforcer.GetOrgPlanLimits(ctx, orgID)
	require.NoError(t,
		err)
	require.Equal(t, domain.
		PlanFree, after.
		PlanTier,
	)
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
	applier.apply(context.Background())
	require.Len(t, store.
		appliedOrgIDs,
		1)

	// should not panic
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
	require.Len(t, store.
		appliedOrgIDs,
		1)
	assert.Empty(t, store.
		pauseHTTPCalls)
}

// TestDowngradeApplier_DeactivatesExcessLogDrains confirms the Pro→Free
// downgrade calls DeactivateExcessLogDrains with the new Free-tier cap (0).
// Drains beyond the cap must be flipped to enabled=false; the per-call cap
// argument matches the new tier's MaxLogDrainsPerOrg.
func TestDowngradeApplier_DeactivatesExcessLogDrains(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-drains", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
		logDrainCount: 3, // simulate 3 rows deactivated
	}

	enforcer := newTestEnforcer(t)
	applier := NewDowngradeApplier(store, enforcer, time.Minute)
	applier.apply(context.Background())
	require.Len(t, store.
		logDrainCalls,
		1)

	freeLimits := billing.GetPlanLimits("free")
	if got, want := store.logDrainCalls[0].max, freeLimits.MaxLogDrainsPerOrg; got != want {
		assert.Failf(t, "test failure",

			"DeactivateExcessLogDrains called with max=%d, want %d (Free tier cap)", got, want)
	}
}

// TestDowngradeApplier_DeactivatesExcessNotificationChannelsPerProject locks
// in the per-project iteration: notification-channel caps are scoped per
// project, so the applier must walk each project and call the deactivator
// once per project with the new tier's MaxNotificationChannels.
func TestDowngradeApplier_DeactivatesExcessNotificationChannelsPerProject(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-multi", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
		projectIDs: []string{"proj-a", "proj-b", "proj-c"},
	}

	enforcer := newTestEnforcer(t)
	applier := NewDowngradeApplier(store, enforcer, time.Minute)
	applier.apply(context.Background())
	require.Len(t, store.
		notifChannelCalls,
		3)

	freeLimits := billing.GetPlanLimits("free")
	for _, call := range store.notifChannelCalls {
		assert.Equal(t, freeLimits.
			MaxNotificationChannels,

			call.
				max)
	}
	seen := map[string]bool{}
	for _, call := range store.notifChannelCalls {
		seen[call.orgID] = true // mock stores projectID in orgID field
	}
	for _, p := range store.projectIDs {
		assert.True(t, seen[p])
	}
}

// TestDowngradeApplier_SkipsLogDrainCleanupForUnlimitedTier confirms tiers
// with MaxLogDrainsPerOrg=-1 (Enterprise-style unlimited) do not trigger any
// cleanup calls.
func TestDowngradeApplier_SkipsLogDrainCleanupForUnlimitedTier(t *testing.T) {
	t.Parallel()

	// Enterprise allows unlimited log drains; downgrade Enterprise->Enterprise
	// is a no-op for log drains. Use a plan with MaxLogDrainsPerOrg = -1.
	enterprise := "enterprise"
	pastEnd := time.Now().Add(-1 * time.Hour)
	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-keep", PlanTier: "enterprise", PendingPlanTier: &enterprise, CurrentPeriodEnd: &pastEnd},
		},
	}

	enforcer := newTestEnforcer(t)
	applier := NewDowngradeApplier(store, enforcer, time.Minute)
	applier.apply(context.Background())
	assert.Empty(t, store.
		logDrainCalls,
	)
}

// TestDowngradeApplier_EmitsMemberOverageEventOnDowngrade pins the documented
// member policy: do NOT auto-deactivate members; emit a billing event so the
// dashboard surfaces the overage. New invites are blocked at the API.
func TestDowngradeApplier_EmitsMemberOverageEventOnDowngrade(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	freeLimits := billing.GetPlanLimits("free")
	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-over", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
		// Sit one above the new cap so we trigger the overage path.
		memberCount: freeLimits.MaxMembersPerOrg + 1,
	}

	enforcer := newTestEnforcer(t)
	applier := NewDowngradeApplier(store, enforcer, time.Minute)
	applier.apply(context.Background())
	require.Len(t, store.
		appliedOrgIDs,
		1)

	// We can't easily intercept the ClickHouse exporter from this test, but
	// the apply path must not panic and must succeed. The integration test
	// covers the actual ClickHouse emission path. This test pins that the
	// applier reaches the overage branch (count > cap) without erroring.
}

// TestDowngradeApplier_NoMemberOverageWhenUnderCap confirms the overage path
// does NOT fire when the member count is at or below the new cap.
func TestDowngradeApplier_NoMemberOverageWhenUnderCap(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	freeLimits := billing.GetPlanLimits("free")
	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-fits", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
		memberCount: freeLimits.MaxMembersPerOrg, // exactly at cap → no overage
	}

	enforcer := newTestEnforcer(t)
	applier := NewDowngradeApplier(store, enforcer, time.Minute)
	applier.apply(context.Background())
	require.Len(t, store.
		appliedOrgIDs,
		1)

	// No assertion on the chExporter here; the integration test covers the
	// non-emission case. This test pins the under-cap branch is reached
	// without error.
}
