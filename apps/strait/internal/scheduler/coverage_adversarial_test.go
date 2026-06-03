package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
)

// Grace period enforcer: Run loop, WithAdvisoryLocker, deeper enforce paths

func TestGraceEnforcer_Run_StopsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-1", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
	}
	g := NewGracePeriodEnforcer(s, nil, 50*time.Millisecond)

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

func TestGraceEnforcer_WithAdvisoryLocker_Acquired(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-1", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
	}
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return true, nil
		},
	}
	g := NewGracePeriodEnforcer(s, nil, time.Minute).WithAdvisoryLocker(locker)
	g.enforce(context.Background())

	if s.updatedStatuses["org-1"] != "restricted" {
		t.Errorf("expected restricted status, got %q", s.updatedStatuses["org-1"])
	}
}

func TestGraceEnforcer_WithAdvisoryLocker_NotAcquired(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-1", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
	}
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
	}
	g := NewGracePeriodEnforcer(s, nil, time.Minute).WithAdvisoryLocker(locker)
	g.enforce(context.Background())

	if len(s.updatedStatuses) != 0 {
		t.Error("expected no status updates when lock not acquired")
	}
}

func TestGraceEnforcer_WithAdvisoryLocker_Error(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-1", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
	}
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, errors.New("pg down")
		},
	}
	g := NewGracePeriodEnforcer(s, nil, time.Minute).WithAdvisoryLocker(locker)
	g.enforce(context.Background())

	if len(s.updatedStatuses) != 0 {
		t.Error("expected no status updates on locker error")
	}
}

func TestGraceEnforcer_ListError(t *testing.T) {
	t.Parallel()

	s := &mockGraceEnforcerStore{
		listErr: errors.New("db down"),
	}
	g := NewGracePeriodEnforcer(s, nil, time.Minute)
	g.enforce(context.Background())
	// Should not panic.
}

func TestGraceEnforcer_GetOrgSubscriptionError(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &covMockGraceStoreWithGetError{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-phantom", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
	}
	g := NewGracePeriodEnforcer(s, nil, time.Minute)
	g.enforce(context.Background())

	if len(s.updatedStatuses) != 0 {
		t.Error("expected no updates when GetOrgSubscription fails")
	}
}

// covMockGraceStoreWithGetError returns an error from GetOrgSubscription.
type covMockGraceStoreWithGetError struct {
	graceOrgs       []billing.OrgSubscription
	updatedStatuses map[string]string
}

func (m *covMockGraceStoreWithGetError) ListOrgsInGracePeriod(_ context.Context) ([]billing.OrgSubscription, error) {
	return m.graceOrgs, nil
}

func (m *covMockGraceStoreWithGetError) GetOrgSubscription(_ context.Context, _ string) (*billing.OrgSubscription, error) {
	return nil, errors.New("subscription not found")
}

func (m *covMockGraceStoreWithGetError) UpdatePaymentStatus(_ context.Context, orgID string, status string, _ *time.Time) error {
	if m.updatedStatuses == nil {
		m.updatedStatuses = make(map[string]string)
	}
	m.updatedStatuses[orgID] = status
	return nil
}

func (m *covMockGraceStoreWithGetError) UpdateOrgSubscriptionPlan(_ context.Context, _ string, _ string, _ string) error {
	return nil
}

func (m *covMockGraceStoreWithGetError) RestrictExpiredGracePeriod(_ context.Context, orgID string, _ *time.Time) (bool, error) {
	if m.updatedStatuses == nil {
		m.updatedStatuses = make(map[string]string)
	}
	m.updatedStatuses[orgID] = "restricted"
	return true, nil
}

func (m *covMockGraceStoreWithGetError) SuspendExcessProjects(context.Context, string, int) (int, error) {
	return 0, nil
}

func (m *covMockGraceStoreWithGetError) DeactivateExcessCronJobs(context.Context, string, int) ([]string, error) {
	return nil, nil
}

func (m *covMockGraceStoreWithGetError) DeactivateExcessWebhookSubscriptions(context.Context, string, int) (int64, error) {
	return 0, nil
}

func (m *covMockGraceStoreWithGetError) DeactivateExcessEnvironments(context.Context, string, int) (int64, error) {
	return 0, nil
}

func (m *covMockGraceStoreWithGetError) DeactivateExcessLogDrains(context.Context, string, int) (int64, error) {
	return 0, nil
}

func (m *covMockGraceStoreWithGetError) DeactivateExcessNotificationChannelsByProject(context.Context, string, int) (int64, error) {
	return 0, nil
}

func (m *covMockGraceStoreWithGetError) ListProjectsByOrg(context.Context, string) ([]string, error) {
	return nil, nil
}

func (m *covMockGraceStoreWithGetError) PauseHTTPJobsByOrg(context.Context, string, string) ([]string, error) {
	return nil, nil
}

func (m *covMockGraceStoreWithGetError) CountMembersByOrg(context.Context, string) (int, error) {
	return 0, nil
}

func TestGraceEnforcer_ConcurrentlyResolved(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-resolved", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
		freshSubs: map[string]*billing.OrgSubscription{
			// Fresh read shows payment was resolved between list and enforce.
			"org-resolved": {OrgID: "org-resolved", PlanTier: "pro", PaymentStatus: "active", GracePeriodEnd: &pastGrace},
		},
	}
	g := NewGracePeriodEnforcer(s, nil, time.Minute)
	g.enforce(context.Background())

	if len(s.updatedStatuses) != 0 {
		t.Error("expected no updates when org was concurrently resolved")
	}
}

func TestGraceEnforcer_UpdatePlanError(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-plan-err", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
		updatePlanErrs: map[string]error{
			"org-plan-err": errors.New("plan update failed"),
		},
	}
	g := NewGracePeriodEnforcer(s, nil, time.Minute)
	g.enforce(context.Background())

	if len(s.updatedStatuses) != 0 {
		t.Error("expected atomic restriction failure to leave status unchanged")
	}
}

func TestGraceEnforcer_WithEnforcer(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-enforcer", PlanTier: "starter", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
	}
	enforcer := newTestEnforcer(t)
	g := NewGracePeriodEnforcer(s, enforcer, time.Minute)
	g.enforce(context.Background())

	if s.updatedStatuses["org-enforcer"] != "restricted" {
		t.Error("expected restricted status with enforcer")
	}
	if s.updatedPlans["org-enforcer"] != "free" {
		t.Error("expected free plan with enforcer")
	}
}

// Usage flusher: Run loop, advisory locker paths, upsert errors

func TestUsageFlusher_Run_StopsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	s := &mockUsageFlusherStore{}
	uf := NewUsageFlusher(s, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		uf.Run(ctx)
		close(done)
	})

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

func TestUsageFlusher_AdvisoryLocker_NotAcquired(t *testing.T) {
	t.Parallel()

	var flushed bool
	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			flushed = true
			return nil, nil
		},
	}
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
	}
	uf := NewUsageFlusher(s, time.Minute).WithAdvisoryLocker(locker)
	uf.flush(context.Background())

	if flushed {
		t.Fatal("expected flush to be skipped when lock not acquired")
	}
}

func TestUsageFlusher_AdvisoryLocker_Error(t *testing.T) {
	t.Parallel()

	var flushed bool
	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			flushed = true
			return nil, nil
		},
	}
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, errors.New("pg connection lost")
		},
	}
	uf := NewUsageFlusher(s, time.Minute).WithAdvisoryLocker(locker)
	uf.flush(context.Background())

	if flushed {
		t.Fatal("expected flush to be skipped on locker error")
	}
}

func TestUsageFlusher_AdvisoryLocker_Acquired(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	var upsertCount atomic.Int32
	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgDailyUsageFn: func(_ context.Context, _ string, _ time.Time) ([]billing.UsageRecord, error) {
			return []billing.UsageRecord{
				{OrgID: "org-1", ProjectID: "proj-1", PeriodDate: today, RunsCount: 5},
			}, nil
		},
		replaceUsageRecordFn: func(_ context.Context, _ *billing.UsageRecord) error {
			upsertCount.Add(1)
			return nil
		},
	}
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return true, nil
		},
	}
	uf := NewUsageFlusher(s, time.Minute).WithAdvisoryLocker(locker)
	uf.flush(context.Background())

	if upsertCount.Load() != usageFlusherReconcileLookbackDays {
		t.Fatalf("expected %d upserts across lookback, got %d", usageFlusherReconcileLookbackDays, upsertCount.Load())
	}
}

func TestUsageFlusher_ListOrgsError(t *testing.T) {
	t.Parallel()

	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			return nil, errors.New("db down")
		},
	}
	uf := NewUsageFlusher(s, time.Minute)
	uf.flush(context.Background())
	// Should not panic.
}

func TestUsageFlusher_UpsertError_ContinuesOtherRecords(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	var mu sync.Mutex
	upsertedProjects := make(map[string]bool)

	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgDailyUsageFn: func(_ context.Context, _ string, _ time.Time) ([]billing.UsageRecord, error) {
			return []billing.UsageRecord{
				{OrgID: "org-1", ProjectID: "proj-fail", PeriodDate: today, RunsCount: 1},
				{OrgID: "org-1", ProjectID: "proj-ok", PeriodDate: today, RunsCount: 2},
			}, nil
		},
		replaceUsageRecordFn: func(_ context.Context, rec *billing.UsageRecord) error {
			if rec.ProjectID == "proj-fail" {
				return errors.New("upsert failed")
			}
			mu.Lock()
			upsertedProjects[rec.ProjectID] = true
			mu.Unlock()
			return nil
		},
	}
	uf := NewUsageFlusher(s, time.Minute)
	uf.flush(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if !upsertedProjects["proj-ok"] {
		t.Error("expected proj-ok to be upserted despite proj-fail error")
	}
}

// Downgrade applier: Run loop, SuspendExcessProjects path

func TestDowngradeApplier_Run_StopsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	store := &advMockDowngradeStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			return nil, nil
		},
	}
	d := NewDowngradeApplier(store, nil, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		d.Run(ctx)
		close(done)
	})

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

func TestDowngradeApplier_SuspendExcessProjects(t *testing.T) {
	t.Parallel()

	free := "free"
	var suspended bool
	store := &advMockDowngradeStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			return []billing.OrgSubscription{
				{OrgID: "org-1", PlanTier: "pro", PendingPlanTier: &free},
			}, nil
		},
		suspendFn: func(_ context.Context, _ string, _ int) (int, error) {
			suspended = true
			return 2, nil
		},
	}
	d := NewDowngradeApplier(store, nil, time.Minute)
	d.apply(context.Background())

	if !suspended {
		t.Error("expected SuspendExcessProjects to be called")
	}
}

func TestDowngradeApplier_SuspendExcessProjects_Error(t *testing.T) {
	t.Parallel()

	free := "free"
	store := &advMockDowngradeStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			return []billing.OrgSubscription{
				{OrgID: "org-1", PlanTier: "pro", PendingPlanTier: &free},
			}, nil
		},
		suspendFn: func(_ context.Context, _ string, _ int) (int, error) {
			return 0, errors.New("suspend failed")
		},
	}
	d := NewDowngradeApplier(store, nil, time.Minute)
	// Should not panic.
	d.apply(context.Background())
}

func TestDowngradeApplier_WithEnforcer_InvalidatesCache(t *testing.T) {
	t.Parallel()

	free := "free"
	store := &advMockDowngradeStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			return []billing.OrgSubscription{
				{OrgID: "org-1", PlanTier: "pro", PendingPlanTier: &free},
			}, nil
		},
	}
	enforcer := newTestEnforcer(t)
	d := NewDowngradeApplier(store, enforcer, time.Minute)
	// Should not panic even with enforcer.
	d.apply(context.Background())
}

func TestDowngradeApplier_NilPendingPlanTier(t *testing.T) {
	t.Parallel()

	store := &advMockDowngradeStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			return []billing.OrgSubscription{
				{OrgID: "org-1", PlanTier: "pro", PendingPlanTier: nil},
			}, nil
		},
	}
	d := NewDowngradeApplier(store, nil, time.Minute)
	// PendingPlanTier is nil -- should skip SuspendExcessProjects without panic.
	d.apply(context.Background())
}

func TestDowngradeApplier_LockerReleaseError(t *testing.T) {
	t.Parallel()

	store := &advMockDowngradeStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			return nil, nil
		},
	}
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return true, nil
		},
		releaseFn: func(_ context.Context, _ int64) error {
			return errors.New("release failed")
		},
	}
	d := NewDowngradeApplier(store, nil, time.Minute).WithAdvisoryLocker(locker)
	// Should not panic on release error.
	d.apply(context.Background())
}

// Budget monitor: checkRunLimitWarnings, WithRunLimitNotifications

// mockRunLimitStore implements RunLimitStore for testing.
type covMockRunLimitStore struct {
	listAllSubscribedOrgIDsFn                    func(ctx context.Context) ([]string, error)
	listProjectsByOrgFn                          func(ctx context.Context, orgID string) ([]string, error)
	listEnabledNotificationChannelsFn            func(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	listEnabledNotificationChannelsByProjectIDFn func(ctx context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error)
	createNotificationDeliveryFn                 func(ctx context.Context, d *domain.NotificationDelivery) error
}

func (m *covMockRunLimitStore) ListAllSubscribedOrgIDs(ctx context.Context) ([]string, error) {
	if m.listAllSubscribedOrgIDsFn != nil {
		return m.listAllSubscribedOrgIDsFn(ctx)
	}
	return nil, nil
}

func (m *covMockRunLimitStore) ListProjectsByOrg(ctx context.Context, orgID string) ([]string, error) {
	if m.listProjectsByOrgFn != nil {
		return m.listProjectsByOrgFn(ctx, orgID)
	}
	return nil, nil
}

func (m *covMockRunLimitStore) ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error) {
	if m.listEnabledNotificationChannelsFn != nil {
		return m.listEnabledNotificationChannelsFn(ctx, projectID)
	}
	return nil, nil
}

func (m *covMockRunLimitStore) ListEnabledNotificationChannelsByProjectIDs(ctx context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error) {
	if m.listEnabledNotificationChannelsByProjectIDFn != nil {
		return m.listEnabledNotificationChannelsByProjectIDFn(ctx, projectIDs)
	}
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

func (m *covMockRunLimitStore) CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error {
	if m.createNotificationDeliveryFn != nil {
		return m.createNotificationDeliveryFn(ctx, d)
	}
	return nil
}

// covMockEnforcer is a minimal enforcer that returns controllable results for Check80PercentMonthlyWarning.
// Since billing.Enforcer is a concrete type, we use a real enforcer via newTestEnforcer.

func TestBudgetMonitor_WithRunLimitNotifications(t *testing.T) {
	t.Parallel()

	rl := &covMockRunLimitStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
	}
	enforcer := newTestEnforcer(t)
	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithRunLimitNotifications(rl, enforcer)

	if bm.runLimitStore == nil {
		t.Fatal("expected runLimitStore to be set")
	}
	if bm.enforcer == nil {
		t.Fatal("expected enforcer to be set")
	}
}

func TestBudgetMonitor_CheckRunLimitWarnings_ListOrgsError(t *testing.T) {
	t.Parallel()

	rl := &covMockRunLimitStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			return nil, errors.New("db error")
		},
	}
	enforcer := newTestEnforcer(t)
	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithRunLimitNotifications(rl, enforcer)
	// Should not panic.
	bm.check(context.Background())
}

func TestBudgetMonitor_CheckRunLimitWarnings_NoWarning(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	rl := &covMockRunLimitStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			return []string{"org-1"}, nil
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
	enforcer := newTestEnforcer(t)
	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithRunLimitNotifications(rl, enforcer)
	bm.check(context.Background())

	// The enforcer will return false for Check80PercentMonthlyWarning with the mock store,
	// so no notifications should be sent.
	if len(deliveries) != 0 {
		t.Fatalf("expected no deliveries when run limit is not near, got %d", len(deliveries))
	}
}

func TestBudgetMonitor_CheckRunLimitWarnings_Dedup(t *testing.T) {
	t.Parallel()

	currentMonth := time.Now().UTC().Format("2006-01")
	rl := &covMockRunLimitStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
	}
	enforcer := newTestEnforcer(t)
	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithRunLimitNotifications(rl, enforcer)

	// Pre-set the alert key as if it was already alerted.
	alertKey := fmt.Sprintf("runlimit:%s:80:%s", "org-1", currentMonth)
	bm.alertedMu.Lock()
	bm.alerted[alertKey] = true
	bm.alertedMu.Unlock()

	bm.check(context.Background())
	// Already alerted, should be skipped without error.
}

// Concurrent reconciler: Run loop

func TestConcurrentReconciler_Run_StopsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	enforcer := newTestEnforcer(t)
	r := NewConcurrentReconciler(enforcer, nil, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		r.Run(ctx)
		close(done)
	})

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

// Cron: recordCronDrift, concurrent trigger races

func TestCronScheduler_RecordCronDrift_EmptyExpr(t *testing.T) {
	t.Parallel()

	cs := NewCronScheduler(context.Background(), &mockCronStore{}, &mockQueue{}, nil)
	// Empty cron expression should be a no-op.
	cs.recordCronDrift(context.Background(), "")
}

func TestCronScheduler_RecordCronDrift_InvalidExpr(t *testing.T) {
	t.Parallel()

	cs := NewCronScheduler(context.Background(), &mockCronStore{}, &mockQueue{}, nil)
	// Invalid expression should not panic.
	cs.recordCronDrift(context.Background(), "INVALID_EXPR")
}

func TestCronScheduler_RecordCronDrift_NilMetrics(t *testing.T) {
	t.Parallel()

	cs := NewCronScheduler(context.Background(), &mockCronStore{}, &mockQueue{}, nil)
	// Nil metrics should be a no-op.
	cs.recordCronDrift(context.Background(), "* * * * *")
}

func TestCronScheduler_ConcurrentTriggerSameJob(t *testing.T) {
	t.Parallel()

	var enqueued atomic.Int32
	ms := &mockCronStore{}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued.Add(1)
			return nil
		},
	}
	cs := NewCronScheduler(context.Background(), ms, q, nil)

	job := domain.Job{
		ID:                "job-race",
		ProjectID:         "p1",
		CronOverlapPolicy: domain.OverlapPolicyAllow,
		Cron:              "* * * * *",
	}

	var wg conc.WaitGroup
	for range 20 {
		wg.Go(func() {
			cs.triggerJob(context.Background(), job)
		})
	}
	wg.Wait()

	if enqueued.Load() != 1 {
		t.Fatalf("expected one durable cron fire enqueue, got %d", enqueued.Load())
	}
}

func TestCronScheduler_DurableFireKeySkipsWorkflowAfterLockRelease(t *testing.T) {
	t.Parallel()

	var triggered atomic.Int32
	ms := &mockCronStore{}
	wt := &mockWorkflowTrigger{
		triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
			triggered.Add(1)
			return &domain.WorkflowRun{ID: "wf-run-1"}, nil
		},
	}

	wf := domain.Workflow{ID: "wf-race", ProjectID: "p1", Cron: "* * * * *"}
	NewCronScheduler(context.Background(), ms, &mockQueue{}, wt).triggerWorkflow(context.Background(), wf)
	NewCronScheduler(context.Background(), ms, &mockQueue{}, wt).triggerWorkflow(context.Background(), wf)

	if triggered.Load() != 1 {
		t.Fatalf("expected one durable cron workflow fire, got %d", triggered.Load())
	}
}

func TestCronScheduler_TriggerJob_NoTTL_NilExpiresAt(t *testing.T) {
	t.Parallel()

	var capturedRun *domain.JobRun
	ms := &mockCronStore{}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, r *domain.JobRun) error {
			capturedRun = r
			return nil
		},
	}
	cs := NewCronScheduler(context.Background(), ms, q, nil)

	job := domain.Job{
		ID:         "job-no-ttl",
		ProjectID:  "p1",
		RunTTLSecs: 0,
		Cron:       "* * * * *",
	}
	cs.triggerJob(context.Background(), job)

	if capturedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	if capturedRun.ExpiresAt != nil {
		t.Error("expected nil ExpiresAt when no TTL configured")
	}
}

func TestCronScheduler_TriggerWorkflow_WithTimezone(t *testing.T) {
	t.Parallel()

	var triggered atomic.Int32
	wt := &mockWorkflowTrigger{
		triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
			triggered.Add(1)
			return &domain.WorkflowRun{}, nil
		},
	}
	ms := &mockCronStore{
		listCronWorkflowsFn: func(_ context.Context) ([]domain.Workflow, error) {
			return []domain.Workflow{
				{ID: "wf-tz", Cron: "0 9 * * *", CronTimezone: "America/New_York", ProjectID: "p1"},
			}, nil
		},
	}
	cs := NewCronScheduler(context.Background(), ms, &mockQueue{}, wt)
	err := cs.LoadJobs(context.Background())
	if err != nil {
		t.Fatalf("expected LoadJobs to succeed with timezone, got: %v", err)
	}
}

func TestCronScheduler_TriggerWorkflow_SkipIfRunning_ActiveRuns(t *testing.T) {
	t.Parallel()

	var triggered atomic.Int32
	wt := &mockWorkflowTrigger{
		triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
			triggered.Add(1)
			return &domain.WorkflowRun{}, nil
		},
	}
	ms := &mockCronStore{
		countRunningWfRunsFn: func(_ context.Context, _ string) (int, error) {
			return 2, nil // 2 running
		},
	}
	cs := NewCronScheduler(context.Background(), ms, &mockQueue{}, wt)

	wf := domain.Workflow{
		ID:            "wf-skip-active",
		ProjectID:     "p1",
		Cron:          "* * * * *",
		SkipIfRunning: true,
	}
	cs.triggerWorkflow(context.Background(), wf)

	if triggered.Load() != 0 {
		t.Fatal("expected SkipIfRunning to prevent trigger when runs are active")
	}
}

func TestCronScheduler_LoadJobs_ListJobsError(t *testing.T) {
	t.Parallel()

	ms := &mockCronStore{
		listCronJobsFn: func(_ context.Context) ([]domain.Job, error) {
			return nil, errors.New("job list failed")
		},
	}
	cs := NewCronScheduler(context.Background(), ms, &mockQueue{}, nil)
	err := cs.LoadJobs(context.Background())
	if err == nil {
		t.Fatal("expected error when ListCronJobs fails")
	}
}

// Batch flusher: GetJob error, enqueue error during flush, advisory lock races

func TestBatchFlusher_GetJobReturnsError(t *testing.T) {
	t.Parallel()

	bs := &covMockBatchStoreWithJobError{
		flushable: []store.FlushableBatch{
			{JobID: "job-missing", ProjectID: "proj-1", BatchKey: "", ItemCount: 1},
		},
	}
	var enqueued atomic.Int32
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued.Add(1)
			return nil
		},
	}
	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())

	if enqueued.Load() != 0 {
		t.Fatal("expected no enqueue when job lookup returns error")
	}
}

// covMockBatchStoreWithJobError is a batch store that returns an error from GetJob.
type covMockBatchStoreWithJobError struct {
	flushable []store.FlushableBatch
}

func (m *covMockBatchStoreWithJobError) ListFlushableBatches(_ context.Context) ([]store.FlushableBatch, error) {
	return m.flushable, nil
}

func (m *covMockBatchStoreWithJobError) DrainBatchBuffer(_ context.Context, _, _ string, _ int) ([]domain.BatchBufferItem, error) {
	return nil, nil
}

func (m *covMockBatchStoreWithJobError) ListBatchBufferItems(_ context.Context, _, _ string, _ int) ([]domain.BatchBufferItem, error) {
	return nil, nil
}

func (m *covMockBatchStoreWithJobError) DeleteBatchBufferItems(_ context.Context, _ []string) error {
	return nil
}

func (m *covMockBatchStoreWithJobError) GetJob(_ context.Context, _ string) (*domain.Job, error) {
	return nil, errors.New("job not found")
}

func (m *covMockBatchStoreWithJobError) CreateRun(_ context.Context, _ *domain.JobRun) error {
	return nil
}

func (m *covMockBatchStoreWithJobError) TryAdvisoryLock(_ context.Context, _ int64) (bool, error) {
	return true, nil
}

func (m *covMockBatchStoreWithJobError) ReleaseAdvisoryLock(_ context.Context, _ int64) error {
	return nil
}

func TestBatchFlusher_EnqueueError_Continues(t *testing.T) {
	t.Parallel()

	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "a", ItemCount: 1},
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "b", ItemCount: 1},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "i1", Payload: json.RawMessage(`{}`), CreatedBy: "u1"},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, TimeoutSecs: 60, BatchMaxSize: 10},
		},
	}
	var errorCount, successCount atomic.Int32
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			if errorCount.Add(1) == 1 {
				return errors.New("queue full")
			}
			successCount.Add(1)
			return nil
		},
	}
	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())

	// First batch should fail, second should succeed.
	if successCount.Load() != 1 {
		t.Fatalf("expected 1 successful enqueue after 1 error, got %d", successCount.Load())
	}
}

func TestBatchFlusher_LargeBatch(t *testing.T) {
	t.Parallel()

	const batchSize = 100
	items := make([]domain.BatchBufferItem, batchSize)
	for i := range batchSize {
		items[i] = domain.BatchBufferItem{
			ID:        fmt.Sprintf("i-%d", i),
			Payload:   json.RawMessage(fmt.Sprintf(`{"idx":%d}`, i)),
			CreatedBy: "u1",
		}
	}

	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "", ItemCount: batchSize},
		},
		drainedItems: items,
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, TimeoutSecs: 60, BatchMaxSize: batchSize},
		},
	}

	var capturedRun *domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}
	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())

	if capturedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	var payload map[string]any
	if err := json.Unmarshal(capturedRun.Payload, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	payloadItems := payload["items"].([]any)
	if len(payloadItems) != batchSize {
		t.Fatalf("expected %d items in payload, got %d", batchSize, len(payloadItems))
	}
}

// Debounce poller: TTL edge cases

func TestDebouncePoller_CustomTTL(t *testing.T) {
	t.Parallel()

	ttl := 120
	ds := &mockDebounceStore{
		duePending: []domain.DebouncePending{
			{
				ID:        "dp-1",
				JobID:     "job-1",
				ProjectID: "proj-1",
				Payload:   json.RawMessage(`{}`),
				TTLSecs:   &ttl,
				CreatedBy: "u1",
				FireAt:    time.Now().Add(-time.Second),
			},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, TimeoutSecs: 300, RunTTLSecs: 600},
		},
	}
	var capturedRun *domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}
	poller := NewDebouncePoller(ds, q, time.Second)
	poller.poll(context.Background())

	if capturedRun == nil || capturedRun.ExpiresAt == nil {
		t.Fatal("expected run with ExpiresAt")
	}
	delta := time.Until(*capturedRun.ExpiresAt)
	// TTLSecs=120, so ExpiresAt should be roughly 2 minutes from now.
	if delta < 100*time.Second || delta > 140*time.Second {
		t.Fatalf("expected ExpiresAt ~2m from now, got %v", delta)
	}
}

func TestDebouncePoller_JobTTL_Fallback(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{
		duePending: []domain.DebouncePending{
			{
				ID:        "dp-1",
				JobID:     "job-1",
				ProjectID: "proj-1",
				Payload:   json.RawMessage(`{}`),
				TTLSecs:   nil,
				CreatedBy: "u1",
				FireAt:    time.Now().Add(-time.Second),
			},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, TimeoutSecs: 300, RunTTLSecs: 600},
		},
	}
	var capturedRun *domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}
	poller := NewDebouncePoller(ds, q, time.Second)
	poller.poll(context.Background())

	if capturedRun == nil || capturedRun.ExpiresAt == nil {
		t.Fatal("expected run with ExpiresAt")
	}
	delta := time.Until(*capturedRun.ExpiresAt)
	// RunTTLSecs=600, so ExpiresAt should be roughly 10 minutes from now.
	if delta < 9*time.Minute || delta > 11*time.Minute {
		t.Fatalf("expected ExpiresAt ~10m from now, got %v", delta)
	}
}

func TestDebouncePoller_TimeoutFallback(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{
		duePending: []domain.DebouncePending{
			{
				ID:        "dp-1",
				JobID:     "job-1",
				ProjectID: "proj-1",
				Payload:   json.RawMessage(`{}`),
				TTLSecs:   nil,
				CreatedBy: "u1",
				FireAt:    time.Now().Add(-time.Second),
			},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, TimeoutSecs: 120, RunTTLSecs: 0},
		},
	}
	var capturedRun *domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}
	poller := NewDebouncePoller(ds, q, time.Second)
	poller.poll(context.Background())

	if capturedRun == nil || capturedRun.ExpiresAt == nil {
		t.Fatal("expected run with ExpiresAt")
	}
	delta := time.Until(*capturedRun.ExpiresAt)
	// TimeoutSecs=120 + 60 = 180s = 3 minutes.
	if delta < 2*time.Minute || delta > 4*time.Minute {
		t.Fatalf("expected ExpiresAt ~3m from now, got %v", delta)
	}
}

func TestDebouncePoller_WithTags(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{
		duePending: []domain.DebouncePending{
			{
				ID:        "dp-1",
				JobID:     "job-1",
				ProjectID: "proj-1",
				Payload:   json.RawMessage(`{}`),
				Tags:      json.RawMessage(`{"env":"prod","region":"us"}`),
				CreatedBy: "u1",
				FireAt:    time.Now().Add(-time.Second),
			},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, TimeoutSecs: 60},
		},
	}
	var capturedRun *domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}
	poller := NewDebouncePoller(ds, q, time.Second)
	poller.poll(context.Background())

	if capturedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	if capturedRun.Tags["env"] != "prod" || capturedRun.Tags["region"] != "us" {
		t.Fatalf("expected tags {env:prod, region:us}, got %v", capturedRun.Tags)
	}
}

func TestDebouncePoller_Run_StopsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	ds := &mockDebounceStore{}
	q := &mockQueue{}
	poller := NewDebouncePoller(ds, q, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		poller.Run(ctx)
		close(done)
	})

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

// Stale subscription checker: locker error path

func TestStaleSubscriptionChecker_Check_LockerError(t *testing.T) {
	t.Parallel()
	var checked bool
	s := &advMockStaleSubStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			checked = true
			return nil, nil
		},
	}
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, errors.New("pg error")
		},
	}
	c := NewStaleSubscriptionChecker(s, time.Minute).WithAdvisoryLocker(locker)
	c.check(context.Background())
	if checked {
		t.Fatal("expected check skipped on locker error")
	}
}

// SLO error budget: adversarial float inputs

func TestCalculateErrorBudget_ExtremeValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current float64
		target  float64
		metric  string
	}{
		{"very_small_success", 1e-15, 0.99, domain.SLOMetricSuccessRate},
		{"very_large_success", 1e15, 0.99, domain.SLOMetricSuccessRate},
		{"negative_target_latency", 1.0, -1.0, domain.SLOMetricP95LatencySecs},
		{"max_float_success", math.MaxFloat64, 0.99, domain.SLOMetricSuccessRate},
		{"max_float_latency", math.MaxFloat64, math.MaxFloat64, domain.SLOMetricP95LatencySecs},
		{"smallest_positive_latency", math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64, domain.SLOMetricP95LatencySecs},
		{"both_zero_latency", 0.0, 0.0, domain.SLOMetricP95LatencySecs},
		{"both_one_success", 1.0, 1.0, domain.SLOMetricSuccessRate},
		{"slightly_over_one", 1.001, 1.0, domain.SLOMetricSuccessRate},
		{"unknown_metric_with_values", 42.0, 99.0, "custom_metric_xyz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			budget := CalculateErrorBudget(tt.current, tt.target, tt.metric)
			if math.IsInf(budget, 0) {
				t.Errorf("budget should not be Inf for %s", tt.name)
			}
			if budget < 0 || budget > 1 {
				if tt.metric != "custom_metric_xyz" {
					t.Errorf("budget out of [0,1] for %s: %v", tt.name, budget)
				}
			}
		})
	}
}

// Anomaly monitor: release lock error, detection error, notification paths

func TestAnomalyMonitor_AdvisoryLockerReleaseError(t *testing.T) {
	t.Parallel()

	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			return nil, nil
		},
	}
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return true, nil
		},
		releaseFn: func(_ context.Context, _ int64) error {
			return errors.New("release failed")
		},
	}
	am := NewAnomalyMonitor(s, time.Minute).WithAdvisoryLocker(locker)
	// Should not panic on release error.
	am.check(context.Background())
}

func TestAnomalyMonitor_Run_StopsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			return nil, nil
		},
	}
	am := NewAnomalyMonitor(s, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		am.Run(ctx)
		close(done)
	})

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

// Budget monitor: spending limit edge cases

func TestBudgetMonitor_SpendingLimit_NilPeriodStart_FallbackToNow(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return &billing.OrgSubscription{
				OrgID:                 "org-1",
				PlanTier:              "starter",
				SpendingLimitMicrousd: 100_000_000,
				LimitAction:           "notify",
				CurrentPeriodStart:    nil, // nil period start
			}, nil
		},
		sumOrgPeriodSpendFn: func(_ context.Context, _ string, _ time.Time) (int64, error) {
			// Return enough to trigger 100% (overage >= limit).
			return 200_000_000, nil
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
	bm.check(context.Background())

	// With nil period start, it should use time.Now() as fallback and still alert.
	if len(deliveries) == 0 {
		t.Fatal("expected at least one delivery with nil period start")
	}
}

func TestBudgetMonitor_SpendingLimit_SubError(t *testing.T) {
	t.Parallel()

	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return nil, errors.New("subscription not found")
		},
	}

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	// Should not panic when GetOrgSubscription returns error.
	bm.check(context.Background())
}

func TestBudgetMonitor_SpendingLimit_SpendQueryTimeout(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return &billing.OrgSubscription{
				OrgID:                 "org-1",
				PlanTier:              "starter",
				SpendingLimitMicrousd: 100_000_000,
				CurrentPeriodStart:    &now,
			}, nil
		},
		sumOrgPeriodSpendFn: func(_ context.Context, _ string, _ time.Time) (int64, error) {
			return 0, errors.New("query timeout")
		},
	}

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	// Should not panic on spend query error.
	bm.check(context.Background())
}

func TestBudgetMonitor_SpendingLimit_BulkChannelError(t *testing.T) {
	t.Parallel()

	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return newSpendingLimitSub("org-1", 100_000_000, "starter"), nil
		},
		sumOrgPeriodSpendFn: func(_ context.Context, _ string, _ time.Time) (int64, error) {
			// Trigger 100%.
			return 200_000_000, nil
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return nil, errors.New("channel query failed")
		},
	}

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	// Should not panic when channel bulk-fetch fails.
	bm.check(context.Background())
}

func TestBudgetMonitor_SpendingLimit_ProjectListError(t *testing.T) {
	t.Parallel()

	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return newSpendingLimitSub("org-1", 100_000_000, "starter"), nil
		},
		sumOrgPeriodSpendFn: func(_ context.Context, _ string, _ time.Time) (int64, error) {
			return 200_000_000, nil
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, errors.New("project list failed")
		},
	}

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	// Should not panic when project list fails.
	bm.check(context.Background())
}

// safeGo panic recovery

func TestSafeGo_RecoversPanic(t *testing.T) {
	// Not parallel: mutates package-level exitFunc.

	var exitCode int
	var exitCalled bool
	origExit := exitFunc
	exitFunc = func(code int) {
		exitCode = code
		exitCalled = true
	}
	defer func() { exitFunc = origExit }()

	var wg conc.WaitGroup
	safeGo(&wg, "test-panic", func() {
		panic("test panic in safeGo")
	})
	wg.Wait()

	if !exitCalled {
		t.Fatal("expected exitFunc to be called on panic")
	}
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
}

// Memory cleanup: Run loop

func TestMemoryCleanup_Run_StopsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	s := &advMockMemoryStore{}
	mc := NewMemoryCleanup(s, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		mc.Run(ctx)
		close(done)
	})

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

// Spending limit notification: CreateNotificationDelivery error

func TestBudgetMonitor_SpendingNotification_DeliveryError(t *testing.T) {
	t.Parallel()

	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return newSpendingLimitSub("org-1", 100_000_000, "starter"), nil
		},
		sumOrgPeriodSpendFn: func(_ context.Context, _ string, _ time.Time) (int64, error) {
			return 200_000_000, nil // Over 100%.
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{
				{ID: "ch-1", ProjectID: "proj-1", ChannelType: domain.ChannelTypeWebhook},
			}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			return errors.New("delivery creation failed")
		},
	}

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	// Should not panic when delivery creation fails.
	bm.check(context.Background())
}

// Spending limit dedup across both 80% and 100%

func TestBudgetMonitor_SpendingLimit_100Then80_DedupCheck(t *testing.T) {
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
			// Overage >= 100% of limit.
			return 200_000_000, nil
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

	// First check: 100% alert fires.
	bm.check(context.Background())
	firstCount := len(deliveries)
	if firstCount == 0 {
		t.Fatal("expected at least one delivery for 100% spending limit")
	}

	// Second check: should be deduped.
	bm.check(context.Background())
	if len(deliveries) != firstCount {
		t.Fatalf("expected dedup on second check, got %d deliveries", len(deliveries))
	}
}
