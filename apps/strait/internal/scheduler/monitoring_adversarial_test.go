package scheduler

import (
	"context"
	"math"
	"testing"
	"time"

	"strait/internal/billing"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

// Anomaly Monitor Tests.

// TestAnomalyMonitor_ZeroInterval verifies that a zero interval gets clamped to the default.
func TestAnomalyMonitor_ZeroInterval(t *testing.T) {
	t.Parallel()
	s := &mockAnomalyMonitorStore{}
	am := NewAnomalyMonitor(s, 0)
	require.Equal(t, 15*
		time.
			Minute, am.
		interval,
	)

}

// TestAnomalyMonitor_NegativeInterval verifies that a negative interval gets clamped to the default.
func TestAnomalyMonitor_NegativeInterval(t *testing.T) {
	t.Parallel()
	s := &mockAnomalyMonitorStore{}
	am := NewAnomalyMonitor(s, -5*time.Second)
	require.Equal(t, 15*
		time.
			Minute, am.
		interval,
	)

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
	e := &mockEnqueuer{}
	bm := NewBudgetMonitor(struct{}{}, e, 0)
	require.Equal(t, 5*
		time.Minute,
		bm.interval,
	)

}

// TestBudgetMonitor_MaxIntBudget verifies construction with math.MaxInt64 interval works.
func TestBudgetMonitor_MaxIntBudget(t *testing.T) {
	t.Parallel()
	e := &mockEnqueuer{}
	bm := NewBudgetMonitor(struct{}{}, e, time.Duration(math.MaxInt64))
	require.Equal(t, time.
		Duration(math.MaxInt64),

		bm.interval)

}

// Stats Aggregator Tests.

// TestStatsAggregator_NilStore verifies that NewStatsAggregator does not panic with a nil store.
func TestStatsAggregator_NilStore(t *testing.T) {
	t.Parallel()
	// Passing nil should not panic during construction.
	sa := NewStatsAggregator(nil)
	require.NotNil(t, sa)

}

// Grace Period Enforcer Tests.

// TestGracePeriod_ZeroDuration verifies construction with zero interval.
func TestGracePeriod_ZeroDuration(t *testing.T) {
	t.Parallel()
	s := &mockGraceEnforcerStore{}
	gpe := NewGracePeriodEnforcer(s, nil, 0)
	require.EqualValues(t, 0,
		gpe.interval,
	)

}

// TestGracePeriod_NegativeDuration verifies construction with negative interval.
func TestGracePeriod_NegativeDuration(t *testing.T) {
	t.Parallel()
	s := &mockGraceEnforcerStore{}
	gpe := NewGracePeriodEnforcer(s, nil, -10*time.Second)
	require.Equal(t, -10*
		time.
			Second, gpe.
		interval,
	)

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
	require.Len(t, s.updatedStatuses,

		0)

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
		require.False(t, am.
			interval <=
			0)

		e := &mockEnqueuer{}
		bm := NewBudgetMonitor(struct{}{}, e, time.Duration(intervalNs))
		require.False(t, bm.
			interval <=
			0)

	})
}
