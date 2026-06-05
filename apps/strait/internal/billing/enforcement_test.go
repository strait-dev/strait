package billing

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupEnforcer(t *testing.T) (*Enforcer, *mockBillingStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	return enforcer, store, mr
}

func TestEnforcer_CheckDailyRunLimit_Free(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	// Free plan: unlimited daily runs (MaxRunsPerDay = -1).
	// Verify many runs succeed without any limit error.
	for range 10_000 {
		require.NoError(t,
			enforcer.
				CheckDailyRunLimit(context.
					Background(),
					"org_free",
				))

	}
}

func TestEnforcer_CheckDailyRunLimit_Starter(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org_starter": {OrgID: "org_starter", PlanTier: "starter", Status: "active"},
	}

	// Starter plan: unlimited daily runs.
	for range 10_000 {
		require.NoError(t,
			enforcer.
				CheckDailyRunLimit(context.
					Background(),
					"org_starter",
				))

	}
}

func TestEnforcer_CheckDailyRunLimit_Enterprise(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org_ent": {OrgID: "org_ent", PlanTier: "enterprise", Status: "active"},
	}

	// Enterprise: unlimited
	for range 1000 {
		require.NoError(t,
			enforcer.
				CheckDailyRunLimit(context.
					Background(),
					"org_ent",
				))

	}
}

// TestEnforcer_Integration_FreeTierDailyRunUnlimited verifies that the free tier
// has unlimited daily runs (MaxRunsPerDay = -1).
func TestEnforcer_Integration_FreeTierDailyRunUnlimited(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org_free_explicit": {
			OrgID:    "org_free_explicit",
			PlanTier: string(domain.PlanFree),
			Status:   "active",
		},
	}

	ctx := context.Background()

	// Daily runs are unlimited for all plans. Verify many runs succeed.
	for range 10_000 {
		require.NoError(t,
			enforcer.
				CheckDailyRunLimit(ctx,
					"org_free_explicit",
				))

	}
}

func TestEnforcer_CheckDailyRunLimit_EmptyOrgID(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)
	require.NoError(t,
		enforcer.
			CheckDailyRunLimit(context.
				Background(),
				"",
			))

}

func TestEnforcer_DecrRollback(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	ctx := context.Background()

	// With unlimited daily runs, decrement should not panic or error.
	for range 100 {
		_ = enforcer.CheckDailyRunLimit(ctx, "org_rollback")
	}

	// Decrement (simulating a failed run) should work cleanly.
	enforcer.DecrDailyRunCount(ctx, "org_rollback")
	require.NoError(t,
		enforcer.
			CheckDailyRunLimit(ctx,
				"org_rollback",
			))

	// Should still allow runs (unlimited).

}

func TestEnforcer_CheckConcurrentRunLimit(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	ctx := context.Background()
	// Free plan: ConcurrentFree concurrent runs max.
	for range ConcurrentFree {
		require.NoError(t,
			enforcer.
				CheckConcurrentRunLimit(ctx, "org_conc"))

	}

	// Next run should fail.
	err := enforcer.CheckConcurrentRunLimit(ctx, "org_conc")
	require.Error(t,
		err)

	// Decrement one, should allow another.
	enforcer.DecrConcurrentRunCount(ctx, "org_conc")
	require.NoError(t,
		enforcer.
			CheckConcurrentRunLimit(ctx, "org_conc"))

}

func TestEnforcer_CheckConcurrentRunLimit_ActivePaymentGraceStillEnforcesPlanCap(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	ctx := context.Background()
	graceEnd := time.Now().Add(time.Hour)
	store.subscriptions = map[string]*OrgSubscription{
		"org_grace": {
			OrgID:          "org_grace",
			PlanTier:       string(domain.PlanFree),
			Status:         "active",
			PaymentStatus:  "grace",
			GracePeriodEnd: &graceEnd,
		},
	}

	for range ConcurrentFree {
		require.NoError(t,
			enforcer.
				CheckConcurrentRunLimit(ctx, "org_grace"),
		)

	}
	err := enforcer.CheckConcurrentRunLimit(ctx, "org_grace")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, errors.As(err,
		&le))
	require.Equal(t,
		"org_concurrent_run_limit_exceeded",

		le.Code,
	)

}

func TestEnforcer_CheckConcurrentRunLimit_PaymentLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	enforcer := NewEnforcer(&mockBillingStore{
		getOrgSubscriptionFn: func(context.Context, string) (*OrgSubscription, error) {
			return nil, errors.New("subscription store unavailable")
		},
	}, rdb, slog.Default())

	err := enforcer.CheckConcurrentRunLimit(context.Background(), "org-plan-error")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"service_degraded",
		le.
			Code)

}

func TestEnforcer_CheckConcurrentRunLimit_AddonLoadErrorFailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	enforcer := NewEnforcer(&mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-addon-error": {OrgID: "org-addon-error", PlanTier: "pro", Status: "active"},
		},
		listActiveAddonsErr: errors.New("addon store unavailable"),
	}, rdb, slog.Default())

	err := enforcer.CheckConcurrentRunLimit(context.Background(), "org-addon-error")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"billing_plan_unavailable",

		le.Code,
	)

}

func TestEnforcer_CheckConcurrentRunLimit_RedisErrorFailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	require.NoError(t,
		rdb.Close())

	enforcer := NewEnforcer(&mockBillingStore{}, rdb, slog.Default())

	err := enforcer.CheckConcurrentRunLimit(context.Background(), "org-redis-error")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"service_degraded",
		le.
			Code)

}

func TestEnforcer_CheckConcurrentRunLimit_RequiredNilRedisFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{}, nil, slog.Default(), WithRequireRedis())

	err := enforcer.CheckConcurrentRunLimit(context.Background(), "org-nil-redis")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"service_degraded",
		le.
			Code)

}

func TestEnforcer_CheckProjectLimit(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	// Free tier: 1 project max. Having 1 project means count >= limit = blocked.
	store.projects = map[string][]string{
		"org_full": {"p1"},
	}

	err := enforcer.CheckProjectLimit(context.Background(), "org_full")
	require.Error(t,
		err)

	// With 0 projects (under limit), should pass.
	store.projects["org_empty"] = []string{}
	require.NoError(t,
		enforcer.
			CheckProjectLimit(context.
				Background(), "org_empty",
			))

}

func TestEnforcer_CheckProjectLimit_PlanLimitLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{
		getOrgSubscriptionFn: func(context.Context, string) (*OrgSubscription, error) {
			return nil, errors.New("subscription store unavailable")
		},
	}, nil, slog.Default())

	err := enforcer.CheckProjectLimit(context.Background(), "org-plan-error")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"billing_plan_unavailable",

		le.Code,
	)

}

func TestEnforcer_CheckProjectLimit_CountErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{
		countProjectsErr: errors.New("project count unavailable"),
	}, nil, slog.Default())

	err := enforcer.CheckProjectLimit(context.Background(), "org-count-error")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"service_degraded",
		le.
			Code)

}

func TestEnforcer_CheckSpendingLimit_FreeTierZeroSpend_Passes(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.periodSpendByOrg = map[string]int64{
		"org_free": 0,
	}
	require.NoError(t,
		enforcer.
			CheckSpendingLimit(context.
				Background(),
				"org_free",
			))

}

func TestEnforcer_CheckSpendingLimit_FreeTierSpendReadErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.sumSpendErr = errors.New("spend aggregation unavailable")

	err := enforcer.CheckSpendingLimit(context.Background(), "org_free")
	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"service_degraded",
		le.
			Code)

}

func TestEnforcer_CheckSpendingLimit_FreeTierAnySpend_Blocks(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	// Orchestration-only: free tier has no included compute credit.
	// Any spend triggers the limit immediately.
	store.periodSpendByOrg = map[string]int64{
		"org_free": 1,
	}

	err := enforcer.CheckSpendingLimit(context.Background(), "org_free")
	require.Error(t,
		err)

}

func TestEnforcer_CheckSpendingLimit_FreeTierOverBudget_Blocks(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.periodSpendByOrg = map[string]int64{
		"org_free": 1_250_000,
	}

	err := enforcer.CheckSpendingLimit(context.Background(), "org_free")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"spending_limit_reached",

		le.Code,
	)
	require.EqualValues(t, 0, le.Limit)

}

func TestEnforcer_CheckSpendingLimit_FreeSubscriptionOverIncludedCredit(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org_free": {OrgID: "org_free", PlanTier: "free", Status: "active", SpendingLimitMicrousd: -1},
	}
	store.periodSpendByOrg = map[string]int64{
		"org_free": 1_100_000,
	}

	err := enforcer.CheckSpendingLimit(context.Background(), "org_free")
	require.Error(t,
		err)

}

func TestEnforcer_CheckSpendingLimit_HardCapZeroBlocksAnySpend(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	// Orchestration-only: no included compute credit. A $0 spending cap means
	// the org is blocked as soon as any spend is recorded.
	store.subscriptions = map[string]*OrgSubscription{
		"org_starter": {
			OrgID:                 "org_starter",
			PlanTier:              "starter",
			Status:                "active",
			SpendingLimitMicrousd: 0,
			LimitAction:           "reject",
		},
	}
	store.periodSpendByOrg = map[string]int64{
		"org_starter": 0,
	}
	require.NoError(t,
		enforcer.
			CheckSpendingLimit(context.
				Background(),
				"org_starter",
			))

	store.periodSpendByOrg["org_starter"] = 1
	err := enforcer.CheckSpendingLimit(context.Background(), "org_starter")
	require.Error(t,
		err)

}

func TestEnforcer_GetOrgPlanLimits_Cache(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org_cached": {OrgID: "org_cached", PlanTier: "pro", Status: "active"},
	}

	ctx := context.Background()
	limits1, err := enforcer.GetOrgPlanLimits(ctx, "org_cached")
	require.NoError(t,
		err)
	assert.Equal(t, domain.
		PlanPro,
		limits1.
			PlanTier)

	// Change plan in store, cache should still return pro
	store.subscriptions["org_cached"].PlanTier = "free"
	limits2, err := enforcer.GetOrgPlanLimits(ctx, "org_cached")
	require.NoError(t,
		err)
	assert.Equal(t, domain.
		PlanPro,
		limits2.
			PlanTier)

	// Invalidate cache
	enforcer.InvalidateOrgCache("org_cached")
	limits3, err := enforcer.GetOrgPlanLimits(ctx, "org_cached")
	require.NoError(t,
		err)
	assert.Equal(t, domain.
		PlanFree,
		limits3.
			PlanTier)

}

func TestReconcileConcurrentRunCount(t *testing.T) {
	t.Parallel()
	enforcer, _, mr := setupEnforcer(t)

	ctx := context.Background()

	// Manually set Redis counter to 10
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rdb.Set(ctx, "strait:org_concurrent:org_recon", 10, 0)
	require.NoError(t,
		enforcer.
			ReconcileConcurrentRunCount(ctx,
				"org_recon",

				3))

	// Reconcile with actual count of 3

	val, err := rdb.Get(ctx, "strait:org_concurrent:org_recon").Int64()
	require.NoError(t,
		err)
	assert.EqualValues(t, 3,
		val)

}

func TestConcurrentCounter_CrashRecovery(t *testing.T) {
	t.Parallel()
	enforcer, store, mr := setupEnforcer(t)

	ctx := context.Background()

	// Use a pro-tier org so the concurrent limit is high enough to simulate a crash.
	store.subscriptions = map[string]*OrgSubscription{
		"org_crash": {OrgID: "org_crash", PlanTier: "pro", Status: "active"},
	}

	// Simulate 5 runs started (increment without decrement = crash scenario).
	for range 5 {
		require.NoError(t,
			enforcer.
				CheckConcurrentRunLimit(ctx, "org_crash"),
		)

	}
	require.NoError(t,
		enforcer.
			ReconcileConcurrentRunCount(ctx,
				"org_crash",

				2))

	// Reconcile: actual executing count is 2 (3 crashed).

	// Verify counter is now 2.
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	val, err := rdb.Get(ctx, "strait:org_concurrent:org_crash").Int64()
	require.NoError(t,
		err)
	assert.EqualValues(t, 2,
		val)

}

func isLimitError(err error, target **LimitError) bool {
	var le *LimitError
	if errors.As(err, &le) {
		*target = le
		return true
	}
	return false
}

// mockExecutingRunCounter implements ExecutingRunCounter for tests.
type mockExecutingRunCounter struct {
	orgCounts map[string]int
	listOrgs  []string
	listErr   error
	countErr  map[string]error
}

func (m *mockExecutingRunCounter) CountExecutingRunsByOrg(_ context.Context, orgID string) (int, error) {
	if m.countErr != nil {
		if err, ok := m.countErr[orgID]; ok {
			return 0, err
		}
	}
	return m.orgCounts[orgID], nil
}

func (m *mockExecutingRunCounter) BulkCountExecutingRunsByOrg(_ context.Context, orgIDs []string) (map[string]int, error) {
	result := make(map[string]int, len(orgIDs))
	for _, orgID := range orgIDs {
		if m.countErr != nil {
			if err, ok := m.countErr[orgID]; ok {
				return nil, err
			}
		}
		result[orgID] = m.orgCounts[orgID]
	}
	return result, nil
}

func (m *mockExecutingRunCounter) ListOrgsWithExecutingRuns(_ context.Context) ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listOrgs, nil
}

func TestConcurrentCounterTTL_Is24Hours(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 24*
		time.Hour,
		concurrentCounterTTL,
	)

}

func TestReconcileAll_RestoresExpiredKey(t *testing.T) {
	t.Parallel()
	enforcer, _, mr := setupEnforcer(t)
	ctx := context.Background()

	// DB reports org-X has 3 executing runs. Redis has no key (expired).
	counter := &mockExecutingRunCounter{
		orgCounts: map[string]int{"org-X": 3},
		listOrgs:  []string{"org-X"},
	}
	require.NoError(t,
		enforcer.
			ReconcileAllConcurrentCounts(ctx,
				counter,
			),
	)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	val, err := rdb.Get(ctx, "strait:org_concurrent:org-X").Int64()
	require.NoError(t,
		err)
	assert.EqualValues(t, 3,
		val)

}

func TestReconcileAll_ResetsStaleKey(t *testing.T) {
	t.Parallel()
	enforcer, _, mr := setupEnforcer(t)
	ctx := context.Background()

	// Redis has stale key for org-Y, DB says 0 runs.
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	rdb.Set(ctx, "strait:org_concurrent:org-Y", 5, 0)

	counter := &mockExecutingRunCounter{
		orgCounts: map[string]int{"org-Y": 0},
		listOrgs:  []string{},
	}
	require.NoError(t,
		enforcer.
			ReconcileAllConcurrentCounts(ctx,
				counter,
			),
	)

	val, err := rdb.Get(ctx, "strait:org_concurrent:org-Y").Int64()
	require.NoError(t,
		err)
	assert.EqualValues(t, 0,
		val)

}

func TestReconcileAll_HandlesDBAndRedisUnion(t *testing.T) {
	t.Parallel()
	enforcer, _, mr := setupEnforcer(t)
	ctx := context.Background()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	// Redis has keys for org-B (stale) and org-C (stale).
	rdb.Set(ctx, "strait:org_concurrent:org-B", 5, 0)
	rdb.Set(ctx, "strait:org_concurrent:org-C", 2, 0)

	// DB has org-A (3 runs) and org-B (1 run), org-C (0 runs).
	counter := &mockExecutingRunCounter{
		orgCounts: map[string]int{"org-A": 3, "org-B": 1, "org-C": 0},
		listOrgs:  []string{"org-A", "org-B"},
	}
	require.NoError(t,
		enforcer.
			ReconcileAllConcurrentCounts(ctx,
				counter,
			),
	)

	for org, want := range map[string]int64{"org-A": 3, "org-B": 1, "org-C": 0} {
		val, err := rdb.Get(ctx, "strait:org_concurrent:"+org).Int64()
		require.NoError(t,
			err)
		assert.Equal(t, want,
			val)

	}
}

func TestReconcileAll_BulkQueryError_ReturnsError(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)
	ctx := context.Background()

	counter := &mockExecutingRunCounter{
		orgCounts: map[string]int{"org-Y": 2},
		listOrgs:  []string{"org-X", "org-Y"},
		// BulkCountExecutingRunsByOrg will fail for org-X, returning error for entire batch.
		countErr: map[string]error{"org-X": errors.New("db error")},
	}

	err := enforcer.ReconcileAllConcurrentCounts(ctx, counter)
	require.Error(t,
		err)

}

func TestReconcileAll_NilRedis(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, nil, slog.Default())

	counter := &mockExecutingRunCounter{}
	require.NoError(t,
		enforcer.
			ReconcileAllConcurrentCounts(context.
				Background(), counter))

}

func TestReserveWorkerConnection_EnforcesCapAcrossEnforcers(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdbA := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdbA.Close() })
	rdbB := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdbB.Close() })
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org_workers": {OrgID: "org_workers", PlanTier: string(domain.PlanFree), Status: "active"},
		},
	}
	enforcerA := NewEnforcer(store, rdbA, slog.Default())
	enforcerB := NewEnforcer(store, rdbB, slog.Default())
	ctx := context.Background()

	release, err := enforcerA.ReserveWorkerConnection(ctx, "org_workers", "replica-a-worker", time.Minute)
	require.NoError(t,
		err)

	t.Cleanup(release)

	_, err = enforcerB.ReserveWorkerConnection(ctx, "org_workers", "replica-b-worker", time.Minute)
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, errors.As(err,
		&le))
	require.Equal(t,
		"worker_connections_reached",

		le.
			Code)

	release()
	if _, err := enforcerB.ReserveWorkerConnection(ctx, "org_workers", "replica-b-worker", time.Minute); err != nil {
		require.Failf(t, "test failure",

			"reservation after release should pass: %v", err)
	}
}

func TestReserveWorkerConnection_PlanLimitLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	store := &mockBillingStore{
		getOrgSubscriptionFn: func(context.Context, string) (*OrgSubscription, error) {
			return nil, errors.New("subscription store unavailable")
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	_, err := enforcer.ReserveWorkerConnection(context.Background(), "org-plan-error", "worker-1", time.Minute)
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"billing_plan_unavailable",

		le.Code,
	)

}

func TestReserveWorkerConnection_RedisErrorFailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	require.NoError(t,
		rdb.Close())

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org_workers": {OrgID: "org_workers", PlanTier: string(domain.PlanFree), Status: "active"},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	_, err := enforcer.ReserveWorkerConnection(context.Background(), "org_workers", "worker-1", time.Minute)
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"service_degraded",
		le.
			Code)

}

func TestReserveWorkerConnection_NilRedisFailsClosed(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org_workers": {OrgID: "org_workers", PlanTier: string(domain.PlanFree), Status: "active"},
		},
	}
	enforcer := NewEnforcer(store, nil, slog.Default())

	_, err := enforcer.ReserveWorkerConnection(context.Background(), "org_workers", "worker-1", time.Minute)
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"service_degraded",
		le.
			Code)

}

func TestCheckWorkerConnectionLimit_PlanLimitLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()

	enforcer := NewEnforcer(&mockBillingStore{
		getOrgSubscriptionFn: func(context.Context, string) (*OrgSubscription, error) {
			return nil, errors.New("subscription store unavailable")
		},
	}, nil, slog.Default())

	err := enforcer.CheckWorkerConnectionLimit(context.Background(), "org-plan-error", 0)
	require.Error(t,
		err)
	require.True(t, strings.Contains(err.Error(), "resolve worker connection plan limit"))

}

// Member limit tests.

func TestCheckMemberLimit_FreeUnderLimit_Passes(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	freeLimits := GetPlanLimits(domain.PlanFree)
	store.memberCounts = map[string]int{"org_free": freeLimits.MaxMembersPerOrg - 1}
	require.NoError(t,
		enforcer.
			CheckMemberLimit(context.
				Background(), "org_free",
			))

}

func TestCheckMemberLimit_FreeAtLimit_Blocked(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	freeLimits := GetPlanLimits(domain.PlanFree)
	store.memberCounts = map[string]int{"org_free": freeLimits.MaxMembersPerOrg}

	err := enforcer.CheckMemberLimit(context.Background(), "org_free")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, errors.As(err,
		&le))
	assert.Equal(t, "member_limit_reached",

		le.Code)
	assert.Equal(t, int64(freeLimits.
		MaxMembersPerOrg,
	),
		le.Limit)

}

func TestCheckMemberLimit_PlanLimitLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{
		getOrgSubscriptionFn: func(context.Context, string) (*OrgSubscription, error) {
			return nil, errors.New("subscription store unavailable")
		},
	}, nil, slog.Default())

	err := enforcer.CheckMemberLimit(context.Background(), "org-plan-error")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"billing_plan_unavailable",

		le.Code,
	)

}

func TestCheckMemberLimit_CountErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{
		countMembersErr: errors.New("member count unavailable"),
	}, nil, slog.Default())

	err := enforcer.CheckMemberLimit(context.Background(), "org-count-error")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"service_degraded",
		le.
			Code)

}

func TestCheckMemberLimit_StarterUnderLimit_Passes(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	starterLimits := GetPlanLimits(domain.PlanStarter)
	store.subscriptions = map[string]*OrgSubscription{
		"org_starter": {OrgID: "org_starter", PlanTier: "starter", Status: "active"},
	}
	store.memberCounts = map[string]int{"org_starter": starterLimits.MaxMembersPerOrg - 1}
	require.NoError(t,
		enforcer.
			CheckMemberLimit(context.
				Background(), "org_starter",
			))

}

func TestCheckMemberLimit_StarterAtLimit_Blocked(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	starterLimits := GetPlanLimits(domain.PlanStarter)
	store.subscriptions = map[string]*OrgSubscription{
		"org_starter": {OrgID: "org_starter", PlanTier: "starter", Status: "active"},
	}
	store.memberCounts = map[string]int{"org_starter": starterLimits.MaxMembersPerOrg}

	err := enforcer.CheckMemberLimit(context.Background(), "org_starter")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, errors.As(err,
		&le))
	assert.Equal(t, "member_limit_reached",

		le.Code)
	assert.Equal(t, int64(starterLimits.
		MaxMembersPerOrg,
	), le.Limit,
	)

}

func TestCheckMemberLimit_ProUnlimited_AlwaysPasses(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.subscriptions = map[string]*OrgSubscription{
		"org_ent": {OrgID: "org_ent", PlanTier: "enterprise", Status: "active"},
	}
	store.memberCounts = map[string]int{"org_ent": 1000}
	require.NoError(t,
		enforcer.
			CheckMemberLimit(context.
				Background(), "org_ent",
			))

}

// Org creation limit tests.

func TestCheckOrgCreationLimit_FreeAt1_Passes(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.orgCountsByUser = map[string]int{"user1": 0}
	require.NoError(t,
		enforcer.
			CheckOrgCreationLimit(
				context.Background(),
				"user1", domain.PlanFree,
			),
	)

}

func TestCheckOrgCreationLimit_FreeAt1_Blocked(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.orgCountsByUser = map[string]int{"user1": 1}

	err := enforcer.CheckOrgCreationLimit(context.Background(), "user1", domain.PlanFree)
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, errors.As(err,
		&le))
	assert.Equal(t, "org_limit_reached",

		le.
			Code)
	assert.EqualValues(t, 1,
		le.Limit,
	)

}

func TestCheckOrgCreationLimit_CountErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{
		countOrgsByUserErr: errors.New("org count unavailable"),
	}, nil, slog.Default())

	err := enforcer.CheckOrgCreationLimit(context.Background(), "user-count-error", domain.PlanFree)
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"service_degraded",
		le.
			Code)

}

func TestCheckOrgCreationLimit_StarterAt2_Passes(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.orgCountsByUser = map[string]int{"user2": 1}
	require.NoError(t,
		enforcer.
			CheckOrgCreationLimit(
				context.Background(),
				"user2", domain.PlanStarter,
			))

}

func TestCheckOrgCreationLimit_ProUnlimited(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.orgCountsByUser = map[string]int{"user3": 100}
	require.NoError(t,
		enforcer.
			CheckOrgCreationLimit(
				context.Background(),
				"user3", domain.PlanEnterprise,
			))

}

// 80% daily run warning tests.

func TestCheck80PercentWarning_UnlimitedAlwaysFalse(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	// All plans now have unlimited daily runs, so 80% warning should always be false.
	ctx := context.Background()
	for range 1000 {
		_ = enforcer.CheckDailyRunLimit(ctx, "org_warn_unlimited")
	}

	warned, err := enforcer.Check80PercentDailyRunWarning(ctx, "org_warn_unlimited")
	require.NoError(t,
		err)
	assert.False(t, warned)

}

func TestCheck80PercentWarning_Unlimited_False(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.subscriptions = map[string]*OrgSubscription{
		"org_ent": {OrgID: "org_ent", PlanTier: "enterprise", Status: "active"},
	}

	ctx := context.Background()
	for range 100 {
		_ = enforcer.CheckDailyRunLimit(ctx, "org_ent")
	}

	warned, err := enforcer.Check80PercentDailyRunWarning(ctx, "org_ent")
	require.NoError(t,
		err)
	assert.False(t, warned)

}

func TestCheck80PercentWarning_ZeroUsage_False(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	warned, err := enforcer.Check80PercentDailyRunWarning(context.Background(), "org_zero")
	require.NoError(t,
		err)
	assert.False(t, warned)

}

// Grace period enforcement tests.

func TestGracePeriod_InFlight_Allowed_DuringGrace(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	graceEnd := time.Now().Add(24 * time.Hour)
	store.subscriptions = map[string]*OrgSubscription{
		"org_grace": {
			OrgID:          "org_grace",
			PlanTier:       "starter",
			Status:         "active",
			PaymentStatus:  "grace",
			GracePeriodEnd: &graceEnd,
		},
	}
	require.NoError(t,
		enforcer.
			CheckDailyRunLimit(context.
				Background(),
				"org_grace",
			))

	// During active grace, daily run limit should be skipped (allowed).

}

func TestGracePeriod_InFlight_Rejected_AfterGrace(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	graceEnd := time.Now().Add(-1 * time.Hour) // expired
	store.subscriptions = map[string]*OrgSubscription{
		"org_expired": {
			OrgID:          "org_expired",
			PlanTier:       "starter",
			Status:         "active",
			PaymentStatus:  "grace",
			GracePeriodEnd: &graceEnd,
		},
	}

	err := enforcer.CheckDailyRunLimit(context.Background(), "org_expired")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, errors.As(err,
		&le))
	assert.Equal(t, "grace_period_expired",

		le.Code)

}

func TestGracePeriod_PaymentOK_NoGraceCheck(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org_ok": {
			OrgID:         "org_ok",
			PlanTier:      "starter",
			Status:        "active",
			PaymentStatus: "ok",
		},
	}
	require.NoError(t,
		enforcer.
			CheckDailyRunLimit(context.
				Background(),
				"org_ok",
			))

	// Normal limit checking: should succeed without grace period interference.

}

func TestGracePeriod_PaymentRestricted_RejectedImmediately(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org_restricted": {
			OrgID:         "org_restricted",
			PlanTier:      "starter",
			Status:        "active",
			PaymentStatus: "restricted",
		},
	}

	err := enforcer.CheckDailyRunLimit(context.Background(), "org_restricted")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, errors.As(err,
		&le))
	assert.Equal(t, "payment_restricted",

		le.
			Code)

}

func TestDeepSecPaymentSuspendedRejectedImmediately(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org_suspended": {
			OrgID:         "org_suspended",
			PlanTier:      "starter",
			Status:        "active",
			PaymentStatus: "suspended",
		},
	}

	err := enforcer.CheckDailyRunLimit(context.Background(), "org_suspended")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, errors.As(err,
		&le))
	assert.Equal(t, "payment_suspended",

		le.
			Code)

}

func TestGracePeriod_ConcurrentLimit_StillChecked_DuringGrace(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	graceEnd := time.Now().Add(24 * time.Hour)
	store.subscriptions = map[string]*OrgSubscription{
		"org_conc_grace": {
			OrgID:          "org_conc_grace",
			PlanTier:       "starter",
			Status:         "active",
			PaymentStatus:  "grace",
			GracePeriodEnd: &graceEnd,
		},
	}
	require.NoError(t,
		enforcer.
			CheckConcurrentRunLimit(context.Background(), "org_conc_grace"))

	// During active grace, concurrent limit should also be skipped.

	// Also verify restricted concurrent limit is rejected.
	store.subscriptions["org_conc_restricted"] = &OrgSubscription{
		OrgID:         "org_conc_restricted",
		PlanTier:      "starter",
		Status:        "active",
		PaymentStatus: "restricted",
	}
	err := enforcer.CheckConcurrentRunLimit(context.Background(), "org_conc_restricted")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, errors.As(err,
		&le))
	assert.Equal(t, "payment_restricted",

		le.
			Code)

}

// Org billing cache tests.

func TestOrgCache_CacheHitAvoidsDatabaseCall(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	var dbCalls int
	store.getOrgSubscriptionFn = func(_ context.Context, _ string) (*OrgSubscription, error) {
		dbCalls++
		return nil, ErrSubscriptionNotFound
	}

	ctx := context.Background()

	// First call: cache miss, hits DB.
	_, err2 := enforcer.GetOrgPlanLimits(ctx, "org-cache-test")
	require.Nil(t, err2)

	firstDBCalls := dbCalls

	// Second call: cache hit, no additional DB call.
	_, err2 = enforcer.GetOrgPlanLimits(ctx, "org-cache-test")
	require.Nil(t, err2)
	require.Equal(t,
		firstDBCalls,
		dbCalls)

}

func TestOrgCache_InvalidateForcesRefresh(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org-inv": {OrgID: "org-inv", PlanTier: "pro", Status: "active"},
	}

	ctx := context.Background()

	// Populate cache.
	limits1, _ := enforcer.GetOrgPlanLimits(ctx, "org-inv")
	require.Equal(t,
		domain.PlanPro,
		limits1.
			PlanTier)

	// Change underlying data.
	store.subscriptions["org-inv"].PlanTier = "starter"

	// Without invalidation, should still return cached pro.
	limits2, _ := enforcer.GetOrgPlanLimits(ctx, "org-inv")
	require.Equal(t,
		domain.PlanPro,
		limits2.
			PlanTier)

	// After invalidation, should reflect new plan.
	enforcer.InvalidateOrgCache("org-inv")
	limits3, _ := enforcer.GetOrgPlanLimits(ctx, "org-inv")
	require.Equal(t,
		domain.PlanStarter,
		limits3.
			PlanTier,
	)

}

func TestOrgCache_EmptyOrgIDReturnsFree(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	limits, err2 := enforcer.GetOrgPlanLimits(context.Background(), "")
	require.Nil(t, err2)
	require.Equal(t,
		domain.PlanFree,
		limits.
			PlanTier)

}

func TestOrgCache_SubscriptionNotFoundCachesFree(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	var dbCalls int
	store.getOrgSubscriptionFn = func(_ context.Context, _ string) (*OrgSubscription, error) {
		dbCalls++
		return nil, ErrSubscriptionNotFound
	}

	ctx := context.Background()

	// First call: DB miss -> free plan cached.
	limits, _ := enforcer.GetOrgPlanLimits(ctx, "org-nosub")
	require.Equal(t,
		domain.PlanFree,
		limits.
			PlanTier)
	require.EqualValues(t, 1, dbCalls)

	// Second call: cache hit, no DB call.
	_, _ = enforcer.GetOrgPlanLimits(ctx, "org-nosub")
	require.EqualValues(t, 1, dbCalls)

}

func TestOrgCache_EnforcementModeFromCache(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org-mode": {OrgID: "org-mode", PlanTier: "pro", Status: "active", EnforcementMode: "warn"},
	}

	ctx := context.Background()

	// Populate cache via GetOrgPlanLimits.
	_, _ = enforcer.GetOrgPlanLimits(ctx, "org-mode")

	// getEnforcementMode should read from cache.
	mode := enforcer.getEnforcementMode(t.Context(), "org-mode")
	require.Equal(t,
		"warn", mode,
	)

}

func TestOrgCache_EnforcementModeFallsBackToEnforce(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	// No cache entry for this org.
	mode := enforcer.getEnforcementMode(t.Context(), "org-uncached")
	require.Equal(t,
		"enforce",
		mode)

}

func TestOrgCache_InvalidateNonexistentKey(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	// Should not panic.
	enforcer.InvalidateOrgCache("org-nonexistent")
}

func TestOrgCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	ctx := context.Background()
	const goroutines = 50
	var wg conc.WaitGroup

	for range goroutines {
		wg.Go(func() {
			for range 20 {
				_, _ = enforcer.GetOrgPlanLimits(ctx, "org-conc-cache")
			}
		})
	}

	// Invalidators running concurrently.
	for range 10 {
		wg.Go(func() {
			for range 20 {
				enforcer.InvalidateOrgCache("org-conc-cache")
			}
		})
	}

	wg.Wait()
}

func TestEnforcer_GetStripeCustomerID(t *testing.T) {
	t.Parallel()

	t.Run("returns_customer_id", func(t *testing.T) {
		t.Parallel()
		enforcer, store, _ := setupEnforcer(t)
		custID := "cust_abc123"
		store.subscriptions = map[string]*OrgSubscription{
			"org-stripe": {OrgID: "org-stripe", PlanTier: "pro", Status: "active", StripeCustomerID: &custID},
		}

		got, err := enforcer.GetStripeCustomerID(context.Background(), "org-stripe")
		require.NoError(t,
			err)
		assert.Equal(t, "cust_abc123",

			got)

	})

	t.Run("returns_empty_when_nil", func(t *testing.T) {
		t.Parallel()
		enforcer, store, _ := setupEnforcer(t)
		store.subscriptions = map[string]*OrgSubscription{
			"org-nil": {OrgID: "org-nil", PlanTier: "starter", Status: "active", StripeCustomerID: nil},
		}

		got, err := enforcer.GetStripeCustomerID(context.Background(), "org-nil")
		require.NoError(t,
			err)
		assert.Equal(t, "",
			got)

	})

	t.Run("returns_empty_when_empty_string", func(t *testing.T) {
		t.Parallel()
		enforcer, store, _ := setupEnforcer(t)
		empty := ""
		store.subscriptions = map[string]*OrgSubscription{
			"org-empty": {OrgID: "org-empty", PlanTier: "starter", Status: "active", StripeCustomerID: &empty},
		}

		got, err := enforcer.GetStripeCustomerID(context.Background(), "org-empty")
		require.NoError(t,
			err)
		assert.Equal(t, "",
			got)

	})

	t.Run("returns_error_when_not_found", func(t *testing.T) {
		t.Parallel()
		enforcer, _, _ := setupEnforcer(t)

		_, err := enforcer.GetStripeCustomerID(context.Background(), "org-nonexistent")
		require.Error(t,
			err)
		assert.True(t, errors.Is(err,
			ErrSubscriptionNotFound,
		))

	})
}

func TestEnforcer_CheckDailyRunLimit_ProOverageAllowed(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org_pro": {OrgID: "org_pro", PlanTier: "pro", Status: "active"},
	}

	limits := GetPlanLimits(domain.PlanPro)
	for range limits.MaxRunsPerDay {
		require.NoError(t,
			enforcer.
				CheckDailyRunLimit(context.
					Background(),
					"org_pro",
				))

	}

	// Pro plans allow overage -- no error expected past the daily limit.
	err := enforcer.CheckDailyRunLimit(context.Background(), "org_pro")
	require.NoError(t,
		err)

}

func TestEnforcer_GetOrgPlanLimits_AllowsHTTPMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		planTier string
		want     bool
	}{
		{"free", "free", true},
		{"starter", "starter", true},
		{"pro", "pro", true},
		{"enterprise", "enterprise", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			enforcer, store, _ := setupEnforcer(t)

			orgID := "org_" + tt.name
			store.subscriptions = map[string]*OrgSubscription{
				orgID: {OrgID: orgID, PlanTier: tt.planTier, Status: "active"},
			}

			limits, err := enforcer.GetOrgPlanLimits(context.Background(), orgID)
			require.NoError(t,
				err)
			assert.Equal(t, tt.
				want, limits.
				AllowsHTTPMode,
			)

		})
	}
}

func TestEnforcer_GetOrgPlanLimits_NoSubscription_DefaultsFree(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	// No subscription = defaults to free plan.
	limits, err := enforcer.GetOrgPlanLimits(context.Background(), "org_unknown")
	require.NoError(t,
		err)
	assert.True(t, limits.
		AllowsHTTPMode,
	)
	assert.Equal(t, domain.
		PlanFree,
		limits.
			PlanTier)

}

func TestEnforcer_GetDailyRunCount_EmptyOrgID(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)
	count, err := enforcer.GetDailyRunCount(context.Background(), "")
	require.NoError(t,
		err)
	assert.EqualValues(t, 0,
		count)

}

func TestEnforcer_GetDailyRunCount_NoKey(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)
	count, err := enforcer.GetDailyRunCount(context.Background(), "org-missing")
	require.NoError(t,
		err)
	assert.EqualValues(t, 0,
		count)

}

func TestEnforcer_GetDailyRunCount_WithRuns(t *testing.T) {
	t.Parallel()
	enforcer, _, mr := setupEnforcer(t)

	key := "strait:org_runs:org-counted:" + time.Now().UTC().Format("2006-01-02")
	require.NoError(t,
		mr.Set(key,
			"5"))

	count, err := enforcer.GetDailyRunCount(context.Background(), "org-counted")
	require.NoError(t,
		err)
	assert.EqualValues(t, 5,
		count)

}

func TestEnforcer_GetOrgPlanLimits_WithAddons(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org-addon": {OrgID: "org-addon", PlanTier: "pro", Status: "active"},
	}
	store.activeAddons = []Addon{
		{AddonType: AddonConcurrency100, Quantity: 2, Active: true},
	}

	limits, err := enforcer.GetOrgPlanLimits(context.Background(), "org-addon")
	require.NoError(t,
		err)

	baseLimits := GetPlanLimits(domain.PlanPro)
	wantConcurrent := baseLimits.MaxConcurrentRuns + 200
	assert.Equal(t, wantConcurrent,

		limits.
			MaxConcurrentRuns,
	)

	// 2 packs x 100

}

func TestEnforcer_GetOrgPlanLimits_AddonLoadErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org-addon-error": {OrgID: "org-addon-error", PlanTier: "pro", Status: "active"},
	}
	store.listActiveAddonsErr = errors.New("addon store unavailable")

	_, err := enforcer.GetOrgPlanLimits(context.Background(), "org-addon-error")
	require.Error(t,
		err)
	require.True(t, strings.Contains(err.Error(), "listing active add-ons"))

}

func TestEnforcer_CheckConcurrentRunLimit_UnlimitedEnterprise(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org-ent": {OrgID: "org-ent", PlanTier: "enterprise", Status: "active"},
	}
	store.executingRuns = map[string]int{
		"org-ent": 999,
	}

	err := enforcer.CheckConcurrentRunLimit(context.Background(), "org-ent")
	require.NoError(t,
		err)

}

func TestEnforcer_NewEnforcer_CacheTTL(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)
	assert.Equal(t, 5*
		time.Minute,
		enforcer.
			cacheTTL,
	)

}

func TestEnforcer_Check80PercentDailyRunWarning_EmptyOrgID(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)
	warned, err := enforcer.Check80PercentDailyRunWarning(context.Background(), "")
	require.NoError(t,
		err)
	assert.False(t, warned)

}

// CheckMaxDispatchPriority -- fail-closed on DB errors.

// TestCheckMaxDispatchPriority_DBError_FailsClosed verifies that a DB error
// when resolving the org causes CheckMaxDispatchPriority to fail closed
// (return a *LimitError) rather than allowing the request.
func TestCheckMaxDispatchPriority_DBError_FailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		getProjectOrgIDFn: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("simulated db outage")
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	err := enforcer.CheckMaxDispatchPriority(context.Background(), "proj-1", 5)
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"dispatch_priority_exceeded",

		le.
			Code)

}

// TestCheckMaxDispatchPriority_PlanLimitsError_FailsClosed verifies that a DB
// error when loading plan limits also causes fail-closed behavior.
func TestCheckMaxDispatchPriority_PlanLimitsError_FailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		getProjectOrgIDFn: func(_ context.Context, _ string) (string, error) {
			return "org-1", nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*OrgSubscription, error) {
			return nil, errors.New("simulated subscription lookup failure")
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	err := enforcer.CheckMaxDispatchPriority(context.Background(), "proj-1", 5)
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))

}

// TestCheckMaxDispatchPriority_WithinCap_Allows verifies that the shared
// launch priority range allows positive priorities on every plan.
func TestCheckMaxDispatchPriority_WithinCap_Allows(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org-free": {OrgID: "org-free", PlanTier: "free", Status: "active"},
	}
	store.getProjectOrgIDFn = func(_ context.Context, _ string) (string, error) {
		return "org-free", nil
	}

	limits := GetPlanLimits("free")
	require.GreaterOrEqual(t, limits.
		MaxDispatchPriority,

		5)
	require.NoError(t,
		enforcer.
			CheckMaxDispatchPriority(context.
				Background(), "proj-free", 5))

}

// TestCheckMaxDispatchPriority_ExceedsCap_Blocks verifies that a priority
// above the shared platform cap returns a *LimitError.
func TestCheckMaxDispatchPriority_ExceedsCap_Blocks(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org-free": {OrgID: "org-free", PlanTier: "free", Status: "active"},
	}
	store.getProjectOrgIDFn = func(_ context.Context, _ string) (string, error) {
		return "org-free", nil
	}

	err := enforcer.CheckMaxDispatchPriority(context.Background(), "proj-free", 11)
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"dispatch_priority_exceeded",

		le.
			Code)

}

// TestCheckMaxDispatchPriority_ZeroPriority_AlwaysAllowed verifies that
// requestedPriority=0 is always allowed regardless of errors.
func TestCheckMaxDispatchPriority_ZeroPriority_AlwaysAllowed(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		getProjectOrgIDFn: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("should not be called for priority=0")
		},
	}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	enforcer := NewEnforcer(store, rdb, slog.Default())
	require.NoError(t,
		enforcer.
			CheckMaxDispatchPriority(context.
				Background(), "proj-1", 0))

}

// DecrMonthlyRunCount -- mirrors DecrDailyRunCount behavior for monthly quota.

// TestDecrMonthlyRunCount_DecrAfterIncr verifies that decrementing after an
// increment returns the counter to the baseline value.
func TestDecrMonthlyRunCount_DecrAfterIncr(t *testing.T) {
	t.Parallel()
	enforcer, store, mr := setupEnforcer(t)
	ctx := context.Background()

	// Wire a starter subscription so the monthly limit kicks in.
	store.subscriptions = map[string]*OrgSubscription{
		"org-monthly-1": {OrgID: "org-monthly-1", PlanTier: "starter", Status: "active"},
	}
	require.NoError(t,
		enforcer.
			CheckMonthlyRunLimit(ctx,
				"org-monthly-1",
			),
	)

	// CheckMonthlyRunLimit increments the counter atomically on each allowed call.

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	key := monthlyRunKey("org-monthly-1", time.Now())
	before, _ := rdb.Get(ctx, key).Int64()
	require.EqualValues(t, 1, before)

	// Rollback (abort path).
	enforcer.DecrMonthlyRunCount(ctx, "org-monthly-1")

	after, _ := rdb.Get(ctx, key).Int64()
	assert.EqualValues(t, 0,
		after)

}

func TestCheckMonthlyRunLimit_PaymentLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		getOrgSubscriptionFn: func(context.Context, string) (*OrgSubscription, error) {
			return nil, errors.New("subscription store unavailable")
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	err := enforcer.CheckMonthlyRunLimit(context.Background(), "org-payment-error")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"service_degraded",
		le.
			Code)

}

func TestCheckMonthlyRunLimit_RedisErrorFailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	require.NoError(t,
		rdb.Close())

	orgID := "org-monthly-redis-error"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			orgID: {OrgID: orgID, PlanTier: "starter", Status: "active"},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	err := enforcer.CheckMonthlyRunLimit(context.Background(), orgID)
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"service_degraded",
		le.
			Code)

}

func TestCheckMonthlyRunLimit_RequiredNilRedisFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{}, nil, slog.Default(), WithRequireRedis())

	err := enforcer.CheckMonthlyRunLimit(context.Background(), "org-monthly-nil-redis")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"service_degraded",
		le.
			Code)

}

func TestCheckMonthlyRunLimit_PaidOverageDisabledHardCaps(t *testing.T) {
	t.Parallel()
	enforcer, store, mr := setupEnforcer(t)
	ctx := context.Background()
	orgID := "org-paid-overage-disabled"
	store.subscriptions = map[string]*OrgSubscription{
		orgID: {OrgID: orgID, PlanTier: "starter", Status: "active", OverageDisabled: true},
	}

	limits := GetPlanLimits(domain.PlanStarter)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	require.NoError(t,
		rdb.Set(
			ctx, monthlyRunKey(orgID,
				time.Now()), int64(limits.MaxRunsPerMonth),

			0).Err())

	err := enforcer.CheckMonthlyRunLimitForRun(ctx, orgID, "run-paid-disabled")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, errors.As(err,
		&le))
	require.Equal(t,
		"plan_cap_reached",
		le.
			Code)
	require.False(t,
		store.pausedOrgID !=
			orgID ||
			store.
				pausedReason !=
				"quota_exceeded",
	)

}

func TestCheckMonthlyRunLimit_PaidOverageEnabledAllowsPastAllowance(t *testing.T) {
	t.Parallel()
	enforcer, store, mr := setupEnforcer(t)
	ctx := context.Background()
	orgID := "org-paid-overage-enabled"
	runID := "run-paid-enabled"
	store.subscriptions = map[string]*OrgSubscription{
		orgID: {OrgID: orgID, PlanTier: "starter", Status: "active"},
	}

	limits := GetPlanLimits(domain.PlanStarter)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	require.NoError(t,
		rdb.Set(
			ctx, monthlyRunKey(orgID,
				time.Now()), int64(limits.MaxRunsPerMonth),

			0).Err())
	require.NoError(t,
		enforcer.
			CheckMonthlyRunLimitForRun(ctx, orgID,
				runID,
			))

	if got := enforcer.IsRunOverage(ctx, runID); !got {
		require.Fail(t,

			"over-allowance run should be marked for Stripe overage metering")
	}
}

type overageMarkerFailRedis struct {
	redis.Cmdable
}

func (r overageMarkerFailRedis) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	cmd := redis.NewStatusCmd(ctx, "set", key, value, expiration)
	if strings.HasPrefix(key, "billing:run_overage:") {
		cmd.SetErr(errors.New("simulated overage marker outage"))
		return cmd
	}
	return r.Cmdable.Set(ctx, key, value, expiration)
}

func TestCheckMonthlyRunLimit_OverageMarkerFailureFailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	baseRedis := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { baseRedis.Close() })

	ctx := context.Background()
	orgID := "org-paid-overage-marker-error"
	runID := "run-paid-marker-error"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			orgID: {OrgID: orgID, PlanTier: "starter", Status: "active"},
		},
	}
	limits := GetPlanLimits(domain.PlanStarter)
	require.NoError(t,
		baseRedis.
			Set(ctx, monthlyRunKey(orgID, time.
				Now(),
			),
				int64(limits.MaxRunsPerMonth), 0).Err())

	enforcer := NewEnforcer(store, overageMarkerFailRedis{Cmdable: baseRedis}, slog.Default())

	err := enforcer.CheckMonthlyRunLimitForRun(ctx, orgID, runID)
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err, &le))
	require.Equal(t,
		"service_degraded",
		le.
			Code)

	if got := enforcer.IsRunOverage(ctx, runID); got {
		require.Fail(t,

			"failed marker write must not leave a run marked as overage")
	}
}

func TestCheckMonthlyRunLimit_FreeCardOverageOptInAllowsPastAllowance(t *testing.T) {
	t.Parallel()
	enforcer, store, mr := setupEnforcer(t)
	ctx := context.Background()
	orgID := "org-free-card-overage"
	customerID := "cus_free_card"
	store.subscriptions = map[string]*OrgSubscription{
		orgID: {
			OrgID:            orgID,
			PlanTier:         "free",
			Status:           "active",
			StripeCustomerID: &customerID,
			OverageDisabled:  false,
		},
	}

	limits := GetPlanLimits(domain.PlanFree)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	require.NoError(t,
		rdb.Set(
			ctx, monthlyRunKey(orgID,
				time.Now()), int64(limits.MaxRunsPerMonth),

			0).Err())
	require.NoError(t,
		enforcer.
			CheckMonthlyRunLimitForRun(ctx, orgID,
				"run-free-card",
			))

}

// TestDecrMonthlyRunCount_FloorsAtZero verifies that decrementing from 0 does
// not produce a negative value.
func TestDecrMonthlyRunCount_FloorsAtZero(t *testing.T) {
	t.Parallel()
	enforcer, _, mr := setupEnforcer(t)
	ctx := context.Background()

	// Decrement with no prior increment — should be a no-op.
	enforcer.DecrMonthlyRunCount(ctx, "org-monthly-floor")
	enforcer.DecrMonthlyRunCount(ctx, "org-monthly-floor")

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	key := monthlyRunKey("org-monthly-floor", time.Now())
	val, err := rdb.Get(ctx, key).Int64()
	assert.False(t, err ==
		nil &&
		val < 0)

	// If the key doesn't exist (redis.Nil) that's also acceptable (counter = 0).
}

// TestDecrMonthlyRunCount_EmptyOrgID verifies that an empty orgID is a no-op.
func TestDecrMonthlyRunCount_EmptyOrgID(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)
	// Should not panic.
	enforcer.DecrMonthlyRunCount(context.Background(), "")
}

// TestDecrMonthlyRunCount_NilRedis verifies that a nil Redis client is a no-op.
func TestDecrMonthlyRunCount_NilRedis(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, nil, slog.Default())
	// Should not panic.
	enforcer.DecrMonthlyRunCount(context.Background(), "org-1")
}

// TestDecrMonthlyRunCount_Parallel verifies that parallel incr/decr operations
// leave the counter consistent (no race condition, no negative value).
func TestDecrMonthlyRunCount_Parallel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	enforcer, store, mr := setupEnforcer(t)
	ctx := context.Background()

	store.subscriptions = map[string]*OrgSubscription{
		"org-parallel": {OrgID: "org-parallel", PlanTier: "starter", Status: "active"},
	}

	const ops = 50
	done := make(chan struct{})
	concWG.Go(func() {
		for range ops {
			_ = enforcer.CheckMonthlyRunLimit(ctx, "org-parallel")
		}
		close(done)
	})
	concWG.Go(func() {
		for range ops {
			enforcer.DecrMonthlyRunCount(ctx, "org-parallel")
		}
	})
	<-done

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	key := monthlyRunKey("org-parallel", time.Now())
	val, err := rdb.Get(ctx, key).Int64()
	assert.False(t, err ==
		nil &&
		val < 0)

}
