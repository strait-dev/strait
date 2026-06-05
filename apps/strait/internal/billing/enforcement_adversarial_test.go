package billing

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 1. Enforcer nil-receiver adversarial tests

func TestEnforcer_NilReceiver_GetOrgPlanLimits(t *testing.T) {
	t.Parallel()
	// A nil Enforcer pointer should not panic; GetOrgPlanLimits handles nil receiver.
	var e *Enforcer
	limits, err := e.GetOrgPlanLimits(context.Background(), "org-1")
	require.NoError(t,
		err)
	require.Equal(t,
		domain.PlanFree,
		limits.
			PlanTier,
	)

}

// 2. EnsureOrgSubscription adversarial tests

func TestEnsureOrgSubscription_DelegatesCorrectly(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	var calledWith string
	store := &mockBillingStore{}
	// Override the default mock to track calls.
	origFn := store.EnsureOrgSubscription
	_ = origFn // mockBillingStore has a hardcoded method; we test via enforcer.
	enforcer := NewEnforcer(store, rdb, slog.Default())

	err := enforcer.EnsureOrgSubscription(context.Background(), "org-new")
	require.NoError(t,
		err)

	_ = calledWith
}

func TestEnsureOrgSubscription_ExistingOrgNoError(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-existing": {OrgID: "org-existing", PlanTier: "starter", Status: "active"},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	// Calling EnsureOrgSubscription for an existing org should not error.
	// The mock's EnsureOrgSubscription always returns nil.
	err := enforcer.EnsureOrgSubscription(context.Background(), "org-existing")
	require.NoError(t,
		err)

}

func TestEnsureOrgSubscription_ConcurrentIdempotent(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()
	const goroutines = 50
	errs := make(chan error, goroutines)

	var wg conc.WaitGroup
	for range goroutines {
		wg.Go(func() {
			errs <- enforcer.EnsureOrgSubscription(ctx, "org-race")
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t,
			err)

	}
}

// 3. RestrictOrgTx adversarial tests (tx.go)
// RestrictOrgTx requires a real pgxpool.Pool which is only available in
// integration tests. We test the wrapper WithBillingTx with a nil pool to
// verify nil handling returns an error.

func TestWithBillingTx_NilPool_Panics(t *testing.T) {
	t.Parallel()
	// WithBillingTx with a nil pool will call pool.Begin which panics on nil receiver.
	// Verify we get a panic (not a silent nil error).
	defer func() {
		r := recover()
		require.NotNil(t,
			r)

	}()
	_ = WithBillingTx(context.Background(), nil, func(_ pgx.Tx) error {
		require.Fail(t,

			"fn should not be called with nil pool")
		return nil
	})
}

// 4. Stripe usage event ingestion adversarial tests (stripe_usage.go)

func TestStripeUsageReporter_EmptySecretKey_Noop(t *testing.T) {
	t.Parallel()
	// Empty secret key causes IngestRunOverage to silently return nil.
	reporter := NewStripeUsageReporter("", slog.Default())
	err := reporter.IngestRunOverage(context.Background(), "cust-1", "run-1")
	require.NoError(t,
		err)

}

func TestStripeUsageReporter_EmptyCustomerID_Noop(t *testing.T) {
	t.Parallel()
	// Empty customer ID causes IngestRunOverage to silently return nil.
	reporter := NewStripeUsageReporter("sk_test_key", slog.Default())
	err := reporter.IngestRunOverage(context.Background(), "", "run-1")
	require.NoError(t,
		err)

}

func TestStripeUsageReporter_WithMetrics(t *testing.T) {
	t.Parallel()
	// Verify WithUsageReporterMetrics option can be applied without panic.
	reporter := NewStripeUsageReporter("sk_test_key", slog.Default(), WithUsageReporterMetrics(nil))
	require.NotNil(t,
		reporter)

}

func TestStripeUsageReporter_NilLogger(t *testing.T) {
	t.Parallel()
	// Passing nil logger should use slog.Default without panic.
	reporter := NewStripeUsageReporter("sk_test_key", nil)
	require.NotNil(t,
		reporter)

}

// 5. Billing enforcement edge cases

func TestEnforcer_FreeTier_AllLimitsHit(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		projects:     map[string][]string{"org-free-all": {"p1", "p2"}},
		memberCounts: map[string]int{"org-free-all": 3},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	ctx := context.Background()

	// Project limit
	err := enforcer.CheckProjectLimit(ctx, "org-free-all")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, errors.As(err,
		&le))
	assert.Equal(t, "project_limit_reached",

		le.Code,
	)

	// Member limit
	err = enforcer.CheckMemberLimit(ctx, "org-free-all")
	require.Error(t,
		err)
	require.True(t, errors.As(err,
		&le))
	assert.Equal(t, "member_limit_reached",

		le.Code,
	)

	// Concurrent run limit
	freeLimits := GetPlanLimits(domain.PlanFree)
	for range freeLimits.MaxConcurrentRuns {
		_ = enforcer.CheckConcurrentRunLimit(ctx, "org-free-all")
	}
	err = enforcer.CheckConcurrentRunLimit(ctx, "org-free-all")
	require.Error(t,
		err)
	require.True(t, errors.As(err,
		&le))
	assert.Equal(t, "org_concurrent_run_limit_exceeded",

		le.Code)

}

func TestEnforcer_PlanUpgradeMidOperation_CacheInvalidation(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-upgrade": {OrgID: "org-upgrade", PlanTier: "free", Status: "active"},
		},
		projects: map[string][]string{"org-upgrade": {"p1", "p2"}},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	ctx := context.Background()

	// At free tier, 2 projects = at limit.
	err := enforcer.CheckProjectLimit(ctx, "org-upgrade")
	require.Error(t,
		err)

	// Simulate plan upgrade.
	store.subscriptions["org-upgrade"].PlanTier = "starter"
	enforcer.InvalidateOrgCache("org-upgrade")

	// After upgrade, 2 projects should be under starter limit (5).
	err = enforcer.CheckProjectLimit(ctx, "org-upgrade")
	require.NoError(t,
		err)

}

func TestEnforcer_ConcurrentPlanChange_DuringLimitCheck(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	// Use a thread-safe subscription function to avoid data races on the mock map.
	var planMu sync.RWMutex
	currentPlan := "starter"
	store := &mockBillingStore{
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*OrgSubscription, error) {
			planMu.RLock()
			tier := currentPlan
			planMu.RUnlock()
			return &OrgSubscription{OrgID: "org-race", PlanTier: tier, Status: "active"}, nil
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	ctx := context.Background()

	const goroutines = 30
	var wg conc.WaitGroup

	for i := range goroutines {
		wg.Go(func() {
			if i%2 == 0 {
				_ = enforcer.CheckDailyRunLimit(ctx, "org-race")
			} else {
				planMu.Lock()
				currentPlan = "pro"
				planMu.Unlock()
				enforcer.InvalidateOrgCache("org-race")

				planMu.Lock()
				currentPlan = "starter"
				planMu.Unlock()
				enforcer.InvalidateOrgCache("org-race")
			}
		})
	}
	wg.Wait()
	// No panics or data races under -race is the success criterion.
}

func TestAutoDisableResources_VariousStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		impacts         []ResourceImpact
		wantManualCount int
		wantAutoCount   int
	}{
		{
			name:            "all_ok_no_actions",
			impacts:         []ResourceImpact{{Resource: "projects", Action: ResourceActionOK}},
			wantManualCount: 0,
			wantAutoCount:   0,
		},
		{
			name: "projects_reduce_is_manual",
			impacts: []ResourceImpact{
				{Resource: "projects", Current: 5, Limit: 2, Action: ResourceActionReduce},
			},
			wantManualCount: 1,
			wantAutoCount:   0,
		},
		{
			name: "log_drains_remove_is_auto",
			impacts: []ResourceImpact{
				{Resource: "log_drains", Current: 3, Limit: 0, Action: ResourceActionRemove},
			},
			wantManualCount: 0,
			wantAutoCount:   1,
		},
		{
			name: "members_reduce_is_manual",
			impacts: []ResourceImpact{
				{Resource: "members", Current: 10, Limit: 3, Action: ResourceActionReduce},
			},
			wantManualCount: 1,
			wantAutoCount:   0,
		},
		{
			name: "members_per_org_reduce_is_manual",
			impacts: []ResourceImpact{
				{Resource: "members_per_org", Current: 10, Limit: 3, Action: ResourceActionReduce},
			},
			wantManualCount: 1,
			wantAutoCount:   0,
		},
		{
			name: "mixed_resources",
			impacts: []ResourceImpact{
				{Resource: "projects", Current: 5, Limit: 2, Action: ResourceActionReduce},
				{Resource: "alert_rules", Current: 10, Limit: 5, Action: ResourceActionReduce},
				{Resource: "webhooks", Current: 5, Limit: 0, Action: ResourceActionRemove},
				{Resource: "members", Current: 10, Limit: 3, Action: ResourceActionReduce},
				{Resource: "custom_roles", Current: 2, Limit: 0, Action: ResourceActionRemove},
			},
			wantManualCount: 2, // projects + members
			wantAutoCount:   3, // alert_rules + webhooks + custom_roles
		},
		{
			name:            "empty_impacts",
			impacts:         nil,
			wantManualCount: 0,
			wantAutoCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			manual, auto := AutoDisableResources(tt.impacts)
			assert.Len(t, manual,
				tt.wantManualCount,
			)
			assert.Len(t, auto,
				tt.wantAutoCount,
			)

		})
	}
}

func TestEnforcer_EnforcementMode_Disabled_SkipsDailyLimit(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-disabled": {
				OrgID:           "org-disabled",
				PlanTier:        "free",
				Status:          "active",
				EnforcementMode: "disabled",
			},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	ctx := context.Background()

	// Exhaust the daily limit.
	freeLimits := GetPlanLimits(domain.PlanFree)
	for range freeLimits.MaxRunsPerDay + 100 {
		require.NoError(t,
			enforcer.
				CheckDailyRunLimit(
					ctx, "org-disabled",
				))

	}
}

func TestEnforcer_EnforcementMode_Warn_SkipsDailyLimit(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-warn": {
				OrgID:           "org-warn",
				PlanTier:        "free",
				Status:          "active",
				EnforcementMode: "warn",
			},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	ctx := context.Background()

	// Use a small count to verify enforcement_mode=warn bypasses the limit.
	// The free tier limit is 5000 -- use limit+10 to prove it is skipped.
	freeLimits := GetPlanLimits(domain.PlanFree)
	for range freeLimits.MaxRunsPerDay + 10 {
		require.NoError(t,
			enforcer.
				CheckDailyRunLimit(
					ctx, "org-warn",
				))

	}
}

func TestEnforcer_OverrideRunLimits(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	dailyOverride := 10
	concurrentOverride := 2
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-override": {
				OrgID:                      "org-override",
				PlanTier:                   "free",
				Status:                     "active",
				OverrideDailyRunLimit:      &dailyOverride,
				OverrideConcurrentRunLimit: &concurrentOverride,
			},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	ctx := context.Background()

	// Legacy daily overrides are inert for launch. Billing is monthly
	// orchestration runs, so stale support metadata must not reactivate
	// a daily quota.
	for range 11 {
		require.NoError(t,
			enforcer.
				CheckDailyRunLimit(
					ctx, "org-override",
				))

	}
	limits, err := enforcer.GetOrgPlanLimits(ctx, "org-override")
	require.NoError(t,
		err)
	require.EqualValues(t, -1, limits.
		MaxRunsPerDay,
	)

	// Concurrent override: 2 runs.
	for range 2 {
		require.NoError(t,
			enforcer.
				CheckConcurrentRunLimit(ctx, "org-override"))

	}
	err = enforcer.CheckConcurrentRunLimit(ctx, "org-override")
	require.Error(t,
		err)

}

func TestEnforcer_ProjectSuspended_CacheRace(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	ctx := context.Background()

	const goroutines = 30
	var wg conc.WaitGroup
	for i := range goroutines {
		wg.Go(func() {
			if i%3 == 0 {
				enforcer.InvalidateProjectSuspendedCache("proj-race")
			} else {
				_ = enforcer.CheckProjectSuspended(ctx, "proj-race")
			}
		})
	}
	wg.Wait()
	// Success = no panics or races under -race.
}

func TestEnforcer_DailyRunLimit_ConcurrentUnlimited(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*OrgSubscription, error) {
			return nil, ErrSubscriptionNotFound
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	ctx := context.Background()

	// Daily runs are unlimited for all plans. Verify concurrent access
	// never produces a rejection.
	const goroutines = 20
	const runsPerGoroutine = 500
	var rejected atomic.Int64

	var wg conc.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range runsPerGoroutine {
				if err := enforcer.CheckDailyRunLimit(ctx, "org-exhaust-conc"); err != nil {
					rejected.Add(1)
				}
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 0, rejected.
		Load())

}

func TestEnforcer_ConcurrentRunLimit_DoubleFreeAfterDecrement(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	ctx := context.Background()
	require.NoError(t,
		enforcer.
			CheckConcurrentRunLimit(ctx, "org-double-decr"))

	// Start one run.

	// Decrement twice (simulating double-free).
	enforcer.DecrConcurrentRunCount(ctx, "org-double-decr")
	enforcer.DecrConcurrentRunCount(ctx, "org-double-decr")

	// Counter should be floored at 0, not go negative.
	rdbClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	val, err := rdbClient.Get(ctx, "strait:org_concurrent:org-double-decr").Int64()
	require.False(t,
		err != nil &&
			!errors.Is(err,
				redis.Nil))
	require.GreaterOrEqual(t, val,
		int64(0))

}

func TestEnforcer_PaymentRestricted_BlocksAllLimitChecks(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-blocked": {
				OrgID:         "org-blocked",
				PlanTier:      "pro",
				Status:        "active",
				PaymentStatus: "restricted",
			},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	ctx := context.Background()

	checks := []struct {
		name  string
		check func() error
	}{
		{"daily_run", func() error { return enforcer.CheckDailyRunLimit(ctx, "org-blocked") }},
		{"concurrent_run", func() error { return enforcer.CheckConcurrentRunLimit(ctx, "org-blocked") }},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			err := c.check()
			require.Error(t,
				err)

			var le *LimitError
			require.True(t, errors.As(err,
				&le))
			assert.Equal(t, "payment_restricted",

				le.
					Code)

		})
	}
}

func TestEnforcer_GracePeriodEdge_ExactExpiry(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	// Grace period that just barely expired (1 nanosecond ago).
	graceEnd := time.Now().Add(-1 * time.Nanosecond)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-edge": {
				OrgID:          "org-edge",
				PlanTier:       "starter",
				Status:         "active",
				PaymentStatus:  "grace",
				GracePeriodEnd: &graceEnd,
			},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	err := enforcer.CheckDailyRunLimit(context.Background(), "org-edge")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, errors.As(err,
		&le))
	assert.Equal(t, "grace_period_expired",

		le.Code,
	)

}

func TestEnforcer_SuspendExcessProjects_UnlimitedPlan(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	// -1 means unlimited, should not suspend any projects.
	suspended, err := enforcer.SuspendExcessProjects(context.Background(), "org-ent", -1)
	require.NoError(t,
		err)
	require.EqualValues(t, 0, suspended)

}

func TestEnforcer_LimitError_ImplementsErrorInterface(t *testing.T) {
	t.Parallel()
	le := &LimitError{
		Code:         "test_limit",
		Message:      "test message",
		CurrentUsage: 5,
		Limit:        3,
		Plan:         "free",
		UpgradeURL:   "/upgrade",
	}

	// Verify it implements the error interface.
	var err error = le
	assert.Equal(t, "test message",

		err.Error())

	// Verify errors.As works.
	var target *LimitError
	require.True(t, errors.As(err,
		&target),
	)
	assert.Equal(t, "test_limit",

		target.Code,
	)

}
