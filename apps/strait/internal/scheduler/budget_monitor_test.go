package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc"
)

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

func TestBudgetMonitor_Run_StopsOnContextCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	bm := NewBudgetMonitor(struct{}{}, nil, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		bm.Run(ctx)
		close(done)
	})

	cancel()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
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

func TestBudgetMonitor_PruneAlertedForPeriods_DropsOldKeys(t *testing.T) {
	t.Parallel()

	bm := NewBudgetMonitor(struct{}{}, nil, time.Minute)
	bm.alerted = map[string]bool{
		"spending:org-1:80:2026-04-14":  true,
		"spending:org-1:100:2026-04-15": true,
		"runlimit:org-2:80:2026-04":     true,
		"runlimit:org-3:80:2026-03":     true,
		"malformed":                     true,
	}
	bm.alertedDate = "2026-04-14"

	bm.pruneAlertedForPeriods("2026-04-15", "2026-04")

	if bm.alertedDate != "2026-04-15" {
		t.Fatalf("alertedDate = %q, want 2026-04-15", bm.alertedDate)
	}
	if len(bm.alerted) != 2 {
		t.Fatalf("alerted keys = %v, want 2 current-period keys", bm.alerted)
	}
	for _, key := range []string{"spending:org-1:100:2026-04-15", "runlimit:org-2:80:2026-04"} {
		if !bm.alerted[key] {
			t.Fatalf("expected key %q to remain after prune", key)
		}
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

func (m *mockSpendingLimitStore) ListEnabledNotificationChannelsByProjectIDs(ctx context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error) {
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

func (m *mockSpendingLimitStore) CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error {
	if m.createNotificationDeliveryFn != nil {
		return m.createNotificationDeliveryFn(ctx, d)
	}
	return nil
}

type monthlyWarningBillingStore struct {
	billing.Store
	sub *billing.OrgSubscription
}

func (s *monthlyWarningBillingStore) GetOrgSubscription(context.Context, string) (*billing.OrgSubscription, error) {
	return s.sub, nil
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

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
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
	if deliveries[0].DedupeKey == "" {
		t.Fatal("expected budget notification delivery to carry a durable dedupe key")
	}
	assertProjectScopedBudgetPayload(t, deliveries[0].Payload)
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

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
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
	for _, d := range deliveries {
		assertProjectScopedBudgetPayload(t, d.Payload)
	}
}

func TestBudgetMonitor_RunLimitPayloadOmitsOrgScopeValues(t *testing.T) {
	t.Parallel()

	assertProjectScopedBudgetPayload(t, runLimitNotificationPayload())
}

func TestBudgetMonitor_RunLimitWarningUsesMonthlyAllowance(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	limits := billing.GetPlanLimits(domain.PlanFree)
	limits.MaxRunsPerMonth = 10
	entitlements, err := json.Marshal(limits)
	if err != nil {
		t.Fatalf("marshal entitlements: %v", err)
	}
	enforcer := billing.NewEnforcer(&monthlyWarningBillingStore{
		sub: &billing.OrgSubscription{
			OrgID:        "org-1",
			PlanTier:     string(domain.PlanFree),
			Status:       "active",
			Entitlements: entitlements,
		},
	}, rdb, slog.Default())

	for range 8 {
		if err := enforcer.CheckMonthlyRunLimit(context.Background(), "org-1"); err != nil {
			t.Fatalf("seed monthly usage: %v", err)
		}
	}

	var deliveries []*domain.NotificationDelivery
	store := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		listProjectsByOrgFn: func(context.Context, string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(context.Context, string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-webhook", ProjectID: "proj-1", ChannelType: domain.ChannelTypeWebhook}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).
		WithRunLimitNotifications(store, enforcer)
	bm.check(context.Background())

	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1 monthly run allowance warning", len(deliveries))
	}
	if deliveries[0].EventType != domain.NotificationEventRunLimitApproaching {
		t.Fatalf("event type = %s, want %s", deliveries[0].EventType, domain.NotificationEventRunLimitApproaching)
	}
	assertProjectScopedBudgetPayload(t, deliveries[0].Payload)
}

func assertProjectScopedBudgetPayload(t *testing.T, payload json.RawMessage) {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	for _, key := range []string{"org_id", "overage_pct", "spending_limit_usd", "current_spend_usd"} {
		if _, ok := decoded[key]; ok {
			t.Fatalf("project-scoped budget payload leaked %q: %s", key, string(payload))
		}
	}
	if decoded["event"] == "" {
		t.Fatalf("payload missing event: %s", string(payload))
	}
	if decoded["threshold_pct"] == nil {
		t.Fatalf("payload missing threshold_pct: %s", string(payload))
	}
}

func TestBudgetMonitor_SpendingAlertRetriesAfterDeliveryFailure(t *testing.T) {
	t.Parallel()

	attempts := 0
	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(context.Context, string) (*billing.OrgSubscription, error) {
			return newSpendingLimitSub("org-1", 100_000_000, "starter"), nil
		},
		sumOrgPeriodSpendFn: func(context.Context, string, time.Time) (int64, error) {
			return 119_990_000, nil
		},
		listProjectsByOrgFn: func(context.Context, string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(context.Context, string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-webhook", ProjectID: "proj-1", ChannelType: domain.ChannelTypeWebhook}}, nil
		},
		createNotificationDeliveryFn: func(context.Context, *domain.NotificationDelivery) error {
			attempts++
			if attempts == 1 {
				return errors.New("transient delivery insert failure")
			}
			return nil
		},
	}

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	bm.check(context.Background())
	bm.check(context.Background())

	if attempts != 2 {
		t.Fatalf("delivery attempts = %d, want retry after first failure", attempts)
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

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
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

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
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

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	bm.check(context.Background())

	if deliveryCalled {
		t.Fatal("expected no spending alert for free org with hard cap (limit=0)")
	}
}
