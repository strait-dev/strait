package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"
)

type mockBudgetStore struct {
	listProjectsFn func(ctx context.Context) ([]store.ProjectComputeQuota, error)
	sumDailyCostFn func(ctx context.Context, projectID, timezone string) (int64, error)
}

func (m *mockBudgetStore) ListProjectsWithComputeLimit(ctx context.Context) ([]store.ProjectComputeQuota, error) {
	if m.listProjectsFn != nil {
		return m.listProjectsFn(ctx)
	}
	return nil, nil
}

func (m *mockBudgetStore) SumDailyComputeCost(ctx context.Context, projectID, timezone string) (int64, error) {
	if m.sumDailyCostFn != nil {
		return m.sumDailyCostFn(ctx, projectID, timezone)
	}
	return 0, nil
}

type mockEnqueuer struct {
	enqueueFn func(ctx context.Context, projectID string, payload json.RawMessage) error
	calls     []enqueueCall
}

type enqueueCall struct {
	ProjectID string
	Payload   json.RawMessage
}

func (m *mockEnqueuer) EnqueueBudgetAlert(ctx context.Context, projectID string, payload json.RawMessage) error {
	m.calls = append(m.calls, enqueueCall{ProjectID: projectID, Payload: payload})
	if m.enqueueFn != nil {
		return m.enqueueFn(ctx, projectID, payload)
	}
	return nil
}

func TestBudgetMonitor_AboveThreshold_AlertFires(t *testing.T) {
	t.Parallel()

	enqueuer := &mockEnqueuer{}
	s := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
			return []store.ProjectComputeQuota{
				{ProjectID: "proj-1", Timezone: "UTC", ComputeDailyCostLimitMicrousd: 100_000},
			}, nil
		},
		sumDailyCostFn: func(_ context.Context, _ string, _ string) (int64, error) {
			return 85_000, nil // 85% > 80% threshold
		},
	}

	bm := NewBudgetMonitor(s, enqueuer, time.Minute)
	bm.check(context.Background())

	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(enqueuer.calls))
	}
	if enqueuer.calls[0].ProjectID != "proj-1" {
		t.Fatalf("expected project proj-1, got %s", enqueuer.calls[0].ProjectID)
	}
}

func TestBudgetMonitor_BelowThreshold_NoAlert(t *testing.T) {
	t.Parallel()

	enqueuer := &mockEnqueuer{}
	s := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
			return []store.ProjectComputeQuota{
				{ProjectID: "proj-1", Timezone: "UTC", ComputeDailyCostLimitMicrousd: 100_000},
			}, nil
		},
		sumDailyCostFn: func(_ context.Context, _ string, _ string) (int64, error) {
			return 50_000, nil // 50% < 80% threshold
		},
	}

	bm := NewBudgetMonitor(s, enqueuer, time.Minute)
	bm.check(context.Background())

	if len(enqueuer.calls) != 0 {
		t.Fatalf("expected 0 alerts, got %d", len(enqueuer.calls))
	}
}

func TestBudgetMonitor_SameDayRecheck_Dedup(t *testing.T) {
	t.Parallel()

	enqueuer := &mockEnqueuer{}
	s := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
			return []store.ProjectComputeQuota{
				{ProjectID: "proj-1", Timezone: "UTC", ComputeDailyCostLimitMicrousd: 100_000},
			}, nil
		},
		sumDailyCostFn: func(_ context.Context, _ string, _ string) (int64, error) {
			return 90_000, nil
		},
	}

	bm := NewBudgetMonitor(s, enqueuer, time.Minute)
	bm.check(context.Background())
	bm.check(context.Background()) // second check same day

	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 alert (dedup), got %d", len(enqueuer.calls))
	}
}

func TestBudgetMonitor_NextDay_AlertsAgain(t *testing.T) {
	t.Parallel()

	enqueuer := &mockEnqueuer{}
	s := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
			return []store.ProjectComputeQuota{
				{ProjectID: "proj-1", Timezone: "UTC", ComputeDailyCostLimitMicrousd: 100_000},
			}, nil
		},
		sumDailyCostFn: func(_ context.Context, _ string, _ string) (int64, error) {
			return 90_000, nil
		},
	}

	bm := NewBudgetMonitor(s, enqueuer, time.Minute)
	bm.check(context.Background())

	// Simulate next day by changing the alerted map key to yesterday.
	bm.alertedMu.Lock()
	bm.alerted = map[string]bool{"proj-1:1970-01-01": true}
	bm.alertedMu.Unlock()

	bm.check(context.Background())

	if len(enqueuer.calls) != 2 {
		t.Fatalf("expected 2 alerts (new day), got %d", len(enqueuer.calls))
	}
}

func TestBudgetMonitor_NoProjectsWithLimit_NoOp(t *testing.T) {
	t.Parallel()

	enqueuer := &mockEnqueuer{}
	s := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
			return nil, nil
		},
	}

	bm := NewBudgetMonitor(s, enqueuer, time.Minute)
	bm.check(context.Background())

	if len(enqueuer.calls) != 0 {
		t.Fatalf("expected 0 alerts, got %d", len(enqueuer.calls))
	}
}

func TestBudgetMonitor_StoreError_LogsWarningContinues(t *testing.T) {
	t.Parallel()

	enqueuer := &mockEnqueuer{}
	s := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
			return nil, errors.New("db down")
		},
	}

	bm := NewBudgetMonitor(s, enqueuer, time.Minute)
	// Should not panic.
	bm.check(context.Background())

	if len(enqueuer.calls) != 0 {
		t.Fatalf("expected 0 alerts, got %d", len(enqueuer.calls))
	}
}

func TestBudgetMonitor_CostError_LogsWarningContinues(t *testing.T) {
	t.Parallel()

	enqueuer := &mockEnqueuer{}
	s := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
			return []store.ProjectComputeQuota{
				{ProjectID: "proj-1", Timezone: "UTC", ComputeDailyCostLimitMicrousd: 100_000},
			}, nil
		},
		sumDailyCostFn: func(_ context.Context, _ string, _ string) (int64, error) {
			return 0, errors.New("query failed")
		},
	}

	bm := NewBudgetMonitor(s, enqueuer, time.Minute)
	bm.check(context.Background())

	if len(enqueuer.calls) != 0 {
		t.Fatalf("expected 0 alerts after cost error, got %d", len(enqueuer.calls))
	}
}

func TestBudgetMonitor_NilEnqueuer_LogsNoPanic(t *testing.T) {
	t.Parallel()

	s := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
			return []store.ProjectComputeQuota{
				{ProjectID: "proj-1", Timezone: "UTC", ComputeDailyCostLimitMicrousd: 100_000},
			}, nil
		},
		sumDailyCostFn: func(_ context.Context, _ string, _ string) (int64, error) {
			return 90_000, nil
		},
	}

	bm := NewBudgetMonitor(s, nil, time.Minute)
	// Should not panic with nil enqueuer.
	bm.check(context.Background())
}

func TestBudgetMonitor_Run_StopsOnContextCancel(t *testing.T) {
	t.Parallel()

	s := &mockBudgetStore{}
	bm := NewBudgetMonitor(s, nil, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		bm.Run(ctx)
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

func TestBudgetMonitor_EnqueueError_ContinuesWithoutMarkingAlerted(t *testing.T) {
	t.Parallel()

	enqueuer := &mockEnqueuer{
		enqueueFn: func(_ context.Context, _ string, _ json.RawMessage) error {
			return errors.New("enqueue failed")
		},
	}
	s := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
			return []store.ProjectComputeQuota{
				{ProjectID: "proj-1", Timezone: "UTC", ComputeDailyCostLimitMicrousd: 100_000},
			}, nil
		},
		sumDailyCostFn: func(_ context.Context, _ string, _ string) (int64, error) {
			return 90_000, nil
		},
	}

	bm := NewBudgetMonitor(s, enqueuer, time.Minute)
	bm.check(context.Background())

	// Enqueue was called but failed — project should NOT be marked as alerted.
	bm.alertedMu.Lock()
	alerted := bm.alerted["proj-1:"+time.Now().UTC().Format("2006-01-02")]
	bm.alertedMu.Unlock()

	if alerted {
		t.Fatal("project should not be marked as alerted when enqueue fails")
	}
}

func TestBudgetMonitor_MultipleProjects_AlertsEachIndependently(t *testing.T) {
	t.Parallel()

	enqueuer := &mockEnqueuer{}
	s := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
			return []store.ProjectComputeQuota{
				{ProjectID: "proj-1", Timezone: "UTC", ComputeDailyCostLimitMicrousd: 100_000},
				{ProjectID: "proj-2", Timezone: "UTC", ComputeDailyCostLimitMicrousd: 200_000},
			}, nil
		},
		sumDailyCostFn: func(_ context.Context, projectID string, _ string) (int64, error) {
			switch projectID {
			case "proj-1":
				return 90_000, nil // over threshold
			case "proj-2":
				return 50_000, nil // under threshold
			}
			return 0, nil
		},
	}

	bm := NewBudgetMonitor(s, enqueuer, time.Minute)
	bm.check(context.Background())

	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 alert (only proj-1), got %d", len(enqueuer.calls))
	}
	if enqueuer.calls[0].ProjectID != "proj-1" {
		t.Fatalf("expected alert for proj-1, got %s", enqueuer.calls[0].ProjectID)
	}
}

func TestBudgetMonitor_ExactlyAtThreshold_NoAlert(t *testing.T) {
	t.Parallel()

	enqueuer := &mockEnqueuer{}
	s := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
			return []store.ProjectComputeQuota{
				{ProjectID: "proj-1", Timezone: "UTC", ComputeDailyCostLimitMicrousd: 100_000},
			}, nil
		},
		sumDailyCostFn: func(_ context.Context, _ string, _ string) (int64, error) {
			// Exactly at threshold: 100000 * 80 / 100 = 80000, cost < 80000
			return 79_999, nil
		},
	}

	bm := NewBudgetMonitor(s, enqueuer, time.Minute)
	bm.check(context.Background())

	if len(enqueuer.calls) != 0 {
		t.Fatalf("expected 0 alerts at exactly below threshold, got %d", len(enqueuer.calls))
	}
}

func TestBudgetMonitor_Run_ChecksOnInterval(t *testing.T) {
	t.Parallel()

	var checkCount atomic.Int32
	s := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
			checkCount.Add(1)
			return nil, nil
		},
	}

	bm := NewBudgetMonitor(s, nil, 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())

	go bm.Run(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()

	count := checkCount.Load()
	if count < 2 {
		t.Fatalf("expected at least 2 checks, got %d", count)
	}
}

func TestFormatBudgetAlertKey(t *testing.T) {
	t.Parallel()
	date := time.Date(2026, 3, 16, 14, 30, 0, 0, time.UTC)
	key := FormatBudgetAlertKey("proj-1", date)
	expected := "proj-1:2026-03-16"
	if key != expected {
		t.Fatalf("expected %q, got %q", expected, key)
	}
}

func TestBudgetMonitor_ConcurrentCheck_NoDuplicateAlert(t *testing.T) {
	t.Parallel()

	enqueuer := &mockEnqueuer{}
	s := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
			return []store.ProjectComputeQuota{
				{ProjectID: "proj-1", Timezone: "UTC", ComputeDailyCostLimitMicrousd: 100_000},
			}, nil
		},
		sumDailyCostFn: func(_ context.Context, _ string, _ string) (int64, error) {
			time.Sleep(10 * time.Millisecond) // simulate slow query
			return 90_000, nil
		},
	}

	bm := NewBudgetMonitor(s, enqueuer, time.Minute)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			bm.check(context.Background())
		})
	}
	wg.Wait()

	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected exactly 1 alert with concurrent checks, got %d", len(enqueuer.calls))
	}
}

// mockNotifierBudgetStore composes mockBudgetStore with ApprovalNotifierStore.
type mockNotifierBudgetStore struct {
	mockBudgetStore
	listEnabledNotificationChannelsFn func(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	createNotificationDeliveryFn      func(ctx context.Context, d *domain.NotificationDelivery) error
	getWorkflowRunFn                  func(ctx context.Context, id string) (*domain.WorkflowRun, error)
}

func (m *mockNotifierBudgetStore) ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error) {
	if m.listEnabledNotificationChannelsFn != nil {
		return m.listEnabledNotificationChannelsFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockNotifierBudgetStore) CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error {
	if m.createNotificationDeliveryFn != nil {
		return m.createNotificationDeliveryFn(ctx, d)
	}
	return nil
}

func (m *mockNotifierBudgetStore) GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	if m.getWorkflowRunFn != nil {
		return m.getWorkflowRunFn(ctx, id)
	}
	return nil, nil
}

func TestBudgetMonitor_AboveThreshold_SendsNotification(t *testing.T) {
	t.Parallel()
	var deliveries []*domain.NotificationDelivery
	enqueuer := &mockEnqueuer{}
	ms := &mockNotifierBudgetStore{
		mockBudgetStore: mockBudgetStore{
			listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
				return []store.ProjectComputeQuota{
					{ProjectID: "proj-1", Timezone: "UTC", ComputeDailyCostLimitMicrousd: 100_000},
				}, nil
			},
			sumDailyCostFn: func(_ context.Context, _ string, _ string) (int64, error) {
				return 85_000, nil
			},
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	bm := NewBudgetMonitor(ms, enqueuer, time.Minute)
	bm.check(context.Background())

	if len(deliveries) != 1 {
		t.Fatalf("expected 1 notification delivery, got %d", len(deliveries))
	}
	if deliveries[0].EventType != domain.NotificationEventBudgetThreshold {
		t.Errorf("expected event type %s, got %s", domain.NotificationEventBudgetThreshold, deliveries[0].EventType)
	}
}

func TestBudgetMonitor_AboveThreshold_NoNotificationWithoutInterface(t *testing.T) {
	t.Parallel()
	enqueuer := &mockEnqueuer{}
	ms := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
			return []store.ProjectComputeQuota{
				{ProjectID: "proj-1", Timezone: "UTC", ComputeDailyCostLimitMicrousd: 100_000},
			}, nil
		},
		sumDailyCostFn: func(_ context.Context, _ string, _ string) (int64, error) {
			return 85_000, nil
		},
	}

	bm := NewBudgetMonitor(ms, enqueuer, time.Minute)
	bm.check(context.Background())

	// Webhook alert should still fire even without notification interface.
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 webhook alert, got %d", len(enqueuer.calls))
	}
}

func TestBudgetMonitor_BelowThreshold_NoNotification(t *testing.T) {
	t.Parallel()
	deliveryCalled := false
	ms := &mockNotifierBudgetStore{
		mockBudgetStore: mockBudgetStore{
			listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) {
				return []store.ProjectComputeQuota{
					{ProjectID: "proj-1", Timezone: "UTC", ComputeDailyCostLimitMicrousd: 100_000},
				}, nil
			},
			sumDailyCostFn: func(_ context.Context, _ string, _ string) (int64, error) {
				return 50_000, nil
			},
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	bm := NewBudgetMonitor(ms, &mockEnqueuer{}, time.Minute)
	bm.check(context.Background())

	if deliveryCalled {
		t.Fatal("expected no notification when below threshold")
	}
}

// mockSpendingLimitStore implements SpendingLimitStore for testing.
type mockSpendingLimitStore struct {
	listAllSubscribedOrgIDsFn         func(ctx context.Context) ([]string, error)
	getOrgSubscriptionFn              func(ctx context.Context, orgID string) (*billing.OrgSubscription, error)
	sumOrgPeriodSpendFn               func(ctx context.Context, orgID string, from time.Time) (int64, error)
	listProjectsByOrgFn               func(ctx context.Context, orgID string) ([]string, error)
	listEnabledNotificationChannelsFn func(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	createNotificationDeliveryFn      func(ctx context.Context, d *domain.NotificationDelivery) error
}

func (m *mockSpendingLimitStore) ListAllSubscribedOrgIDs(ctx context.Context) ([]string, error) {
	if m.listAllSubscribedOrgIDsFn != nil {
		return m.listAllSubscribedOrgIDsFn(ctx)
	}
	return nil, nil
}

func (m *mockSpendingLimitStore) GetOrgSubscription(ctx context.Context, orgID string) (*billing.OrgSubscription, error) {
	if m.getOrgSubscriptionFn != nil {
		return m.getOrgSubscriptionFn(ctx, orgID)
	}
	return nil, nil
}

func (m *mockSpendingLimitStore) SumOrgPeriodSpend(ctx context.Context, orgID string, from time.Time) (int64, error) {
	if m.sumOrgPeriodSpendFn != nil {
		return m.sumOrgPeriodSpendFn(ctx, orgID, from)
	}
	return 0, nil
}

func (m *mockSpendingLimitStore) ListProjectsByOrg(ctx context.Context, orgID string) ([]string, error) {
	if m.listProjectsByOrgFn != nil {
		return m.listProjectsByOrgFn(ctx, orgID)
	}
	return nil, nil
}

func (m *mockSpendingLimitStore) ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error) {
	if m.listEnabledNotificationChannelsFn != nil {
		return m.listEnabledNotificationChannelsFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockSpendingLimitStore) CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error {
	if m.createNotificationDeliveryFn != nil {
		return m.createNotificationDeliveryFn(ctx, d)
	}
	return nil
}

func newSpendingLimitSub(orgID string, limitMicro int64, planTier string) *billing.OrgSubscription {
	now := time.Now().AddDate(0, 0, -15)
	return &billing.OrgSubscription{
		OrgID:                 orgID,
		PlanTier:              planTier,
		SpendingLimitMicrousd: limitMicro,
		LimitAction:           "notify",
		CurrentPeriodStart:    &now,
	}
}

func TestBudgetMonitor_80Percent_TriggersWebhook(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return newSpendingLimitSub("org-1", 100_000_000, "starter"), nil // $100 limit
		},
		sumOrgPeriodSpendFn: func(_ context.Context, _ string, _ time.Time) (int64, error) {
			// included credit for starter is $19.99 = 19_990_000 micro
			// overage = spend - credit = 99_990_000 - 19_990_000 = 80_000_000
			// 80% of $100 limit = $80 = 80_000_000 micro
			return 99_990_000, nil
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

	bs := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) { return nil, nil },
	}
	bm := NewBudgetMonitor(bs, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	bm.check(context.Background())

	// At 80%: only webhook should fire, not email.
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery (webhook only), got %d", len(deliveries))
	}
	if deliveries[0].ChannelID != "ch-webhook" {
		t.Errorf("expected webhook channel, got %s", deliveries[0].ChannelID)
	}
	if deliveries[0].EventType != domain.NotificationEventSpendingLimitWarning {
		t.Errorf("expected event %s, got %s", domain.NotificationEventSpendingLimitWarning, deliveries[0].EventType)
	}
}

func TestBudgetMonitor_100Percent_TriggersWebhookAndEmail(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return newSpendingLimitSub("org-1", 100_000_000, "starter"), nil
		},
		sumOrgPeriodSpendFn: func(_ context.Context, _ string, _ time.Time) (int64, error) {
			// overage = 119_990_000 - 19_990_000 = 100_000_000 = 100% of limit
			return 119_990_000, nil
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

	bs := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) { return nil, nil },
	}
	bm := NewBudgetMonitor(bs, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	bm.check(context.Background())

	// At 100%: both webhook and email should fire.
	if len(deliveries) != 2 {
		t.Fatalf("expected 2 deliveries (webhook + email), got %d", len(deliveries))
	}
	channelTypes := map[string]bool{}
	for _, d := range deliveries {
		channelTypes[d.ChannelID] = true
		if d.EventType != domain.NotificationEventSpendingLimitReached {
			t.Errorf("expected event %s, got %s", domain.NotificationEventSpendingLimitReached, d.EventType)
		}
	}
	if !channelTypes["ch-webhook"] || !channelTypes["ch-email"] {
		t.Error("expected both webhook and email channel deliveries")
	}
}

func TestBudgetMonitor_Below80_NoAlert(t *testing.T) {
	t.Parallel()

	deliveryCalled := false
	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return newSpendingLimitSub("org-1", 100_000_000, "starter"), nil
		},
		sumOrgPeriodSpendFn: func(_ context.Context, _ string, _ time.Time) (int64, error) {
			// overage = 50_000_000 - 19_990_000 = 30_010_000 = ~30% of limit
			return 50_000_000, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	bs := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) { return nil, nil },
	}
	bm := NewBudgetMonitor(bs, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	bm.check(context.Background())

	if deliveryCalled {
		t.Fatal("expected no spending alert below 80%")
	}
}

func TestBudgetMonitor_NoSpendingLimit_Skipped(t *testing.T) {
	t.Parallel()

	deliveryCalled := false
	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return newSpendingLimitSub("org-1", -1, "pro"), nil // -1 = no limit
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	bs := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) { return nil, nil },
	}
	bm := NewBudgetMonitor(bs, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	bm.check(context.Background())

	if deliveryCalled {
		t.Fatal("expected no alert when spending limit is -1 (disabled)")
	}
}

func TestBudgetMonitor_FreeOrgHardCapped_NoSpendingAlert(t *testing.T) {
	t.Parallel()

	deliveryCalled := false
	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return newSpendingLimitSub("org-1", 0, "free"), nil // 0 = hard cap
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	bs := &mockBudgetStore{
		listProjectsFn: func(context.Context) ([]store.ProjectComputeQuota, error) { return nil, nil },
	}
	bm := NewBudgetMonitor(bs, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	bm.check(context.Background())

	if deliveryCalled {
		t.Fatal("expected no spending alert for free org with hard cap (limit=0)")
	}
}
