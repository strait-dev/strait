package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

// mockAnomalyMonitorStore implements AnomalyMonitorStore with function callbacks
// for key methods and zero-value stubs for the rest of billing.Store.
type mockAnomalyMonitorStore struct {
	listOrgsWithPendingDowngradeFn    func(ctx context.Context) ([]billing.OrgSubscription, error)
	getOrgSubscriptionFn              func(ctx context.Context, orgID string) (*billing.OrgSubscription, error)
	getOrgUsageForPeriodFn            func(ctx context.Context, orgID string, from, to time.Time) ([]billing.UsageRecord, error)
	listProjectsByOrgFn               func(ctx context.Context, orgID string) ([]string, error)
	listEnabledNotificationChannelsFn func(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	createNotificationDeliveryFn      func(ctx context.Context, d *domain.NotificationDelivery) error
}

// Key methods with callbacks.

func (m *mockAnomalyMonitorStore) ListOrgsWithPendingDowngrade(ctx context.Context) ([]billing.OrgSubscription, error) {
	if m.listOrgsWithPendingDowngradeFn != nil {
		return m.listOrgsWithPendingDowngradeFn(ctx)
	}
	return nil, nil
}

func (m *mockAnomalyMonitorStore) GetOrgSubscription(ctx context.Context, orgID string) (*billing.OrgSubscription, error) {
	if m.getOrgSubscriptionFn != nil {
		return m.getOrgSubscriptionFn(ctx, orgID)
	}
	return nil, nil
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

func (m *mockAnomalyMonitorStore) CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error {
	if m.createNotificationDeliveryFn != nil {
		return m.createNotificationDeliveryFn(ctx, d)
	}
	return nil
}

// Stub methods for the rest of billing.Store.

func (m *mockAnomalyMonitorStore) UpsertOrgSubscription(context.Context, *billing.OrgSubscription) error {
	return nil
}
func (m *mockAnomalyMonitorStore) UpdateOrgSubscriptionPlan(context.Context, string, string, string) error {
	return nil
}
func (m *mockAnomalyMonitorStore) UpdateOrgSubscriptionFull(context.Context, string, string, string, *time.Time, *time.Time) error {
	return nil
}
func (m *mockAnomalyMonitorStore) UpdateSpendingLimit(context.Context, string, int64, string) error {
	return nil
}
func (m *mockAnomalyMonitorStore) SetPendingPlanTier(context.Context, string, string) error {
	return nil
}
func (m *mockAnomalyMonitorStore) ClearPendingPlanTier(context.Context, string) error  { return nil }
func (m *mockAnomalyMonitorStore) ApplyPendingDowngrade(context.Context, string) error { return nil }
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
func (m *mockAnomalyMonitorStore) CountExecutingRunsByOrg(context.Context, string) (int, error) {
	return 0, nil
}
func (m *mockAnomalyMonitorStore) CountAIModelCallsByOrg(_ context.Context, _ string, _, _ time.Time) (int64, error) {
	return 0, nil
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
		listOrgsWithPendingDowngradeFn: func(context.Context) ([]billing.OrgSubscription, error) {
			return []billing.OrgSubscription{{OrgID: "org-1"}}, nil
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

	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	if deliveries[0].EventType != domain.NotificationEventCostAnomaly {
		t.Errorf("expected event type %s, got %s", domain.NotificationEventCostAnomaly, deliveries[0].EventType)
	}
}

func TestAnomalyMonitor_NoSpike_NoAlert(t *testing.T) {
	t.Parallel()

	deliveryCalled := false
	s := &mockAnomalyMonitorStore{
		listOrgsWithPendingDowngradeFn: func(context.Context) ([]billing.OrgSubscription, error) {
			return []billing.OrgSubscription{{OrgID: "org-1"}}, nil
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

	if deliveryCalled {
		t.Fatal("expected no delivery when there is no spike")
	}
}

func TestAnomalyMonitor_CooldownWindow_Dedup(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	s := &mockAnomalyMonitorStore{
		listOrgsWithPendingDowngradeFn: func(context.Context) ([]billing.OrgSubscription, error) {
			return []billing.OrgSubscription{{OrgID: "org-1"}}, nil
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
	am.check(context.Background()) // second check in same 4h block

	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery (dedup), got %d", len(deliveries))
	}
}

func TestAnomalyMonitor_AfterCooldown_AlertsAgain(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	s := &mockAnomalyMonitorStore{
		listOrgsWithPendingDowngradeFn: func(context.Context) ([]billing.OrgSubscription, error) {
			return []billing.OrgSubscription{{OrgID: "org-1"}}, nil
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

	// Simulate new cooldown block by clearing the alerted map.
	am.alertedMu.Lock()
	am.alerted = make(map[string]bool)
	am.alertedMu.Unlock()

	am.check(context.Background())

	if len(deliveries) != 2 {
		t.Fatalf("expected 2 deliveries (new cooldown block), got %d", len(deliveries))
	}
}

func TestAnomalyMonitor_NoOrgsWithActivity_NoOp(t *testing.T) {
	t.Parallel()

	deliveryCalled := false
	s := &mockAnomalyMonitorStore{
		listOrgsWithPendingDowngradeFn: func(context.Context) ([]billing.OrgSubscription, error) {
			return nil, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())

	if deliveryCalled {
		t.Fatal("expected no deliveries when org list is empty")
	}
}

func TestAnomalyMonitor_StoreError_LogsContinues(t *testing.T) {
	t.Parallel()

	s := &mockAnomalyMonitorStore{
		listOrgsWithPendingDowngradeFn: func(context.Context) ([]billing.OrgSubscription, error) {
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
		listOrgsWithPendingDowngradeFn: func(context.Context) ([]billing.OrgSubscription, error) {
			return []billing.OrgSubscription{
				{OrgID: "org-1"},
				{OrgID: "org-2"},
			}, nil
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

	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery (only org-1), got %d", len(deliveries))
	}
	if deliveries[0].ProjectID != "proj-1" {
		t.Errorf("expected delivery for proj-1, got %s", deliveries[0].ProjectID)
	}
}

func TestAnomalyMonitor_Run_StopsOnContextCancel(t *testing.T) {
	t.Parallel()

	s := &mockAnomalyMonitorStore{}
	am := NewAnomalyMonitor(s, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		am.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

func TestAnomalyMonitor_Run_ChecksOnInterval(t *testing.T) {
	t.Parallel()

	var checkCount atomic.Int32
	s := &mockAnomalyMonitorStore{
		listOrgsWithPendingDowngradeFn: func(context.Context) ([]billing.OrgSubscription, error) {
			checkCount.Add(1)
			return nil, nil
		},
	}

	am := NewAnomalyMonitor(s, 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())

	go am.Run(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()

	count := checkCount.Load()
	if count < 2 {
		t.Fatalf("expected at least 2 checks, got %d", count)
	}
}

func TestAnomalyMonitor_NotificationDeliveryCreated(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	s := &mockAnomalyMonitorStore{
		listOrgsWithPendingDowngradeFn: func(context.Context) ([]billing.OrgSubscription, error) {
			return []billing.OrgSubscription{{OrgID: "org-1"}}, nil
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

	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}

	d := deliveries[0]
	if d.EventType != domain.NotificationEventCostAnomaly {
		t.Errorf("expected event type %s, got %s", domain.NotificationEventCostAnomaly, d.EventType)
	}
	if d.ChannelID != "ch-1" {
		t.Errorf("expected channel ch-1, got %s", d.ChannelID)
	}
	if d.ProjectID != "proj-1" {
		t.Errorf("expected project proj-1, got %s", d.ProjectID)
	}
	if d.Status != "pending" {
		t.Errorf("expected status pending, got %s", d.Status)
	}
	if d.MaxAttempts != 3 {
		t.Errorf("expected max_attempts 3, got %d", d.MaxAttempts)
	}

	// Verify payload contains expected fields.
	var payload map[string]any
	if err := json.Unmarshal(d.Payload, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if payload["event"] != domain.NotificationEventCostAnomaly {
		t.Errorf("payload event mismatch: %v", payload["event"])
	}
	if payload["org_id"] != "org-1" {
		t.Errorf("payload org_id mismatch: %v", payload["org_id"])
	}
	if payload["severity"] == nil {
		t.Error("payload missing severity")
	}
}

func TestAnomalyMonitor_CustomThresholds_Used(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	s := &mockAnomalyMonitorStore{
		listOrgsWithPendingDowngradeFn: func(context.Context) ([]billing.OrgSubscription, error) {
			return []billing.OrgSubscription{{OrgID: "org-1"}}, nil
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

	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery with custom threshold, got %d", len(deliveries))
	}
}

func TestAnomalyMonitor_NoChannels_StillLogs(t *testing.T) {
	t.Parallel()

	deliveryCalled := false
	s := &mockAnomalyMonitorStore{
		listOrgsWithPendingDowngradeFn: func(context.Context) ([]billing.OrgSubscription, error) {
			return []billing.OrgSubscription{{OrgID: "org-1"}}, nil
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

	if deliveryCalled {
		t.Fatal("expected no delivery when there are no channels")
	}
}
