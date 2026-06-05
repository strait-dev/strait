package scheduler

import (
	"context"
	"errors"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
)

// Section separator.
// Anomaly Monitor extended tests.
// Uses mockAnomalyMonitorStore and mockCooldown from anomaly_monitor_test.go.
// Section separator.

func TestAnomalyMonitor_WithAdvisoryLock_NotAcquired(t *testing.T) {
	t.Parallel()

	var checkCalled atomic.Bool
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			checkCalled.Store(true)
			return nil, nil
		},
	}

	locker := &monitorMockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute).WithAdvisoryLocker(locker)
	am.check(context.Background())

	if checkCalled.Load() {
		t.Fatal("expected check to be skipped when lock not acquired")
	}
}

func TestAnomalyMonitor_WithAdvisoryLock_AcquireError(t *testing.T) {
	t.Parallel()

	var checkCalled atomic.Bool
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			checkCalled.Store(true)
			return nil, nil
		},
	}

	locker := &monitorMockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, errors.New("lock error")
		},
	}

	am := NewAnomalyMonitor(s, time.Minute).WithAdvisoryLocker(locker)
	am.check(context.Background())

	if checkCalled.Load() {
		t.Fatal("expected check to be skipped on lock error")
	}
}

func TestAnomalyMonitor_WithAdvisoryLock_Acquired_ReleasedAfter(t *testing.T) {
	t.Parallel()

	var lockReleased atomic.Bool
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return nil, nil
		},
	}

	locker := &monitorMockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return true, nil
		},
		releaseFn: func(_ context.Context, _ int64) error {
			lockReleased.Store(true)
			return nil
		},
	}

	am := NewAnomalyMonitor(s, time.Minute).WithAdvisoryLocker(locker)
	am.check(context.Background())

	if !lockReleased.Load() {
		t.Fatal("expected advisory lock to be released")
	}
}

func TestAnomalyMonitor_DefaultInterval(t *testing.T) {
	t.Parallel()

	am := NewAnomalyMonitor(&mockAnomalyMonitorStore{}, 0)
	if am.interval != 15*time.Minute {
		t.Fatalf("expected default interval 15m, got %v", am.interval)
	}
}

func TestAnomalyMonitor_NegativeInterval_DefaultsTo15Min(t *testing.T) {
	t.Parallel()

	am := NewAnomalyMonitor(&mockAnomalyMonitorStore{}, -1*time.Minute)
	if am.interval != 15*time.Minute {
		t.Fatalf("expected default interval 15m, got %v", am.interval)
	}
}

func TestAnomalyMonitor_CooldownCheckError_SkipsOrg(t *testing.T) {
	t.Parallel()

	var alertFired atomic.Bool
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-cd-err"}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			alertFired.Store(true)
			return nil
		},
	}

	cd := &extMockCooldown{
		inCooldownFn: func(_ context.Context, _ string) (bool, error) {
			return false, errors.New("redis error")
		},
	}

	am := NewAnomalyMonitor(s, time.Minute).WithCooldown(cd)
	am.check(context.Background())

	if alertFired.Load() {
		t.Fatal("expected no alert when cooldown check fails")
	}
}

func TestAnomalyMonitor_SetCooldownError_ContinuesWithoutPanic(t *testing.T) {
	t.Parallel()

	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-cd-set-err"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return &billing.OrgSubscription{OrgID: "org-cd-set-err", PlanTier: "pro"}, nil
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
	}

	cd := &extMockCooldown{
		inCooldownFn: func(_ context.Context, _ string) (bool, error) {
			return false, nil
		},
		setCooldownFn: func(_ context.Context, _ string) error {
			return errors.New("redis error")
		},
	}

	am := NewAnomalyMonitor(s, time.Minute).WithCooldown(cd)
	am.check(context.Background()) // should not panic
}

func TestAnomalyMonitor_NilCooldown_NoSkip(t *testing.T) {
	t.Parallel()

	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-nil-cd"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return &billing.OrgSubscription{OrgID: "org-nil-cd", PlanTier: "pro"}, nil
		},
	}
	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background()) // should not panic
}

// Section separator.
// SLO Evaluator extended tests (pure function tests)
// Section separator.

func TestCalculateErrorBudget_NaNInput_ReturnsZero(t *testing.T) {
	t.Parallel()

	nan := math.NaN()
	got := CalculateErrorBudget(nan, 0.99, domain.SLOMetricSuccessRate)
	if got != 0.0 {
		t.Errorf("expected 0.0 for NaN current, got %v", got)
	}

	got = CalculateErrorBudget(0.95, nan, domain.SLOMetricSuccessRate)
	if got != 0.0 {
		t.Errorf("expected 0.0 for NaN target, got %v", got)
	}
}

func TestCalculateErrorBudget_InfInput_ReturnsZero(t *testing.T) {
	t.Parallel()

	inf := math.Inf(1)
	got := CalculateErrorBudget(inf, 0.99, domain.SLOMetricSuccessRate)
	if got != 0.0 {
		t.Errorf("expected 0.0 for Inf current, got %v", got)
	}
}

func TestCalculateErrorBudget_P95Latency_HalfTarget(t *testing.T) {
	t.Parallel()

	got := CalculateErrorBudget(0.5, 1.0, domain.SLOMetricP95LatencySecs)
	if got < 0.49 || got > 0.51 {
		t.Errorf("expected ~0.5, got %v", got)
	}
}

func TestCalculateErrorBudget_P99Latency_OverTarget(t *testing.T) {
	t.Parallel()

	got := CalculateErrorBudget(3.0, 2.0, domain.SLOMetricP99LatencySecs)
	if got != 0.0 {
		t.Errorf("expected 0.0 for over-target latency, got %v", got)
	}
}

func TestCalculateErrorBudget_SuccessRate_995vs99(t *testing.T) {
	t.Parallel()

	got := CalculateErrorBudget(0.995, 0.99, domain.SLOMetricSuccessRate)
	if got < 0.49 || got > 0.51 {
		t.Errorf("expected ~0.5, got %v", got)
	}
}

func TestCalculateErrorBudget_SuccessRate_BothZero(t *testing.T) {
	t.Parallel()

	got := CalculateErrorBudget(0.0, 0.0, domain.SLOMetricSuccessRate)
	if got != 0.0 {
		t.Errorf("expected 0.0, got %v", got)
	}
}

func TestCalculateErrorBudget_Latency_ZeroCurrent(t *testing.T) {
	t.Parallel()

	got := CalculateErrorBudget(0.0, 1.0, domain.SLOMetricP95LatencySecs)
	if got != 1.0 {
		t.Errorf("expected 1.0, got %v", got)
	}
}

func TestCalculateErrorBudget_Latency_NegativeTarget(t *testing.T) {
	t.Parallel()

	got := CalculateErrorBudget(0.5, -1.0, domain.SLOMetricP95LatencySecs)
	if got != 1.0 {
		t.Errorf("expected 1.0 for negative target, got %v", got)
	}
}

// Section separator.
// StatsAggregator extended tests.
// Section separator.

func TestStatsAggregator_CostAggregation_Called(t *testing.T) {
	t.Parallel()

	var costCalled atomic.Bool
	s := &monitorMockStatsStore{
		aggregateFn: func(_ context.Context, _ time.Time) error {
			return nil
		},
		costFn: func(_ context.Context, _ time.Time) error {
			costCalled.Store(true)
			return nil
		},
	}

	a := NewStatsAggregator(s)
	if a.store == nil {
		t.Fatal("expected store to be set")
	}

	if err := a.store.AggregateCostStatsHourly(context.Background(), time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !costCalled.Load() {
		t.Fatal("expected AggregateCostStatsHourly to be called")
	}
}

func TestStatsAggregator_AggregateError_NoPanic(t *testing.T) {
	t.Parallel()

	s := &monitorMockStatsStore{
		aggregateFn: func(_ context.Context, _ time.Time) error {
			return errors.New("aggregate failed")
		},
	}

	a := NewStatsAggregator(s)
	previousHour := time.Now().Add(-time.Hour).Truncate(time.Hour)
	err := a.store.AggregateHourlyStats(context.Background(), previousHour)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStatsAggregator_WithAdvisoryLocker_LockError(t *testing.T) {
	t.Parallel()

	s := &monitorMockStatsStore{
		aggregateFn: func(_ context.Context, _ time.Time) error {
			return nil
		},
	}

	locker := &monitorMockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, errors.New("lock error")
		},
	}

	a := NewStatsAggregator(s).WithAdvisoryLocker(locker)
	if a.advisoryLocker == nil {
		t.Fatal("expected advisory locker to be set")
	}
}

func TestStatsAggregator_HourTruncation(t *testing.T) {
	t.Parallel()

	previousHour := time.Now().Add(-time.Hour).Truncate(time.Hour)
	if previousHour.Minute() != 0 || previousHour.Second() != 0 {
		t.Fatalf("expected hour to be truncated, got %v", previousHour)
	}
}

// Section separator.
// ConcurrentReconciler extended tests.
// Section separator.

func TestConcurrentReconciler_Run_StopsOnContextCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	enforcer := newTestEnforcer(t)
	counter := &extMockExecutingRunCounter{}
	r := NewConcurrentReconciler(enforcer, counter, 10*time.Millisecond)

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

func TestConcurrentReconciler_ReconcileError_ContinuesLoop(t *testing.T) {
	t.Parallel()

	enforcer := newTestEnforcer(t)
	counter := &extMockExecutingRunCounter{
		countErr: errors.New("count error"),
	}
	r := NewConcurrentReconciler(enforcer, counter, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	r.Run(ctx) // should not panic
}

// Section separator.
// Local mock types (prefixed to avoid conflicts with other test files).
// Section separator.

// extMockCooldown provides callback-based cooldown mock (vs the map-based mockCooldown in anomaly_monitor_test.go).
type extMockCooldown struct {
	inCooldownFn  func(ctx context.Context, orgID string) (bool, error)
	setCooldownFn func(ctx context.Context, orgID string) error
}

func (m *extMockCooldown) InCooldown(ctx context.Context, orgID string) (bool, error) {
	if m.inCooldownFn != nil {
		return m.inCooldownFn(ctx, orgID)
	}
	return false, nil
}

func (m *extMockCooldown) SetCooldown(ctx context.Context, orgID string) error {
	if m.setCooldownFn != nil {
		return m.setCooldownFn(ctx, orgID)
	}
	return nil
}

type monitorMockAdvisoryLocker struct {
	acquireFn func(ctx context.Context, lockID int64) (bool, error)
	releaseFn func(ctx context.Context, lockID int64) error
}

func (m *monitorMockAdvisoryLocker) TryAdvisoryLock(ctx context.Context, lockID int64) (bool, error) {
	if m.acquireFn != nil {
		return m.acquireFn(ctx, lockID)
	}
	return true, nil
}

func (m *monitorMockAdvisoryLocker) ReleaseAdvisoryLock(ctx context.Context, lockID int64) error {
	if m.releaseFn != nil {
		return m.releaseFn(ctx, lockID)
	}
	return nil
}

type monitorMockStatsStore struct {
	aggregateFn func(ctx context.Context, hour time.Time) error
	costFn      func(ctx context.Context, hour time.Time) error
}

func (m *monitorMockStatsStore) AggregateHourlyStats(ctx context.Context, hour time.Time) error {
	if m.aggregateFn != nil {
		return m.aggregateFn(ctx, hour)
	}
	return nil
}

func (m *monitorMockStatsStore) AggregateCostStatsHourly(ctx context.Context, hour time.Time) error {
	if m.costFn != nil {
		return m.costFn(ctx, hour)
	}
	return nil
}

type extMockExecutingRunCounter struct {
	countErr error
}

func (m *extMockExecutingRunCounter) CountExecutingRunsByOrg(_ context.Context, _ string) (int, error) {
	return 0, m.countErr
}

func (m *extMockExecutingRunCounter) BulkCountExecutingRunsByOrg(_ context.Context, orgIDs []string) (map[string]int, error) {
	if m.countErr != nil {
		return nil, m.countErr
	}
	return make(map[string]int, len(orgIDs)), nil
}

func (m *extMockExecutingRunCounter) ListOrgsWithExecutingRuns(_ context.Context) ([]string, error) {
	return nil, m.countErr
}
