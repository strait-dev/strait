package scheduler

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
)

// Section separator.
// Adversarial tests: try to break scheduler components through edge cases.
// Section separator.

func TestAdv_DowngradeApplier_ConcurrentApply(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)

	var acquireCount atomic.Int32
	var releaseCount atomic.Int32
	var applyCount atomic.Int32

	// Use a thread-safe mock: only one goroutine acquires the lock at a time.
	var lockMu sync.Mutex
	locked := false
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			acquireCount.Add(1)
			lockMu.Lock()
			defer lockMu.Unlock()
			if locked {
				return false, nil
			}
			locked = true
			return true, nil
		},
		releaseFn: func(_ context.Context, _ int64) error {
			releaseCount.Add(1)
			lockMu.Lock()
			defer lockMu.Unlock()
			locked = false
			return nil
		},
	}

	// Use a thread-safe store wrapper.
	s := &billingAdvMockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-concurrent", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
		applyFn: func(_ context.Context, _ string) error {
			applyCount.Add(1)
			return nil
		},
	}

	applier := NewDowngradeApplier(s, nil, time.Minute).WithAdvisoryLocker(locker)

	var wg conc.WaitGroup
	for range 10 {
		wg.Go(func() {
			applier.apply(context.Background())
		})
	}
	wg.Wait()

	// Advisory lock acquire should be called for each goroutine.
	if acquireCount.Load() != 10 {
		t.Errorf("expected 10 acquire calls, got %d", acquireCount.Load())
	}
	// At least some goroutines should have applied.
	if applyCount.Load() < 1 {
		t.Error("expected at least 1 apply call")
	}
}

func TestAdv_DowngradeApplier_ApplyErrorContinues(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	s := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-fail-first", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
			{OrgID: "org-succeed", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
		applyErrors: map[string]error{
			"org-fail-first": errors.New("db error"),
		},
	}

	applier := NewDowngradeApplier(s, nil, time.Minute)
	applier.apply(context.Background())

	// First org fails, second should still be processed.
	found := false
	for _, id := range s.appliedOrgIDs {
		if id == "org-succeed" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected org-succeed to be applied despite org-fail-first error")
	}
}

func TestAdv_GracePeriod_ExpiredLongAgo(t *testing.T) {
	t.Parallel()

	// Grace expired 30 days ago -- extreme edge case.
	longAgo := time.Now().Add(-30 * 24 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-ancient", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &longAgo},
		},
	}

	g := NewGracePeriodEnforcer(s, nil, time.Hour)
	g.enforce(context.Background())

	if s.updatedStatuses["org-ancient"] != "restricted" {
		t.Errorf("expected org-ancient to be restricted, got %q", s.updatedStatuses["org-ancient"])
	}
}

func TestAdv_GracePeriod_UpdateError_ContinuesOthers(t *testing.T) {
	t.Parallel()

	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-update-fail", PlanTier: "pro", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
			{OrgID: "org-update-ok", PlanTier: "starter", PaymentStatus: "grace", GracePeriodEnd: &pastGrace},
		},
		updateStatusErrs: map[string]error{
			"org-update-fail": errors.New("constraint violation"),
		},
	}

	g := NewGracePeriodEnforcer(s, nil, time.Hour)
	g.enforce(context.Background())

	// org-update-fail should have been skipped, org-update-ok should succeed.
	if s.updatedStatuses["org-update-ok"] != "restricted" {
		t.Errorf("expected org-update-ok to be restricted, got %q", s.updatedStatuses["org-update-ok"])
	}
}

func TestAdv_Reaper_ConcurrentReaping(t *testing.T) {
	t.Parallel()

	var staleCallCount atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			staleCallCount.Add(1)
			return nil, nil
		},
	}

	r := NewReaper(ms, time.Second, 5*time.Minute, 30*24*time.Hour, 90*24*time.Hour, false, nil)

	var wg conc.WaitGroup
	for range 5 {
		wg.Go(func() {
			r.ReapOnce(context.Background())
		})
	}
	wg.Wait()

	// All goroutines ran ReapOnce without panic or data corruption.
	if staleCallCount.Load() < 5 {
		t.Errorf("expected at least 5 stale run checks (one per goroutine), got %d", staleCallCount.Load())
	}
}

func TestAdv_CronScheduler_MalformedCronExpression(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := &mockCronStore{
		listCronJobsFn: func(_ context.Context) ([]domain.Job, error) {
			return []domain.Job{
				{ID: "j-bad-cron", ProjectID: "proj-1", Cron: "not a valid cron * * *"},
			}, nil
		},
		listCronWorkflowsFn: func(_ context.Context) ([]domain.Workflow, error) {
			return nil, nil
		},
	}

	cs := NewCronScheduler(ctx, s, &mockQueue{}, nil)
	err := cs.LoadJobs(ctx)
	// LoadJobs should return an error for malformed cron, not panic.
	if err == nil {
		t.Fatal("expected error for malformed cron expression, got nil")
	}
}

func TestAdv_AnomalyMonitor_AllZeroSpend(t *testing.T) {
	t.Parallel()

	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-zero"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return &billing.OrgSubscription{OrgID: "org-zero", PlanTier: "pro"}, nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, _ string, _, _ time.Time) ([]billing.UsageRecord, error) {
			// 7 days of zero spending.
			records := make([]billing.UsageRecord, 7)
			for i := range records {
				records[i] = billing.UsageRecord{
					OrgID:            "org-zero",
					ComputeCostMicro: 0,
				}
			}
			return records, nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	// Should not panic with division by zero when all spend is zero.
	am.check(context.Background())
}

func TestAdv_AnomalyMonitor_NegativeSpend(t *testing.T) {
	t.Parallel()

	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-neg"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return &billing.OrgSubscription{OrgID: "org-neg", PlanTier: "pro"}, nil
		},
		getOrgUsageForPeriodFn: func(_ context.Context, _ string, _, _ time.Time) ([]billing.UsageRecord, error) {
			return []billing.UsageRecord{
				{OrgID: "org-neg", ComputeCostMicro: -500_000},
			}, nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute)
	// Should handle negative spend gracefully -- no panic.
	am.check(context.Background())
}

func TestAdv_SLOEvaluator_ZeroTargetSLO(t *testing.T) {
	t.Parallel()

	// SLO target = 0%, should not cause division by zero.
	got := CalculateErrorBudget(0.95, 0.0, domain.SLOMetricSuccessRate)
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Errorf("expected finite result for zero target SLO, got %v", got)
	}
}

func TestAdv_SLOEvaluator_100PercentSLO(t *testing.T) {
	t.Parallel()

	// SLO target = 100%: any failure triggers budget depletion.
	got := CalculateErrorBudget(0.999, 1.0, domain.SLOMetricSuccessRate)
	if got != 0.0 {
		t.Errorf("expected 0.0 for 99.9%% actual vs 100%% target, got %v", got)
	}

	// Perfect 100% actual should preserve full budget.
	got = CalculateErrorBudget(1.0, 1.0, domain.SLOMetricSuccessRate)
	if got != 1.0 {
		t.Errorf("expected 1.0 for perfect match at 100%%, got %v", got)
	}
}

func TestAdv_UsageFlusher_ConcurrentFlush(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	var mu sync.Mutex
	var upsertCount int

	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-concurrent"}, nil
		},
		getOrgDailyUsageFn: func(_ context.Context, _ string, _ time.Time) ([]billing.UsageRecord, error) {
			return []billing.UsageRecord{
				{OrgID: "org-concurrent", ProjectID: "proj-1", PeriodDate: today, RunsCount: 1},
			}, nil
		},
		replaceUsageRecordFn: func(_ context.Context, _ *billing.UsageRecord) error {
			mu.Lock()
			upsertCount++
			mu.Unlock()
			return nil
		},
	}

	uf := NewUsageFlusher(s, time.Minute)

	var wg conc.WaitGroup
	for range 10 {
		wg.Go(func() {
			uf.flush(context.Background())
		})
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	// 10 goroutines * 1 record each = 10 upserts.
	if upsertCount != 10 {
		t.Fatalf("expected 10 upserts from concurrent flush, got %d", upsertCount)
	}
}

func TestAdv_UsageReportEmailer_VeryLargeUsageValues(t *testing.T) {
	t.Parallel()

	// Test that buildUsageReportHTML handles near-MaxInt64 values without panic.
	html := buildUsageReportHTML(
		"org-large",
		"enterprise",
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		math.MaxInt64,
		999,
		math.MaxInt64,
	)

	if html == "" {
		t.Fatal("expected non-empty HTML output")
	}
}

// billingAdvMockDowngradeStore is a thread-safe mock for concurrent downgrade tests.
type billingAdvMockDowngradeStore struct {
	mu          sync.Mutex
	pendingOrgs []billing.OrgSubscription
	applyFn     func(ctx context.Context, orgID string) error
}

func (m *billingAdvMockDowngradeStore) ListOrgsWithPendingDowngrade(_ context.Context) ([]billing.OrgSubscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pendingOrgs, nil
}

func (m *billingAdvMockDowngradeStore) ApplyPendingDowngrade(ctx context.Context, orgID string) error {
	if m.applyFn != nil {
		return m.applyFn(ctx, orgID)
	}
	return nil
}

func (m *billingAdvMockDowngradeStore) ApplyPendingDowngradeTierIfPending(ctx context.Context, orgID, _ string) (bool, error) {
	if err := m.ApplyPendingDowngrade(ctx, orgID); err != nil {
		return false, err
	}
	return true, nil
}

func (m *billingAdvMockDowngradeStore) ClearPendingPlanTierIfTier(context.Context, string, string) (bool, error) {
	return true, nil
}

func (m *billingAdvMockDowngradeStore) SuspendExcessProjects(_ context.Context, _ string, _ int) (int, error) {
	return 0, nil
}

func (m *billingAdvMockDowngradeStore) DeactivateExcessCronJobs(_ context.Context, _ string, _ int) (int64, error) {
	return 0, nil
}

func (m *billingAdvMockDowngradeStore) DeactivateExcessWebhookSubscriptions(_ context.Context, _ string, _ int) (int64, error) {
	return 0, nil
}

func (m *billingAdvMockDowngradeStore) DeactivateExcessEnvironments(_ context.Context, _ string, _ int) (int64, error) {
	return 0, nil
}

func (m *billingAdvMockDowngradeStore) ListProjectsByOrg(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (m *billingAdvMockDowngradeStore) PauseHTTPJobsByOrg(_ context.Context, _ string, _ string) (int64, error) {
	return 0, nil
}

func TestAdv_StaleSubChecker_MassiveOrgCount(t *testing.T) {
	t.Parallel()

	pastEnd := time.Now().Add(-48 * time.Hour)
	s := &mockStaleSubStore{
		listFn: func(context.Context) ([]billing.OrgSubscription, error) {
			subs := make([]billing.OrgSubscription, 1000)
			for i := range subs {
				subID := fmt.Sprintf("sub-%d", i)
				subs[i] = billing.OrgSubscription{
					OrgID:                fmt.Sprintf("org-%d", i),
					PlanTier:             "pro",
					StripeSubscriptionID: &subID,
					CurrentPeriodEnd:     &pastEnd,
				}
			}
			return subs, nil
		},
	}

	checker := NewStaleSubscriptionChecker(s, time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Should process all 1000 orgs without panic or timeout.
	checker.check(ctx)
}
