package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAnomalyMonitorStore implements AnomalyMonitorStore with function callbacks
// for key methods and zero-value stubs for the rest of billing.Store.
type mockAnomalyMonitorStore struct {
	listAllSubscribedOrgIDsFn         func(ctx context.Context) ([]string, error)
	getOrgSubscriptionFn              func(ctx context.Context, orgID string) (*billing.OrgSubscription, error)
	getOrgUsageForPeriodFn            func(ctx context.Context, orgID string, from, to time.Time) ([]billing.UsageRecord, error)
	listProjectsByOrgFn               func(ctx context.Context, orgID string) ([]string, error)
	listEnabledNotificationChannelsFn func(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	createNotificationDeliveryFn      func(ctx context.Context, d *domain.NotificationDelivery) error
}

// Key methods with callbacks.

func (m *mockAnomalyMonitorStore) ListAllSubscribedOrgIDs(ctx context.Context) ([]string, error) {
	if m.listAllSubscribedOrgIDsFn != nil {
		return m.listAllSubscribedOrgIDsFn(ctx)
	}
	return nil, nil
}

func (m *mockAnomalyMonitorStore) GetOrgSubscription(ctx context.Context, orgID string) (*billing.OrgSubscription, error) {
	if m.getOrgSubscriptionFn != nil {
		return m.getOrgSubscriptionFn(ctx, orgID)
	}
	return nil, nil
}

func (m *mockAnomalyMonitorStore) GetOrgSubscriptionByStripeCustomerID(context.Context, string) (*billing.OrgSubscription, error) {
	return nil, billing.ErrSubscriptionNotFound
}

func (m *mockAnomalyMonitorStore) GetOrgSubscriptionByStripeSubscriptionID(context.Context, string) (*billing.OrgSubscription, error) {
	return nil, billing.ErrSubscriptionNotFound
}

func (m *mockAnomalyMonitorStore) GetOrgUsageForPeriod(ctx context.Context, orgID string, from, to time.Time) ([]billing.UsageRecord, error) {
	if m.getOrgUsageForPeriodFn != nil {
		return m.getOrgUsageForPeriodFn(ctx, orgID, from, to)
	}
	return nil, nil
}

func (m *mockAnomalyMonitorStore) ListProjectsByOrg(ctx context.Context, orgID string) ([]string, error) {
	if m.listProjectsByOrgFn != nil {
		return m.listProjectsByOrgFn(ctx, orgID)
	}
	return nil, nil
}

func (m *mockAnomalyMonitorStore) ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error) {
	if m.listEnabledNotificationChannelsFn != nil {
		return m.listEnabledNotificationChannelsFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockAnomalyMonitorStore) ListEnabledNotificationChannelsByProjectIDs(ctx context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error) {
	result := make(map[string][]domain.NotificationChannel)
	for _, pid := range projectIDs {
		channels, err := m.ListEnabledNotificationChannels(ctx, pid)
		if err != nil {
			return nil, err
		}
		if len(channels) > 0 {
			result[pid] = channels
		}
	}
	return result, nil
}

func (m *mockAnomalyMonitorStore) CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error {
	if m.createNotificationDeliveryFn != nil {
		return m.createNotificationDeliveryFn(ctx, d)
	}
	return nil
}

// Stub methods for the rest of billing.Store.

func (m *mockAnomalyMonitorStore) UpdateEntitlements(context.Context, string, billing.OrgPlanLimits) error {
	return nil
}
func (m *mockAnomalyMonitorStore) EnsureOrgSubscription(context.Context, string) error { return nil }
func (m *mockAnomalyMonitorStore) UpsertOrgSubscription(context.Context, *billing.OrgSubscription) error {
	return nil
}
func (m *mockAnomalyMonitorStore) UpdateOrgSubscriptionPlan(context.Context, string, string, string) error {
	return nil
}
func (m *mockAnomalyMonitorStore) UpdateOrgSubscriptionStatus(context.Context, string, string) error {
	return nil
}
func (m *mockAnomalyMonitorStore) UpdateOrgSubscriptionFull(context.Context, string, string, string, *time.Time, *time.Time) error {
	return nil
}
func (m *mockAnomalyMonitorStore) UpdateSpendingLimit(context.Context, string, int64, string) error {
	return nil
}
func (m *mockAnomalyMonitorStore) UpdateOverageDisabled(context.Context, string, bool) error {
	return nil
}
func (m *mockAnomalyMonitorStore) SetPendingPlanTier(context.Context, string, string) error {
	return nil
}
func (m *mockAnomalyMonitorStore) SetPendingDowngrade(context.Context, string, string, *time.Time, *time.Time) error {
	return nil
}
func (m *mockAnomalyMonitorStore) ClearPendingPlanTier(context.Context, string) error  { return nil }
func (m *mockAnomalyMonitorStore) ApplyPendingDowngrade(context.Context, string) error { return nil }
func (m *mockAnomalyMonitorStore) ListOrgsWithPendingDowngrade(context.Context) ([]billing.OrgSubscription, error) {
	return nil, nil
}
func (m *mockAnomalyMonitorStore) GetProjectOrgID(context.Context, string) (string, error) {
	return "", nil
}
func (m *mockAnomalyMonitorStore) GetActiveProjectOrgID(context.Context, string) (string, error) {
	return "", nil
}
func (m *mockAnomalyMonitorStore) CountProjectsByOrg(context.Context, string) (int, error) {
	return 0, nil
}
func (m *mockAnomalyMonitorStore) CountMembersByOrg(context.Context, string) (int, error) {
	return 0, nil
}
func (m *mockAnomalyMonitorStore) CountOrgsByUser(context.Context, string) (int, error) {
	return 0, nil
}
func (m *mockAnomalyMonitorStore) CountExecutingRunsByOrg(context.Context, string) (int, error) {
	return 0, nil
}
func (m *mockAnomalyMonitorStore) BulkCountExecutingRunsByOrg(_ context.Context, orgIDs []string) (map[string]int, error) {
	return make(map[string]int, len(orgIDs)), nil
}
func (m *mockAnomalyMonitorStore) SetProjectOrgID(context.Context, string, string) error {
	return nil
}
func (m *mockAnomalyMonitorStore) UpsertUsageRecord(context.Context, *billing.UsageRecord) error {
	return nil
}
func (m *mockAnomalyMonitorStore) GetProjectUsageForPeriod(_ context.Context, _ string, _, _ time.Time) ([]billing.UsageRecord, error) {
	return nil, nil
}
func (m *mockAnomalyMonitorStore) GetOrgDailyUsage(_ context.Context, _ string, _ time.Time) ([]billing.UsageRecord, error) {
	return nil, nil
}
func (m *mockAnomalyMonitorStore) SumOrgPeriodSpend(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *mockAnomalyMonitorStore) GetProjectBudget(context.Context, string) (int64, string, error) {
	return 0, "", nil
}
func (m *mockAnomalyMonitorStore) SetProjectBudget(context.Context, string, int64, string) error {
	return nil
}
func (m *mockAnomalyMonitorStore) GetProjectPeriodSpend(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *mockAnomalyMonitorStore) UpdateAnomalyThresholds(context.Context, string, float64, float64) error {
	return nil
}
func (m *mockAnomalyMonitorStore) TryMarkBillingCapEvent(context.Context, string, billing.BillingCapEvent) (bool, error) {
	return false, nil
}
func (m *mockAnomalyMonitorStore) UpdatePaymentStatus(context.Context, string, string, *time.Time) error {
	return nil
}
func (m *mockAnomalyMonitorStore) ListOrgsInGracePeriod(context.Context) ([]billing.OrgSubscription, error) {
	return nil, nil
}
func (m *mockAnomalyMonitorStore) ListStaleSubscriptions(context.Context) ([]billing.OrgSubscription, error) {
	return nil, nil
}
func (m *mockAnomalyMonitorStore) IsProjectSuspended(context.Context, string) (bool, error) {
	return false, nil
}
func (m *mockAnomalyMonitorStore) SuspendExcessProjects(context.Context, string, int) (int, error) {
	return 0, nil
}

func (m *mockAnomalyMonitorStore) ListOrgAdminEmails(context.Context, string) ([]string, error) {
	return nil, nil
}

func (m *mockAnomalyMonitorStore) HasSentUsageReport(context.Context, string, time.Time) (bool, error) {
	return false, nil
}

func (m *mockAnomalyMonitorStore) RecordSentUsageReport(context.Context, string, time.Time) error {
	return nil
}

func (m *mockAnomalyMonitorStore) UpdateMonthlyUsageEmail(context.Context, string, bool) error {
	return nil
}

func (m *mockAnomalyMonitorStore) ListActiveAddons(context.Context, string) ([]billing.Addon, error) {
	return nil, nil
}

func (m *mockAnomalyMonitorStore) CreateAddon(context.Context, *billing.Addon) error {
	return nil
}

func (m *mockAnomalyMonitorStore) DeactivateAddon(context.Context, string) error {
	return nil
}

func (m *mockAnomalyMonitorStore) CountActiveAddonsByType(context.Context, string, billing.AddonType) (int, error) {
	return 0, nil
}

func (m *mockAnomalyMonitorStore) RecordProcessedWebhook(context.Context, string) error {
	return nil
}

func (m *mockAnomalyMonitorStore) IsWebhookProcessed(context.Context, string) (bool, error) {
	return false, nil
}

func (m *mockAnomalyMonitorStore) DeleteOldWebhookMessages(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func (m *mockAnomalyMonitorStore) GetEnterpriseContract(context.Context, string) (*billing.EnterpriseContract, error) {
	return nil, billing.ErrContractNotFound
}

func (m *mockAnomalyMonitorStore) UpsertEnterpriseContract(context.Context, *billing.EnterpriseContract) error {
	return nil
}

func (m *mockAnomalyMonitorStore) ListExpiringContracts(context.Context, int) ([]billing.EnterpriseContract, error) {
	return nil, nil
}

func (m *mockAnomalyMonitorStore) PauseHTTPJobsByOrg(context.Context, string, string) ([]string, error) {
	return nil, nil
}

func (m *mockAnomalyMonitorStore) UnpauseJobsByPauseReason(context.Context, string, string) (int64, error) {
	return 0, nil
}

func (m *mockAnomalyMonitorStore) CountHTTPJobsByOrg(context.Context, string) (int, error) {
	return 0, nil
}

// mockCooldown implements AnomalyCooldown for testing.
type mockCooldown struct {
	cooled map[string]bool
}

func newMockCooldown() *mockCooldown {
	return &mockCooldown{cooled: make(map[string]bool)}
}

func (c *mockCooldown) InCooldown(_ context.Context, orgID string) (bool, error) {
	return c.cooled[orgID], nil
}

func (c *mockCooldown) SetCooldown(_ context.Context, orgID string) error {
	c.cooled[orgID] = true
	return nil
}

// Helpers.

// buildSpikeUsage creates 7 days of historical usage at baseSpend each plus
// today's usage at todaySpend. Each day has a single record for the given org
// and project.
func buildSpikeUsage(orgID, projectID string, baseSpend, todaySpend int64) []billing.UsageRecord {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	records := make([]billing.UsageRecord, 0, 8)
	for i := 7; i >= 1; i-- {
		records = append(records, billing.UsageRecord{
			OrgID:            orgID,
			ProjectID:        projectID,
			PeriodDate:       today.AddDate(0, 0, -i),
			ComputeCostMicro: baseSpend,
		})
	}
	records = append(records, billing.UsageRecord{
		OrgID:            orgID,
		ProjectID:        projectID,
		PeriodDate:       today,
		ComputeCostMicro: todaySpend,
	})
	return records
}

func defaultOrgSub(orgID string) *billing.OrgSubscription {
	return &billing.OrgSubscription{
		OrgID:                    orgID,
		AnomalyThresholdWarning:  3.0,
		AnomalyThresholdCritical: 10.0,
	}
}

// Tests.

func TestAnomalyMonitor_SpikeDetected_AlertFires(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return defaultOrgSub("org-1"), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			return buildSpikeUsage(orgID, "proj-1", 1000, 5000), nil // 5x spike
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
	require.Len(t, deliveries,
		1)
	assert.Equal(t, domain.
		NotificationEventCostAnomaly,

		deliveries[0].
			EventType)
}

func TestAnomalyMonitor_NoSpike_NoAlert(t *testing.T) {
	t.Parallel()

	deliveryCalled := false
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return defaultOrgSub("org-1"), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			// 1x ratio -- no spike
			return buildSpikeUsage(orgID, "proj-1", 1000, 1000), nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
	require.False(t, deliveryCalled)
}

func TestAnomalyMonitor_Cooldown_SkipsRecentlyAlerted(t *testing.T) {
	t.Parallel()

	var deliveryCount int
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return defaultOrgSub("org-1"), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			return buildSpikeUsage(orgID, "proj-1", 1000, 5000), nil
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCount++
			return nil
		},
	}

	cd := newMockCooldown()
	am := NewAnomalyMonitor(s, time.Minute).WithCooldown(cd)

	// First check fires alert and sets cooldown.
	am.check(context.Background())
	// Second check should skip because cooldown is active.
	am.check(context.Background())
	require.Equal(t, 1,
		deliveryCount)
}

func TestAnomalyMonitor_DefaultCooldownDeduplicatesTicks(t *testing.T) {
	t.Parallel()

	var deliveryCount int
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-default-cooldown"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return defaultOrgSub("org-default-cooldown"), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			return buildSpikeUsage(orgID, "proj-1", 1000, 5000), nil
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCount++
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
	am.check(context.Background())
	require.Equal(t, 1,
		deliveryCount)
}

func TestAnomalyMonitor_Cooldown_AlertsAfter4Hours(t *testing.T) {
	t.Parallel()

	var deliveryCount int
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return defaultOrgSub("org-1"), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			return buildSpikeUsage(orgID, "proj-1", 1000, 5000), nil
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCount++
			return nil
		},
	}

	cd := newMockCooldown()
	am := NewAnomalyMonitor(s, time.Minute).WithCooldown(cd)

	// First check fires alert.
	am.check(context.Background())

	// Simulate cooldown expiry by clearing it.
	cd.cooled = make(map[string]bool)

	// Second check should fire again after cooldown expires.
	am.check(context.Background())
	require.Equal(t, 2,
		deliveryCount)
}

func TestAnomalyMonitor_CooldownKey_PerOrg(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1", "org-2"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, orgID string) (*billing.OrgSubscription, error) {
			return defaultOrgSub(orgID), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			return buildSpikeUsage(orgID, "proj-"+orgID, 1000, 5000), nil
		},
		listProjectsByOrgFn: func(_ context.Context, orgID string) ([]string, error) {
			return []string{"proj-" + orgID}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, projectID string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-" + projectID, ProjectID: projectID}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	cd := newMockCooldown()
	am := NewAnomalyMonitor(s, time.Minute).WithCooldown(cd)

	// First check: both orgs alert.
	am.check(context.Background())
	require.Len(t, deliveries,
		2)

	// Second check: both should be in cooldown.
	am.check(context.Background())
	require.Len(t, deliveries,
		2)

	// Clear cooldown for org-1 only.
	delete(cd.cooled, "org-1")

	// Third check: only org-1 should fire.
	am.check(context.Background())
	require.Len(t, deliveries,
		3)
}

func TestAnomalyMonitor_WarningAt3x(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return defaultOrgSub("org-1"), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			// Exactly 3x spike -- should trigger warning.
			return buildSpikeUsage(orgID, "proj-1", 1000, 3000), nil
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
	require.Len(t, deliveries,
		1)

	var payload map[string]any
	require.NoError(t,
		json.Unmarshal(deliveries[0].Payload,
			&payload,
		),
	)
	assert.Equal(t, string(billing.AnomalySeverityWarning), payload["severity"])
}

func TestAnomalyMonitor_ZeroAverage_Skipped(t *testing.T) {
	t.Parallel()

	deliveryCalled := false
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return defaultOrgSub("org-1"), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			// 7 days of zero spend + today has spend. avg7d = 0, so no ratio.
			return buildSpikeUsage(orgID, "proj-1", 0, 5000), nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
	require.False(t, deliveryCalled)
}

func TestAnomalyMonitor_NoHistorySkipped(t *testing.T) {
	t.Parallel()

	deliveryCalled := false
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return defaultOrgSub("org-1"), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, _ string, _, _ time.Time) ([]billing.UsageRecord, error) {
			// Only today's data, no 7-day history.
			now := time.Now().UTC()
			today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			return []billing.UsageRecord{
				{OrgID: "org-1", ProjectID: "proj-1", PeriodDate: today, ComputeCostMicro: 5000},
			}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
	require.False(t, deliveryCalled)
}

func TestAnomalyMonitor_NoOrgsWithActivity_NoOp(t *testing.T) {
	t.Parallel()

	deliveryCalled := false
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return nil, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
	require.False(t, deliveryCalled)
}

func TestAnomalyMonitor_StoreError_LogsContinues(t *testing.T) {
	t.Parallel()

	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return nil, errors.New("db down")
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	// Should not panic.
	am.check(context.Background())
}

func TestAnomalyMonitor_MultipleOrgs_IndependentAlerts(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1", "org-2"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, orgID string) (*billing.OrgSubscription, error) {
			return defaultOrgSub(orgID), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			switch orgID {
			case "org-1":
				return buildSpikeUsage(orgID, "proj-1", 1000, 5000), nil // 5x spike
			case "org-2":
				return buildSpikeUsage(orgID, "proj-2", 1000, 1000), nil // no spike
			}
			return nil, nil
		},
		listProjectsByOrgFn: func(_ context.Context, orgID string) ([]string, error) {
			switch orgID {
			case "org-1":
				return []string{"proj-1"}, nil
			case "org-2":
				return []string{"proj-2"}, nil
			}
			return nil, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, projectID string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-" + projectID, ProjectID: projectID}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
	require.Len(t, deliveries,
		1)
	assert.Equal(t, "proj-1",
		deliveries[0].ProjectID,
	)
}

func TestAnomalyMonitor_Run_StopsOnContextCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	s := &mockAnomalyMonitorStore{}
	am := NewAnomalyMonitor(s, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		am.Run(ctx)
		close(done)
	})

	cancel()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		require.Fail(t, "Run did not stop on context cancel")
	}
}

func TestAnomalyMonitor_Run_ChecksOnInterval(t *testing.T) {
	t.Parallel()

	checkCh := make(chan struct{}, 10)
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			select {
			case checkCh <- struct{}{}:
			default:
			}
			return nil, nil
		},
	}

	am := NewAnomalyMonitor(s, 20*time.Millisecond)

	go am.Run(t.Context())

	deadline := time.After(2 * time.Second)
	for i := range 2 {
		select {
		case <-checkCh:
		case <-deadline:
			require.Failf(t, "test failure", "timed out waiting for check %d", i+1)
		}
	}
}

func TestAnomalyMonitor_NotificationDeliveryCreated(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return defaultOrgSub("org-1"), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			return buildSpikeUsage(orgID, "proj-1", 1000, 5000), nil
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
	require.Len(t, deliveries,
		1)

	d := deliveries[0]
	assert.Equal(t, domain.
		NotificationEventCostAnomaly,

		d.EventType,
	)
	assert.Equal(t, "ch-1",
		d.ChannelID)
	assert.Equal(t, "proj-1",
		d.ProjectID,
	)
	assert.Equal(t, "pending",
		d.Status)
	assert.Equal(t, 3,
		d.MaxAttempts)

	// Verify payload contains expected fields.
	var payload map[string]any
	require.NoError(t,
		json.Unmarshal(d.
			Payload, &payload))
	assert.Equal(t, domain.
		NotificationEventCostAnomaly,

		payload["event"])
	assert.Equal(t, "proj-1",
		payload["project_id"])
	assert.NotNil(t, payload["severity"])

	for _, key := range []string{"org_id", "today_spend", "avg_7d_spend", "top_contributor", "spike_ratio"} {
		if _, ok := payload[key]; ok {
			require.Failf(t, "test failure",

				"project-scoped payload leaked org-wide field %q: %v", key, payload)
		}
	}
}

func TestAnomalyMonitor_CustomThresholds_Used(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return &billing.OrgSubscription{
				OrgID:                    "org-1",
				AnomalyThresholdWarning:  2.0, // custom lower warning threshold
				AnomalyThresholdCritical: 10.0,
			}, nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			// 2.5x spike -- above custom 2.0 warning but below default 3.0
			return buildSpikeUsage(orgID, "proj-1", 1000, 2500), nil
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
	require.Len(t, deliveries,
		1)
}

func TestAnomalyMonitor_5xSpike_SendsEmail(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return defaultOrgSub("org-1"), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			return buildSpikeUsage(orgID, "proj-1", 1000, 5000), nil // 5x spike = high severity
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{
				{ID: "ch-webhook", ProjectID: "proj-1", ChannelType: domain.ChannelTypeWebhook},
				{ID: "ch-email", ProjectID: "proj-1", ChannelType: domain.ChannelTypeEmail},
			}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
	require.Len(t, deliveries,
		2)

	// At 5x (high severity): both webhook and email should fire.

	channelIDs := map[string]bool{}
	for _, d := range deliveries {
		channelIDs[d.ChannelID] = true
	}
	assert.False(t, !channelIDs["ch-webhook"] || !channelIDs["ch-email"])
}

func TestAnomalyMonitor_3xSpike_NoEmail(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return defaultOrgSub("org-1"), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			return buildSpikeUsage(orgID, "proj-1", 1000, 3000), nil // 3x spike = warning severity
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{
				{ID: "ch-webhook", ProjectID: "proj-1", ChannelType: domain.ChannelTypeWebhook},
				{ID: "ch-email", ProjectID: "proj-1", ChannelType: domain.ChannelTypeEmail},
			}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
	require.Len(t, deliveries,
		1)
	assert.Equal(t, "ch-webhook",
		deliveries[0].ChannelID,
	)

	// At 3x (warning severity): only webhook should fire, not email.
}

func TestAnomalyMonitor_WebhookPayload_RedactsOrgWideAnomalyData(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return defaultOrgSub("org-1"), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			return append(
				buildSpikeUsage(orgID, "proj-main", 1000, 5000),
				buildSpikeUsage(orgID, "proj-other", 1000, 2000)...,
			), nil
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-main", "proj-other"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, projectID string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{
				{ID: "ch-" + projectID, ProjectID: projectID, ChannelType: domain.ChannelTypeWebhook},
			}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
	require.NotEmpty(t,
		deliveries)

	for _, d := range deliveries {
		var payload map[string]any
		require.NoError(t,
			json.Unmarshal(d.
				Payload, &payload))
		require.Equal(t, d.
			ProjectID, payload["project_id"])

		for _, key := range []string{"org_id", "today_spend", "avg_7d_spend", "top_contributor", "spike_ratio"} {
			if _, ok := payload[key]; ok {
				require.Failf(t, "test failure",

					"project-scoped payload leaked org-wide field %q: %v", key, payload)
			}
		}
	}
}

func TestAnomalyMonitor_NoChannels_StillLogs(t *testing.T) {
	t.Parallel()

	deliveryCalled := false
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return defaultOrgSub("org-1"), nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, orgID string, _, _ time.Time) ([]billing.UsageRecord, error) {
			return buildSpikeUsage(orgID, "proj-1", 1000, 5000), nil
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return nil, nil // no channels
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	// Should not panic when there are no channels.
	am.check(context.Background())
	require.False(t, deliveryCalled)
}
