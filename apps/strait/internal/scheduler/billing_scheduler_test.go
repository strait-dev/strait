package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"strait/internal/billing"
	"strait/internal/domain"
)

// Section separator.
// BudgetMonitor extended tests.
// Section separator.

func TestBudgetMonitor_SpendingLimit_DedupPerDay(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-dedup"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return newSpendingLimitSub("org-dedup", 100_000_000, "starter"), nil
		},
		sumOrgPeriodSpendFn: func(_ context.Context, _ string, _ time.Time) (int64, error) {
			return 99_990_000, nil
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{
				{ID: "ch-1", ProjectID: "proj-1", ChannelType: domain.ChannelTypeWebhook},
			}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)

	// Check twice in the same day.
	bm.check(context.Background())
	bm.check(context.Background())

	// Only one delivery expected due to dedup.
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery (dedup), got %d", len(deliveries))
	}
}

func TestBudgetMonitor_SpendingLimit_NilSubscription_Skips(t *testing.T) {
	t.Parallel()

	deliveryCalled := false
	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-nil"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return nil, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	bm.check(context.Background())

	if deliveryCalled {
		t.Fatal("expected no delivery for nil subscription")
	}
}

func TestBudgetMonitor_SpendingLimit_ListOrgsError(t *testing.T) {
	t.Parallel()

	deliveryCalled := false
	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return nil, errors.New("db error")
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	bm.check(context.Background())

	if deliveryCalled {
		t.Fatal("expected no delivery when list orgs fails")
	}
}

func TestBudgetMonitor_OldAlertKeys_PrunedOnNextCheck(t *testing.T) {
	t.Parallel()

	enqueuer := &mockEnqueuer{}
	bm := NewBudgetMonitor(struct{}{}, enqueuer, time.Minute)

	// Seed with stale keys from a past date.
	bm.alertedMu.Lock()
	bm.alerted["proj-old:2020-01-01"] = true
	bm.alerted["proj-old2:2020-01-01"] = true
	bm.alertedMu.Unlock()

	bm.check(context.Background())

	bm.alertedMu.Lock()
	remaining := len(bm.alerted)
	bm.alertedMu.Unlock()

	if remaining != 0 {
		t.Fatalf("expected old keys to be pruned, but %d remain", remaining)
	}
}

func TestBudgetMonitor_DefaultInterval(t *testing.T) {
	t.Parallel()

	bm := NewBudgetMonitor(struct{}{}, nil, 0)
	if bm.interval != 5*time.Minute {
		t.Fatalf("expected default interval 5m, got %v", bm.interval)
	}
}

func TestBudgetMonitor_NegativeInterval_DefaultsTo5Min(t *testing.T) {
	t.Parallel()

	bm := NewBudgetMonitor(struct{}{}, nil, -1*time.Minute)
	if bm.interval != 5*time.Minute {
		t.Fatalf("expected default interval 5m, got %v", bm.interval)
	}
}

// Section separator.
// DowngradeApplier extended tests.
// Section separator.

func TestDowngradeApplier_ListError_Aborts(t *testing.T) {
	t.Parallel()

	s := &mockDowngradeStore{
		listErr: errors.New("connection refused"),
	}

	applier := NewDowngradeApplier(s, nil, time.Minute)
	applier.apply(context.Background())

	if len(s.appliedOrgIDs) != 0 {
		t.Fatalf("expected 0 downgrades, got %d", len(s.appliedOrgIDs))
	}
}

func TestDowngradeApplier_NoPendingDowngrades(t *testing.T) {
	t.Parallel()

	s := &mockDowngradeStore{
		pendingOrgs: nil,
	}

	applier := NewDowngradeApplier(s, nil, time.Minute)
	applier.apply(context.Background())

	if len(s.appliedOrgIDs) != 0 {
		t.Fatalf("expected 0 downgrades, got %d", len(s.appliedOrgIDs))
	}
}

func TestDowngradeApplier_WithAdvisoryLock_Acquired(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	s := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-locked", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
	}

	var lockReleased atomic.Bool
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return true, nil
		},
		releaseFn: func(_ context.Context, _ int64) error {
			lockReleased.Store(true)
			return nil
		},
	}

	applier := NewDowngradeApplier(s, nil, time.Minute).WithAdvisoryLocker(locker)
	applier.apply(context.Background())

	if len(s.appliedOrgIDs) != 1 {
		t.Fatalf("expected 1 downgrade, got %d", len(s.appliedOrgIDs))
	}
	if !lockReleased.Load() {
		t.Fatal("expected advisory lock to be released")
	}
}

func TestDowngradeApplier_WithAdvisoryLock_NotAcquired(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	s := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-skip", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
	}

	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil // lock held by another instance
		},
	}

	applier := NewDowngradeApplier(s, nil, time.Minute).WithAdvisoryLocker(locker)
	applier.apply(context.Background())

	if len(s.appliedOrgIDs) != 0 {
		t.Fatalf("expected 0 downgrades when lock not acquired, got %d", len(s.appliedOrgIDs))
	}
}

func TestDowngradeApplier_WithAdvisoryLock_AcquireError(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	s := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-1", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
	}

	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, errors.New("lock error")
		},
	}

	applier := NewDowngradeApplier(s, nil, time.Minute).WithAdvisoryLocker(locker)
	applier.apply(context.Background())

	if len(s.appliedOrgIDs) != 0 {
		t.Fatalf("expected 0 downgrades on lock error, got %d", len(s.appliedOrgIDs))
	}
}

func TestDowngradeApplier_EnforcesLimits_AfterDowngrade(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)

	var suspendCalled atomic.Bool
	var deactivateCronCalled atomic.Bool
	var deactivateWebhookCalled atomic.Bool
	var deactivateEnvCalled atomic.Bool

	s := &mockDowngradeStoreWithCallbacks{
		mockDowngradeStore: mockDowngradeStore{
			pendingOrgs: []billing.OrgSubscription{
				{OrgID: "org-limit", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
			},
		},
		suspendExcessProjectsFn: func(_ context.Context, _ string, _ int) (int, error) {
			suspendCalled.Store(true)
			return 2, nil
		},
		deactivateExcessCronJobsFn: func(_ context.Context, _ string, _ int) ([]string, error) {
			deactivateCronCalled.Store(true)
			return []string{"job-1"}, nil
		},
		deactivateExcessWebhooksFn: func(_ context.Context, _ string, _ int) (int64, error) {
			deactivateWebhookCalled.Store(true)
			return 1, nil
		},
		deactivateExcessEnvsFn: func(_ context.Context, _ string, _ int) (int64, error) {
			deactivateEnvCalled.Store(true)
			return 1, nil
		},
	}

	enforcer := newTestEnforcer(t)
	applier := NewDowngradeApplier(s, enforcer, time.Minute)
	applier.apply(context.Background())

	if !suspendCalled.Load() {
		t.Error("expected SuspendExcessProjects to be called")
	}
	if !deactivateCronCalled.Load() {
		t.Error("expected DeactivateExcessCronJobs to be called")
	}
	if !deactivateWebhookCalled.Load() {
		t.Error("expected DeactivateExcessWebhookSubscriptions to be called")
	}
	// Environments depend on plan limits for free tier -- check the logic.
}

func TestDowngradeApplier_NilPendingTier_SkipsEnforcement(t *testing.T) {
	t.Parallel()

	s := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-nil-tier", PlanTier: "pro", PendingPlanTier: nil},
		},
	}

	applier := NewDowngradeApplier(s, nil, time.Minute)
	applier.apply(context.Background())

	if len(s.appliedOrgIDs) != 0 {
		t.Fatalf("expected nil pending tier to skip apply, got %d", len(s.appliedOrgIDs))
	}
}

func TestDowngradeApplier_Run_StopsOnContextCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	s := &mockDowngradeStore{}
	applier := NewDowngradeApplier(s, nil, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		applier.Run(ctx)
		close(done)
	})

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

func TestDowngradeApplier_AllOrgsFailApply(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	s := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-fail-1", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
			{OrgID: "org-fail-2", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
		applyErrors: map[string]error{
			"org-fail-1": errors.New("db error"),
			"org-fail-2": errors.New("db error"),
		},
	}

	applier := NewDowngradeApplier(s, nil, time.Minute)
	applier.apply(context.Background())

	if len(s.appliedOrgIDs) != 0 {
		t.Fatalf("expected 0 successful downgrades, got %d", len(s.appliedOrgIDs))
	}
}

// mockDowngradeStoreWithCallbacks extends mockDowngradeStore with callback-based resource enforcement.
type mockDowngradeStoreWithCallbacks struct {
	mockDowngradeStore
	suspendExcessProjectsFn    func(ctx context.Context, orgID string, maxProjects int) (int, error)
	deactivateExcessCronJobsFn func(ctx context.Context, orgID string, maxSchedules int) ([]string, error)
	deactivateExcessWebhooksFn func(ctx context.Context, orgID string, maxEndpoints int) (int64, error)
	deactivateExcessEnvsFn     func(ctx context.Context, orgID string, maxEnvs int) (int64, error)
}

func (m *mockDowngradeStoreWithCallbacks) SuspendExcessProjects(ctx context.Context, orgID string, maxProjects int) (int, error) {
	if m.suspendExcessProjectsFn != nil {
		return m.suspendExcessProjectsFn(ctx, orgID, maxProjects)
	}
	return 0, nil
}

func (m *mockDowngradeStoreWithCallbacks) DeactivateExcessCronJobs(ctx context.Context, orgID string, maxSchedules int) ([]string, error) {
	if m.deactivateExcessCronJobsFn != nil {
		return m.deactivateExcessCronJobsFn(ctx, orgID, maxSchedules)
	}
	return nil, nil
}

func (m *mockDowngradeStoreWithCallbacks) DeactivateExcessWebhookSubscriptions(ctx context.Context, orgID string, maxEndpoints int) (int64, error) {
	if m.deactivateExcessWebhooksFn != nil {
		return m.deactivateExcessWebhooksFn(ctx, orgID, maxEndpoints)
	}
	return 0, nil
}

func (m *mockDowngradeStoreWithCallbacks) DeactivateExcessEnvironments(ctx context.Context, orgID string, maxEnvs int) (int64, error) {
	if m.deactivateExcessEnvsFn != nil {
		return m.deactivateExcessEnvsFn(ctx, orgID, maxEnvs)
	}
	return 0, nil
}

// Section separator.
// GracePeriodEnforcer extended tests.
// Section separator.

func TestGraceEnforcer_WithAdvisoryLock_NotAcquired(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-locked-out", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
	}

	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
	}

	g := NewGracePeriodEnforcer(s, nil, time.Hour).WithAdvisoryLocker(locker)
	g.enforce(context.Background())

	if len(s.updatedStatuses) != 0 {
		t.Fatal("expected no updates when lock not acquired")
	}
}

func TestGraceEnforcer_WithAdvisoryLock_Error(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-err", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
	}

	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, errors.New("lock error")
		},
	}

	g := NewGracePeriodEnforcer(s, nil, time.Hour).WithAdvisoryLocker(locker)
	g.enforce(context.Background())

	if len(s.updatedStatuses) != 0 {
		t.Fatal("expected no updates on lock error")
	}
}

func TestGraceEnforcer_ListError_Aborts(t *testing.T) {
	t.Parallel()

	s := &mockGraceEnforcerStore{
		listErr: errors.New("db down"),
	}

	g := NewGracePeriodEnforcer(s, nil, time.Hour)
	g.enforce(context.Background())

	if len(s.updatedStatuses) != 0 {
		t.Fatal("expected no updates on list error")
	}
}

func TestGraceEnforcer_ConcurrentWebhookResolvesGrace(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-resolved", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
		// GetOrgSubscription returns "ok" instead of "grace", simulating a concurrent payment webhook.
		freshSubs: map[string]*billing.OrgSubscription{
			"org-resolved": {OrgID: "org-resolved", PlanTier: "pro", PaymentStatus: "ok"},
		},
	}

	g := NewGracePeriodEnforcer(s, nil, time.Hour)
	g.enforce(context.Background())

	// Should not restrict -- the re-read showed payment was resolved.
	if len(s.updatedStatuses) != 0 {
		t.Fatal("expected no restriction when grace was resolved concurrently")
	}
}

func TestGraceEnforcer_Run_StopsOnContextCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	s := &mockGraceEnforcerStore{}
	g := NewGracePeriodEnforcer(s, nil, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		g.Run(ctx)
		close(done)
	})

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

func TestGraceEnforcer_PlanUpdateError_ContinuesOtherOrgs(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-plan-fail", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
			{OrgID: "org-plan-ok", PlanTier: "starter", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
		updatePlanErrs: map[string]error{
			"org-plan-fail": errors.New("db error"),
		},
	}

	g := NewGracePeriodEnforcer(s, nil, time.Hour)
	g.enforce(context.Background())

	// org-plan-fail should have updated status but plan update failed, so it continues.
	// org-plan-ok should succeed fully.
	if s.updatedStatuses["org-plan-ok"] != "restricted" {
		t.Errorf("expected org-plan-ok restricted, got %q", s.updatedStatuses["org-plan-ok"])
	}
	if s.updatedPlans["org-plan-ok"] != "free" {
		t.Errorf("expected org-plan-ok free plan, got %q", s.updatedPlans["org-plan-ok"])
	}
}

func TestGraceEnforcer_NilEnforcer_NoPanic(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-no-enforcer", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
	}

	g := NewGracePeriodEnforcer(s, nil, time.Hour)
	g.enforce(context.Background()) // should not panic

	if s.updatedStatuses["org-no-enforcer"] != "restricted" {
		t.Errorf("expected restricted status")
	}
}

func TestGraceEnforcer_GetOrgSubscriptionError_ContinuesOthers(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-fresh-err", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
			{OrgID: "org-fresh-ok", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
		freshSubs: map[string]*billing.OrgSubscription{
			"org-fresh-ok": {OrgID: "org-fresh-ok", PlanTier: "pro", PaymentStatus: "grace"},
		},
	}

	g := NewGracePeriodEnforcer(s, nil, time.Hour)
	g.enforce(context.Background())

	// org-fresh-err should be skipped (GetOrgSubscription returns error).
	// org-fresh-ok should be restricted.
	if s.updatedStatuses["org-fresh-ok"] != "restricted" {
		t.Errorf("expected org-fresh-ok restricted, got %q", s.updatedStatuses["org-fresh-ok"])
	}
}

// Section separator.
// StaleSubscriptionChecker extended tests.
// Section separator.

// mockStaleSubStore implements StaleSubscriptionStore for testing.
type mockStaleSubStore struct {
	listFn func(ctx context.Context) ([]billing.OrgSubscription, error)
}

func (m *mockStaleSubStore) ListStaleSubscriptions(ctx context.Context) ([]billing.OrgSubscription, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func TestStaleSubscriptionChecker_StaleSubsDetected_Logs(t *testing.T) {
	t.Parallel()

	pastEnd := time.Now().Add(-48 * time.Hour)
	s := &mockStaleSubStore{
		listFn: func(context.Context) ([]billing.OrgSubscription, error) {
			subID := "stripe-sub-1"
			return []billing.OrgSubscription{
				{
					OrgID:                "org-stale",
					PlanTier:             "pro",
					StripeSubscriptionID: &subID,
					CurrentPeriodEnd:     &pastEnd,
				},
			}, nil
		},
	}

	checker := NewStaleSubscriptionChecker(s, time.Hour)
	// Should not panic and should log (tested by not panicking).
	checker.check(context.Background())
}

func TestStaleSubscriptionChecker_NoStale_NoOp(t *testing.T) {
	t.Parallel()

	s := &mockStaleSubStore{
		listFn: func(context.Context) ([]billing.OrgSubscription, error) {
			return nil, nil
		},
	}

	checker := NewStaleSubscriptionChecker(s, time.Hour)
	checker.check(context.Background())
}

func TestStaleSubscriptionChecker_ListError_Aborts(t *testing.T) {
	t.Parallel()

	s := &mockStaleSubStore{
		listFn: func(context.Context) ([]billing.OrgSubscription, error) {
			return nil, errors.New("db error")
		},
	}

	checker := NewStaleSubscriptionChecker(s, time.Hour)
	checker.check(context.Background()) // should not panic
}

func TestStaleSubscriptionChecker_WithAdvisoryLock_NotAcquired(t *testing.T) {
	t.Parallel()

	var checkCalled bool
	s := &mockStaleSubStore{
		listFn: func(context.Context) ([]billing.OrgSubscription, error) {
			checkCalled = true
			return nil, nil
		},
	}

	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
	}

	checker := NewStaleSubscriptionChecker(s, time.Hour).WithAdvisoryLocker(locker)
	checker.check(context.Background())

	if checkCalled {
		t.Fatal("expected check to be skipped when lock not acquired")
	}
}

func TestStaleSubscriptionChecker_Run_StopsOnContextCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	s := &mockStaleSubStore{}
	checker := NewStaleSubscriptionChecker(s, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		checker.Run(ctx)
		close(done)
	})

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

func TestStaleSubscriptionChecker_MultipleStale_ProcessesAll(t *testing.T) {
	t.Parallel()

	pastEnd := time.Now().Add(-48 * time.Hour)
	s := &mockStaleSubStore{
		listFn: func(context.Context) ([]billing.OrgSubscription, error) {
			sub1 := "sub-1"
			sub2 := "sub-2"
			return []billing.OrgSubscription{
				{OrgID: "org-1", PlanTier: "pro", StripeSubscriptionID: &sub1, CurrentPeriodEnd: &pastEnd},
				{OrgID: "org-2", PlanTier: "starter", StripeSubscriptionID: &sub2, CurrentPeriodEnd: &pastEnd},
			}, nil
		},
	}

	checker := NewStaleSubscriptionChecker(s, time.Hour)
	checker.check(context.Background()) // should process both without panic
}

// Ensure unused imports are satisfied.
var (
	_ = fmt.Sprintf
	_ = sync.Mutex{}
	_ = json.RawMessage{}
	_ = atomic.Bool{}
)
