package scheduler

import (
	"context"
	"math"
	"testing"
	"time"

	"strait/internal/billing"

	"github.com/sourcegraph/conc"
)

// Anomaly Monitor Tests.

// TestAnomalyMonitor_ZeroInterval verifies that a zero interval gets clamped to the default.
func TestAnomalyMonitor_ZeroInterval(t *testing.T) {
	t.Parallel()
	s := &mockAnomalyMonitorStore{}
	am := NewAnomalyMonitor(s, 0)
	if am.interval != 15*time.Minute {
		t.Fatalf("expected default interval 15m, got %v", am.interval)
	}
}

// TestAnomalyMonitor_NegativeInterval verifies that a negative interval gets clamped to the default.
func TestAnomalyMonitor_NegativeInterval(t *testing.T) {
	t.Parallel()
	s := &mockAnomalyMonitorStore{}
	am := NewAnomalyMonitor(s, -5*time.Second)
	if am.interval != 15*time.Minute {
		t.Fatalf("expected default interval 15m, got %v", am.interval)
	}
}

// TestAnomalyMonitor_NilCooldown verifies the monitor does not panic when cooldown is nil.
func TestAnomalyMonitor_NilCooldown(t *testing.T) {
	t.Parallel()
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
	}
	am := NewAnomalyMonitor(s, time.Minute)
	// cooldown is nil by default; check should not panic.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	am.Run(ctx)
}

// TestAnomalyMonitor_NilLocker verifies the monitor does not panic when advisory locker is nil.
func TestAnomalyMonitor_NilLocker(t *testing.T) {
	t.Parallel()
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			return nil, nil
		},
	}
	am := NewAnomalyMonitor(s, time.Minute)
	// advisoryLocker is nil by default; check should not panic.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	am.Run(ctx)
}

// Budget Monitor Tests.

// TestBudgetMonitor_ZeroBudget verifies construction with a zero budget interval clamps to default.
func TestBudgetMonitor_ZeroBudget(t *testing.T) {
	t.Parallel()
	s := &mockBudgetStore{}
	e := &mockEnqueuer{}
	bm := NewBudgetMonitor(s, e, 0)
	if bm.interval != 5*time.Minute {
		t.Fatalf("expected default interval 5m, got %v", bm.interval)
	}
}

// TestBudgetMonitor_MaxIntBudget verifies construction with math.MaxInt64 interval works.
func TestBudgetMonitor_MaxIntBudget(t *testing.T) {
	t.Parallel()
	s := &mockBudgetStore{}
	e := &mockEnqueuer{}
	bm := NewBudgetMonitor(s, e, time.Duration(math.MaxInt64))
	if bm.interval != time.Duration(math.MaxInt64) {
		t.Fatalf("expected max int interval, got %v", bm.interval)
	}
}

// Stats Aggregator Tests.

// TestStatsAggregator_NilStore verifies that NewStatsAggregator does not panic with a nil store.
func TestStatsAggregator_NilStore(t *testing.T) {
	t.Parallel()
	// Passing nil should not panic during construction.
	sa := NewStatsAggregator(nil)
	if sa == nil {
		t.Fatal("expected non-nil StatsAggregator")
	}
}

// Grace Period Enforcer Tests.

// TestGracePeriod_ZeroDuration verifies construction with zero interval.
func TestGracePeriod_ZeroDuration(t *testing.T) {
	t.Parallel()
	s := &mockGraceEnforcerStore{}
	gpe := NewGracePeriodEnforcer(s, nil, 0)
	if gpe.interval != 0 {
		t.Fatalf("expected interval 0, got %v", gpe.interval)
	}
}

// TestGracePeriod_NegativeDuration verifies construction with negative interval.
func TestGracePeriod_NegativeDuration(t *testing.T) {
	t.Parallel()
	s := &mockGraceEnforcerStore{}
	gpe := NewGracePeriodEnforcer(s, nil, -10*time.Second)
	if gpe.interval != -10*time.Second {
		t.Fatalf("expected interval -10s, got %v", gpe.interval)
	}
}

// TestGracePeriod_AlreadyRestricted verifies that orgs with "restricted" status are skipped.
func TestGracePeriod_AlreadyRestricted(t *testing.T) {
	t.Parallel()
	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-1", PaymentStatus: "restricted", GracePeriodEnd: &pastGrace},
		},
	}
	gpe := NewGracePeriodEnforcer(s, nil, time.Minute)
	// enforce should skip the restricted org without error.
	gpe.enforce(context.Background())
	if len(s.updatedStatuses) != 0 {
		t.Fatalf("expected no status updates for restricted org, got %d", len(s.updatedStatuses))
	}
}

// TestGracePeriod_ConcurrentEnforce verifies that concurrent enforce calls do not panic.
func TestGracePeriod_ConcurrentEnforce(t *testing.T) {
	t.Parallel()
	pastGrace := time.Now().Add(-1 * time.Hour)
	s := &mockGraceEnforcerStore{
		graceOrgs: []billing.OrgSubscription{
			{OrgID: "org-1", PaymentStatus: "grace", PlanTier: "pro", GracePeriodEnd: &pastGrace},
		},
	}
	gpe := NewGracePeriodEnforcer(s, nil, time.Minute)

	var wg conc.WaitGroup
	for range 20 {
		wg.Go(func() {
			gpe.enforce(context.Background())
		})
	}
	wg.Wait()
}

// FuzzMonitorIntervals fuzzes the interval parameter for anomaly and budget monitors.
func FuzzMonitorIntervals(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(-1))
	f.Add(int64(1))
	f.Add(int64(math.MaxInt64))
	f.Add(int64(math.MinInt64))
	f.Add(int64(1_000_000_000)) // 1 second in nanoseconds.

	f.Fuzz(func(t *testing.T, intervalNs int64) {
		s := &mockAnomalyMonitorStore{}
		am := NewAnomalyMonitor(s, time.Duration(intervalNs))
		if am.interval <= 0 {
			t.Fatalf("interval should be positive after clamping, got %v", am.interval)
		}

		bs := &mockBudgetStore{}
		e := &mockEnqueuer{}
		bm := NewBudgetMonitor(bs, e, time.Duration(intervalNs))
		if bm.interval <= 0 {
			t.Fatalf("budget interval should be positive after clamping, got %v", bm.interval)
		}
	})
}
