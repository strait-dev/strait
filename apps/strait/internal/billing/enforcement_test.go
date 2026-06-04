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
		if err := enforcer.CheckDailyRunLimit(context.Background(), "org_free"); err != nil {
			t.Fatalf("unexpected limit error: daily runs should be unlimited for free tier: %v", err)
		}
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
		if err := enforcer.CheckDailyRunLimit(context.Background(), "org_starter"); err != nil {
			t.Fatalf("unexpected error: daily runs should be unlimited for starter tier: %v", err)
		}
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
		if err := enforcer.CheckDailyRunLimit(context.Background(), "org_ent"); err != nil {
			t.Fatalf("enterprise should be unlimited: %v", err)
		}
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
	for i := range 10_000 {
		if err := enforcer.CheckDailyRunLimit(ctx, "org_free_explicit"); err != nil {
			t.Fatalf("unexpected error at run %d: daily runs should be unlimited: %v", i+1, err)
		}
	}
}

func TestEnforcer_CheckDailyRunLimit_EmptyOrgID(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	if err := enforcer.CheckDailyRunLimit(context.Background(), ""); err != nil {
		t.Fatalf("empty org_id should pass: %v", err)
	}
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

	// Should still allow runs (unlimited).
	if err := enforcer.CheckDailyRunLimit(ctx, "org_rollback"); err != nil {
		t.Fatalf("should allow after decrement: %v", err)
	}
}

func TestEnforcer_CheckConcurrentRunLimit(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	ctx := context.Background()
	// Free plan: ConcurrentFree concurrent runs max.
	for range ConcurrentFree {
		if err := enforcer.CheckConcurrentRunLimit(ctx, "org_conc"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Next run should fail.
	err := enforcer.CheckConcurrentRunLimit(ctx, "org_conc")
	if err == nil {
		t.Fatal("expected concurrent limit error")
	}

	// Decrement one, should allow another.
	enforcer.DecrConcurrentRunCount(ctx, "org_conc")
	if err := enforcer.CheckConcurrentRunLimit(ctx, "org_conc"); err != nil {
		t.Fatalf("should pass after decrement: %v", err)
	}
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
		if err := enforcer.CheckConcurrentRunLimit(ctx, "org_grace"); err != nil {
			t.Fatalf("unexpected error before cap: %v", err)
		}
	}
	err := enforcer.CheckConcurrentRunLimit(ctx, "org_grace")
	if err == nil {
		t.Fatal("expected active grace org to remain subject to concurrent run cap")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T: %v", err, err)
	}
	if le.Code != "org_concurrent_run_limit_exceeded" {
		t.Fatalf("Code = %q, want org_concurrent_run_limit_exceeded", le.Code)
	}
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
	if err == nil {
		t.Fatal("expected concurrent limit check to fail closed when payment status cannot be loaded")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
	}
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
	if err == nil {
		t.Fatal("expected concurrent limit check to fail closed when active add-ons cannot be loaded")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "billing_plan_unavailable" {
		t.Fatalf("Code = %q, want billing_plan_unavailable", le.Code)
	}
}

func TestEnforcer_CheckConcurrentRunLimit_RedisErrorFailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	if err := rdb.Close(); err != nil {
		t.Fatalf("close redis client: %v", err)
	}
	enforcer := NewEnforcer(&mockBillingStore{}, rdb, slog.Default())

	err := enforcer.CheckConcurrentRunLimit(context.Background(), "org-redis-error")
	if err == nil {
		t.Fatal("expected concurrent limit check to fail closed when Redis is unavailable")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
	}
}

func TestEnforcer_CheckConcurrentRunLimit_RequiredNilRedisFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{}, nil, slog.Default(), WithRequireRedis())

	err := enforcer.CheckConcurrentRunLimit(context.Background(), "org-nil-redis")
	if err == nil {
		t.Fatal("expected concurrent limit check to fail closed when required Redis is not configured")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
	}
}

func TestEnforcer_CheckProjectLimit(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	freeLimits := GetPlanLimits(domain.PlanFree)

	// Free tier: 1 project max. Having 1 project means count >= limit = blocked.
	store.projects = map[string][]string{
		"org_full": {"p1"},
	}

	err := enforcer.CheckProjectLimit(context.Background(), "org_full")
	if err == nil {
		t.Fatalf("expected project limit error at %d projects on free plan", freeLimits.MaxProjectsPerOrg)
	}

	// With 0 projects (under limit), should pass.
	store.projects["org_empty"] = []string{}
	if err := enforcer.CheckProjectLimit(context.Background(), "org_empty"); err != nil {
		t.Fatalf("should pass with 0 projects: %v", err)
	}
}

func TestEnforcer_CheckProjectLimit_PlanLimitLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{
		getOrgSubscriptionFn: func(context.Context, string) (*OrgSubscription, error) {
			return nil, errors.New("subscription store unavailable")
		},
	}, nil, slog.Default())

	err := enforcer.CheckProjectLimit(context.Background(), "org-plan-error")
	if err == nil {
		t.Fatal("expected project limit check to fail closed when plan limits cannot be loaded")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "billing_plan_unavailable" {
		t.Fatalf("Code = %q, want billing_plan_unavailable", le.Code)
	}
}

func TestEnforcer_CheckProjectLimit_CountErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{
		countProjectsErr: errors.New("project count unavailable"),
	}, nil, slog.Default())

	err := enforcer.CheckProjectLimit(context.Background(), "org-count-error")
	if err == nil {
		t.Fatal("expected project limit check to fail closed when project count cannot be loaded")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
	}
}

func TestEnforcer_CheckSpendingLimit_FreeTierZeroSpend_Passes(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.periodSpendByOrg = map[string]int64{
		"org_free": 0,
	}

	if err := enforcer.CheckSpendingLimit(context.Background(), "org_free"); err != nil {
		t.Fatalf("free tier with zero spend should pass: %v", err)
	}
}

func TestEnforcer_CheckSpendingLimit_FreeTierSpendReadErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.sumSpendErr = errors.New("spend aggregation unavailable")

	err := enforcer.CheckSpendingLimit(context.Background(), "org_free")
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
	}
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
	if err == nil {
		t.Fatal("expected free-tier spending limit error for any spend")
	}
}

func TestEnforcer_CheckSpendingLimit_FreeTierOverBudget_Blocks(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.periodSpendByOrg = map[string]int64{
		"org_free": 1_250_000,
	}

	err := enforcer.CheckSpendingLimit(context.Background(), "org_free")
	if err == nil {
		t.Fatal("expected free-tier spending limit error")
	}

	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "spending_limit_reached" {
		t.Fatalf("Code = %q, want spending_limit_reached", le.Code)
	}
	if le.Limit != 0 {
		t.Fatalf("Limit = %d, want 0 (no included credit in orchestration-only mode)", le.Limit)
	}
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
	if err == nil {
		t.Fatal("expected free-tier spending limit error")
	}
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

	if err := enforcer.CheckSpendingLimit(context.Background(), "org_starter"); err != nil {
		t.Fatalf("zero spend with $0 cap should pass: %v", err)
	}

	store.periodSpendByOrg["org_starter"] = 1
	err := enforcer.CheckSpendingLimit(context.Background(), "org_starter")
	if err == nil {
		t.Fatal("expected spending limit error: any spend exceeds $0 cap")
	}
}

func TestEnforcer_GetOrgPlanLimits_Cache(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org_cached": {OrgID: "org_cached", PlanTier: "pro", Status: "active"},
	}

	ctx := context.Background()
	limits1, err := enforcer.GetOrgPlanLimits(ctx, "org_cached")
	if err != nil {
		t.Fatal(err)
	}
	if limits1.PlanTier != domain.PlanPro {
		t.Errorf("expected pro, got %q", limits1.PlanTier)
	}

	// Change plan in store, cache should still return pro
	store.subscriptions["org_cached"].PlanTier = "free"
	limits2, err := enforcer.GetOrgPlanLimits(ctx, "org_cached")
	if err != nil {
		t.Fatal(err)
	}
	if limits2.PlanTier != domain.PlanPro {
		t.Errorf("expected cached pro, got %q", limits2.PlanTier)
	}

	// Invalidate cache
	enforcer.InvalidateOrgCache("org_cached")
	limits3, err := enforcer.GetOrgPlanLimits(ctx, "org_cached")
	if err != nil {
		t.Fatal(err)
	}
	if limits3.PlanTier != domain.PlanFree {
		t.Errorf("expected free after invalidation, got %q", limits3.PlanTier)
	}
}

func TestReconcileConcurrentRunCount(t *testing.T) {
	t.Parallel()
	enforcer, _, mr := setupEnforcer(t)

	ctx := context.Background()

	// Manually set Redis counter to 10
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rdb.Set(ctx, "strait:org_concurrent:org_recon", 10, 0)

	// Reconcile with actual count of 3
	if err := enforcer.ReconcileConcurrentRunCount(ctx, "org_recon", 3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, err := rdb.Get(ctx, "strait:org_concurrent:org_recon").Int64()
	if err != nil {
		t.Fatalf("failed to get counter: %v", err)
	}
	if val != 3 {
		t.Errorf("counter = %d, want 3", val)
	}
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
		if err := enforcer.CheckConcurrentRunLimit(ctx, "org_crash"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Reconcile: actual executing count is 2 (3 crashed).
	if err := enforcer.ReconcileConcurrentRunCount(ctx, "org_crash", 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify counter is now 2.
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	val, err := rdb.Get(ctx, "strait:org_concurrent:org_crash").Int64()
	if err != nil {
		t.Fatalf("failed to get counter: %v", err)
	}
	if val != 2 {
		t.Errorf("counter = %d after reconciliation, want 2", val)
	}
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
	if concurrentCounterTTL != 24*time.Hour {
		t.Errorf("concurrentCounterTTL = %v, want 24h", concurrentCounterTTL)
	}
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

	if err := enforcer.ReconcileAllConcurrentCounts(ctx, counter); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	val, err := rdb.Get(ctx, "strait:org_concurrent:org-X").Int64()
	if err != nil {
		t.Fatalf("key should exist: %v", err)
	}
	if val != 3 {
		t.Errorf("counter = %d, want 3", val)
	}
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

	if err := enforcer.ReconcileAllConcurrentCounts(ctx, counter); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, err := rdb.Get(ctx, "strait:org_concurrent:org-Y").Int64()
	if err != nil {
		t.Fatalf("key should exist: %v", err)
	}
	if val != 0 {
		t.Errorf("counter = %d, want 0", val)
	}
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

	if err := enforcer.ReconcileAllConcurrentCounts(ctx, counter); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for org, want := range map[string]int64{"org-A": 3, "org-B": 1, "org-C": 0} {
		val, err := rdb.Get(ctx, "strait:org_concurrent:"+org).Int64()
		if err != nil {
			t.Fatalf("key for %s should exist: %v", org, err)
		}
		if val != want {
			t.Errorf("%s counter = %d, want %d", org, val, want)
		}
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
	if err == nil {
		t.Fatal("expected error from bulk count, got nil")
	}
}

func TestReconcileAll_NilRedis(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, nil, slog.Default())

	counter := &mockExecutingRunCounter{}
	if err := enforcer.ReconcileAllConcurrentCounts(context.Background(), counter); err != nil {
		t.Fatalf("expected nil error for nil Redis, got %v", err)
	}
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
	if err != nil {
		t.Fatalf("first reservation should pass: %v", err)
	}
	t.Cleanup(release)

	_, err = enforcerB.ReserveWorkerConnection(ctx, "org_workers", "replica-b-worker", time.Minute)
	if err == nil {
		t.Fatal("expected second cross-replica worker reservation to hit free cap")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T: %v", err, err)
	}
	if le.Code != "worker_connections_reached" {
		t.Fatalf("Code = %q, want worker_connections_reached", le.Code)
	}

	release()
	if _, err := enforcerB.ReserveWorkerConnection(ctx, "org_workers", "replica-b-worker", time.Minute); err != nil {
		t.Fatalf("reservation after release should pass: %v", err)
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
	if err == nil {
		t.Fatal("expected worker reservation to fail closed when plan limits cannot be loaded")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "billing_plan_unavailable" {
		t.Fatalf("Code = %q, want billing_plan_unavailable", le.Code)
	}
}

func TestReserveWorkerConnection_RedisErrorFailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	if err := rdb.Close(); err != nil {
		t.Fatalf("close redis client: %v", err)
	}
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org_workers": {OrgID: "org_workers", PlanTier: string(domain.PlanFree), Status: "active"},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	_, err := enforcer.ReserveWorkerConnection(context.Background(), "org_workers", "worker-1", time.Minute)
	if err == nil {
		t.Fatal("expected worker reservation to fail closed when Redis is unavailable")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
	}
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
	if err == nil {
		t.Fatal("expected worker reservation to fail closed when Redis is not configured")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
	}
}

func TestCheckWorkerConnectionLimit_PlanLimitLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()

	enforcer := NewEnforcer(&mockBillingStore{
		getOrgSubscriptionFn: func(context.Context, string) (*OrgSubscription, error) {
			return nil, errors.New("subscription store unavailable")
		},
	}, nil, slog.Default())

	err := enforcer.CheckWorkerConnectionLimit(context.Background(), "org-plan-error", 0)
	if err == nil {
		t.Fatal("expected worker connection check to fail closed when plan limits cannot be loaded")
	}
	if !strings.Contains(err.Error(), "resolve worker connection plan limit") {
		t.Fatalf("error = %v, want worker connection plan-limit context", err)
	}
}

// Member limit tests.

func TestCheckMemberLimit_FreeUnderLimit_Passes(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	freeLimits := GetPlanLimits(domain.PlanFree)
	store.memberCounts = map[string]int{"org_free": freeLimits.MaxMembersPerOrg - 1}

	if err := enforcer.CheckMemberLimit(context.Background(), "org_free"); err != nil {
		t.Fatalf("expected pass under limit: %v", err)
	}
}

func TestCheckMemberLimit_FreeAtLimit_Blocked(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	freeLimits := GetPlanLimits(domain.PlanFree)
	store.memberCounts = map[string]int{"org_free": freeLimits.MaxMembersPerOrg}

	err := enforcer.CheckMemberLimit(context.Background(), "org_free")
	if err == nil {
		t.Fatalf("expected member limit error at %d members on free plan", freeLimits.MaxMembersPerOrg)
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T", err)
	}
	if le.Code != "member_limit_reached" {
		t.Errorf("code = %q, want member_limit_reached", le.Code)
	}
	if le.Limit != int64(freeLimits.MaxMembersPerOrg) {
		t.Errorf("limit = %d, want %d", le.Limit, freeLimits.MaxMembersPerOrg)
	}
}

func TestCheckMemberLimit_PlanLimitLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{
		getOrgSubscriptionFn: func(context.Context, string) (*OrgSubscription, error) {
			return nil, errors.New("subscription store unavailable")
		},
	}, nil, slog.Default())

	err := enforcer.CheckMemberLimit(context.Background(), "org-plan-error")
	if err == nil {
		t.Fatal("expected member limit check to fail closed when plan limits cannot be loaded")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "billing_plan_unavailable" {
		t.Fatalf("Code = %q, want billing_plan_unavailable", le.Code)
	}
}

func TestCheckMemberLimit_CountErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{
		countMembersErr: errors.New("member count unavailable"),
	}, nil, slog.Default())

	err := enforcer.CheckMemberLimit(context.Background(), "org-count-error")
	if err == nil {
		t.Fatal("expected member limit check to fail closed when member count cannot be loaded")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
	}
}

func TestCheckMemberLimit_StarterUnderLimit_Passes(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	starterLimits := GetPlanLimits(domain.PlanStarter)
	store.subscriptions = map[string]*OrgSubscription{
		"org_starter": {OrgID: "org_starter", PlanTier: "starter", Status: "active"},
	}
	store.memberCounts = map[string]int{"org_starter": starterLimits.MaxMembersPerOrg - 1}

	if err := enforcer.CheckMemberLimit(context.Background(), "org_starter"); err != nil {
		t.Fatalf("expected pass under limit: %v", err)
	}
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
	if err == nil {
		t.Fatalf("expected member limit error at %d members on starter plan", starterLimits.MaxMembersPerOrg)
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T", err)
	}
	if le.Code != "member_limit_reached" {
		t.Errorf("code = %q, want member_limit_reached", le.Code)
	}
	if le.Limit != int64(starterLimits.MaxMembersPerOrg) {
		t.Errorf("limit = %d, want %d", le.Limit, starterLimits.MaxMembersPerOrg)
	}
}

func TestCheckMemberLimit_ProUnlimited_AlwaysPasses(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.subscriptions = map[string]*OrgSubscription{
		"org_ent": {OrgID: "org_ent", PlanTier: "enterprise", Status: "active"},
	}
	store.memberCounts = map[string]int{"org_ent": 1000}

	if err := enforcer.CheckMemberLimit(context.Background(), "org_ent"); err != nil {
		t.Fatalf("enterprise should be unlimited: %v", err)
	}
}

// Org creation limit tests.

func TestCheckOrgCreationLimit_FreeAt1_Passes(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.orgCountsByUser = map[string]int{"user1": 0}

	if err := enforcer.CheckOrgCreationLimit(context.Background(), "user1", domain.PlanFree); err != nil {
		t.Fatalf("expected pass with 0 orgs on free (limit 1): %v", err)
	}
}

func TestCheckOrgCreationLimit_FreeAt1_Blocked(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.orgCountsByUser = map[string]int{"user1": 1}

	err := enforcer.CheckOrgCreationLimit(context.Background(), "user1", domain.PlanFree)
	if err == nil {
		t.Fatal("expected org limit error at 1 org on free plan")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T", err)
	}
	if le.Code != "org_limit_reached" {
		t.Errorf("code = %q, want org_limit_reached", le.Code)
	}
	if le.Limit != 1 {
		t.Errorf("limit = %d, want 1", le.Limit)
	}
}

func TestCheckOrgCreationLimit_CountErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{
		countOrgsByUserErr: errors.New("org count unavailable"),
	}, nil, slog.Default())

	err := enforcer.CheckOrgCreationLimit(context.Background(), "user-count-error", domain.PlanFree)
	if err == nil {
		t.Fatal("expected org creation limit check to fail closed when org count cannot be loaded")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
	}
}

func TestCheckOrgCreationLimit_StarterAt2_Passes(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.orgCountsByUser = map[string]int{"user2": 1}

	if err := enforcer.CheckOrgCreationLimit(context.Background(), "user2", domain.PlanStarter); err != nil {
		t.Fatalf("expected pass with 1 org on starter (limit 2): %v", err)
	}
}

func TestCheckOrgCreationLimit_ProUnlimited(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.orgCountsByUser = map[string]int{"user3": 100}

	if err := enforcer.CheckOrgCreationLimit(context.Background(), "user3", domain.PlanEnterprise); err != nil {
		t.Fatalf("enterprise should be unlimited: %v", err)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warned {
		t.Error("expected false for unlimited daily runs (free tier)")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warned {
		t.Error("expected false for unlimited plan")
	}
}

func TestCheck80PercentWarning_ZeroUsage_False(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	warned, err := enforcer.Check80PercentDailyRunWarning(context.Background(), "org_zero")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warned {
		t.Error("expected false at 0 usage")
	}
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

	// During active grace, daily run limit should be skipped (allowed).
	if err := enforcer.CheckDailyRunLimit(context.Background(), "org_grace"); err != nil {
		t.Fatalf("expected run to be allowed during grace period: %v", err)
	}
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
	if err == nil {
		t.Fatal("expected rejection after grace period expired")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T", err)
	}
	if le.Code != "grace_period_expired" {
		t.Errorf("code = %q, want grace_period_expired", le.Code)
	}
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

	// Normal limit checking: should succeed without grace period interference.
	if err := enforcer.CheckDailyRunLimit(context.Background(), "org_ok"); err != nil {
		t.Fatalf("expected normal limit check to pass: %v", err)
	}
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
	if err == nil {
		t.Fatal("expected rejection for restricted payment status")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T", err)
	}
	if le.Code != "payment_restricted" {
		t.Errorf("code = %q, want payment_restricted", le.Code)
	}
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
	if err == nil {
		t.Fatal("expected rejection for suspended payment status")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T", err)
	}
	if le.Code != "payment_suspended" {
		t.Errorf("code = %q, want payment_suspended", le.Code)
	}
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

	// During active grace, concurrent limit should also be skipped.
	if err := enforcer.CheckConcurrentRunLimit(context.Background(), "org_conc_grace"); err != nil {
		t.Fatalf("expected concurrent run to be allowed during grace period: %v", err)
	}

	// Also verify restricted concurrent limit is rejected.
	store.subscriptions["org_conc_restricted"] = &OrgSubscription{
		OrgID:         "org_conc_restricted",
		PlanTier:      "starter",
		Status:        "active",
		PaymentStatus: "restricted",
	}
	err := enforcer.CheckConcurrentRunLimit(context.Background(), "org_conc_restricted")
	if err == nil {
		t.Fatal("expected concurrent limit rejection for restricted status")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T", err)
	}
	if le.Code != "payment_restricted" {
		t.Errorf("code = %q, want payment_restricted", le.Code)
	}
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
	if err2 != nil {
		t.Fatalf("first call: %v", err2)
	}
	firstDBCalls := dbCalls

	// Second call: cache hit, no additional DB call.
	_, err2 = enforcer.GetOrgPlanLimits(ctx, "org-cache-test")
	if err2 != nil {
		t.Fatalf("second call: %v", err2)
	}
	if dbCalls != firstDBCalls {
		t.Fatalf("DB calls = %d after second call, want %d (cache hit)", dbCalls, firstDBCalls)
	}
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
	if limits1.PlanTier != domain.PlanPro {
		t.Fatalf("expected pro, got %q", limits1.PlanTier)
	}

	// Change underlying data.
	store.subscriptions["org-inv"].PlanTier = "starter"

	// Without invalidation, should still return cached pro.
	limits2, _ := enforcer.GetOrgPlanLimits(ctx, "org-inv")
	if limits2.PlanTier != domain.PlanPro {
		t.Fatalf("expected cached pro, got %q", limits2.PlanTier)
	}

	// After invalidation, should reflect new plan.
	enforcer.InvalidateOrgCache("org-inv")
	limits3, _ := enforcer.GetOrgPlanLimits(ctx, "org-inv")
	if limits3.PlanTier != domain.PlanStarter {
		t.Fatalf("expected starter after invalidation, got %q", limits3.PlanTier)
	}
}

func TestOrgCache_EmptyOrgIDReturnsFree(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	limits, err2 := enforcer.GetOrgPlanLimits(context.Background(), "")
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if limits.PlanTier != domain.PlanFree {
		t.Fatalf("expected free for empty org ID, got %q", limits.PlanTier)
	}
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
	if limits.PlanTier != domain.PlanFree {
		t.Fatalf("expected free, got %q", limits.PlanTier)
	}
	if dbCalls != 1 {
		t.Fatalf("DB calls = %d, want 1", dbCalls)
	}

	// Second call: cache hit, no DB call.
	_, _ = enforcer.GetOrgPlanLimits(ctx, "org-nosub")
	if dbCalls != 1 {
		t.Fatalf("DB calls = %d, want 1 (cached free)", dbCalls)
	}
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
	if mode != "warn" {
		t.Fatalf("enforcement mode = %q, want warn", mode)
	}
}

func TestOrgCache_EnforcementModeFallsBackToEnforce(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	// No cache entry for this org.
	mode := enforcer.getEnforcementMode(t.Context(), "org-uncached")
	if mode != "enforce" {
		t.Fatalf("enforcement mode = %q, want enforce (default)", mode)
	}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "cust_abc123" {
			t.Errorf("got %q, want %q", got, "cust_abc123")
		}
	})

	t.Run("returns_empty_when_nil", func(t *testing.T) {
		t.Parallel()
		enforcer, store, _ := setupEnforcer(t)
		store.subscriptions = map[string]*OrgSubscription{
			"org-nil": {OrgID: "org-nil", PlanTier: "starter", Status: "active", StripeCustomerID: nil},
		}

		got, err := enforcer.GetStripeCustomerID(context.Background(), "org-nil")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	t.Run("returns_empty_when_empty_string", func(t *testing.T) {
		t.Parallel()
		enforcer, store, _ := setupEnforcer(t)
		empty := ""
		store.subscriptions = map[string]*OrgSubscription{
			"org-empty": {OrgID: "org-empty", PlanTier: "starter", Status: "active", StripeCustomerID: &empty},
		}

		got, err := enforcer.GetStripeCustomerID(context.Background(), "org-empty")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	t.Run("returns_error_when_not_found", func(t *testing.T) {
		t.Parallel()
		enforcer, _, _ := setupEnforcer(t)

		_, err := enforcer.GetStripeCustomerID(context.Background(), "org-nonexistent")
		if err == nil {
			t.Fatal("expected error for missing subscription")
		}
		if !errors.Is(err, ErrSubscriptionNotFound) {
			t.Errorf("expected ErrSubscriptionNotFound, got %v", err)
		}
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
		if err := enforcer.CheckDailyRunLimit(context.Background(), "org_pro"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Pro plans allow overage -- no error expected past the daily limit.
	err := enforcer.CheckDailyRunLimit(context.Background(), "org_pro")
	if err != nil {
		t.Fatalf("expected overage to be allowed for pro plan, got: %v", err)
	}
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
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if limits.AllowsHTTPMode != tt.want {
				t.Errorf("AllowsHTTPMode = %v, want %v for %s plan", limits.AllowsHTTPMode, tt.want, tt.name)
			}
		})
	}
}

func TestEnforcer_GetOrgPlanLimits_NoSubscription_DefaultsFree(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	// No subscription = defaults to free plan.
	limits, err := enforcer.GetOrgPlanLimits(context.Background(), "org_unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !limits.AllowsHTTPMode {
		t.Error("AllowsHTTPMode should be true for org with no subscription (free tier allows HTTP mode)")
	}
	if limits.PlanTier != domain.PlanFree {
		t.Errorf("PlanTier = %q, want %q", limits.PlanTier, domain.PlanFree)
	}
}

func TestEnforcer_GetDailyRunCount_EmptyOrgID(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)
	count, err := enforcer.GetDailyRunCount(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 for empty orgID", count)
	}
}

func TestEnforcer_GetDailyRunCount_NoKey(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)
	count, err := enforcer.GetDailyRunCount(context.Background(), "org-missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 for non-existent key", count)
	}
}

func TestEnforcer_GetDailyRunCount_WithRuns(t *testing.T) {
	t.Parallel()
	enforcer, _, mr := setupEnforcer(t)

	key := "strait:org_runs:org-counted:" + time.Now().UTC().Format("2006-01-02")
	if err := mr.Set(key, "5"); err != nil {
		t.Fatalf("seed daily run count: %v", err)
	}

	count, err := enforcer.GetDailyRunCount(context.Background(), "org-counted")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	baseLimits := GetPlanLimits(domain.PlanPro)
	wantConcurrent := baseLimits.MaxConcurrentRuns + 200 // 2 packs x 100
	if limits.MaxConcurrentRuns != wantConcurrent {
		t.Errorf("MaxConcurrentRuns = %d, want %d (base %d + 200)", limits.MaxConcurrentRuns, wantConcurrent, baseLimits.MaxConcurrentRuns)
	}
}

func TestEnforcer_GetOrgPlanLimits_AddonLoadErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org-addon-error": {OrgID: "org-addon-error", PlanTier: "pro", Status: "active"},
	}
	store.listActiveAddonsErr = errors.New("addon store unavailable")

	_, err := enforcer.GetOrgPlanLimits(context.Background(), "org-addon-error")
	if err == nil {
		t.Fatal("expected add-on lookup error")
	}
	if !strings.Contains(err.Error(), "listing active add-ons") {
		t.Fatalf("error = %v, want active add-on lookup context", err)
	}
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
	if err != nil {
		t.Fatalf("enterprise should have unlimited concurrent runs: %v", err)
	}
}

func TestEnforcer_NewEnforcer_CacheTTL(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)
	if enforcer.cacheTTL != 5*time.Minute {
		t.Errorf("cacheTTL = %v, want 5m", enforcer.cacheTTL)
	}
}

func TestEnforcer_Check80PercentDailyRunWarning_EmptyOrgID(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)
	warned, err := enforcer.Check80PercentDailyRunWarning(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warned {
		t.Error("expected false for empty orgID")
	}
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
	if err == nil {
		t.Fatal("expected error when DB is unavailable, got nil (fail-open antipattern)")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "dispatch_priority_exceeded" {
		t.Fatalf("expected dispatch_priority_exceeded, got %q", le.Code)
	}
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
	if err == nil {
		t.Fatal("expected error when plan limits cannot be loaded, got nil")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
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
	if limits.MaxDispatchPriority < 5 {
		t.Fatalf("free plan MaxDispatchPriority=%d, want at least 5", limits.MaxDispatchPriority)
	}

	if err := enforcer.CheckMaxDispatchPriority(context.Background(), "proj-free", 5); err != nil {
		t.Fatalf("expected nil for priority within cap, got %v", err)
	}
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
	if err == nil {
		t.Fatal("expected LimitError for priority exceeding platform cap")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "dispatch_priority_exceeded" {
		t.Fatalf("expected dispatch_priority_exceeded, got %q", le.Code)
	}
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

	if err := enforcer.CheckMaxDispatchPriority(context.Background(), "proj-1", 0); err != nil {
		t.Fatalf("priority 0 should always be allowed, got %v", err)
	}
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

	// CheckMonthlyRunLimit increments the counter atomically on each allowed call.
	if err := enforcer.CheckMonthlyRunLimit(ctx, "org-monthly-1"); err != nil {
		t.Fatalf("CheckMonthlyRunLimit: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	key := monthlyRunKey("org-monthly-1", time.Now())
	before, _ := rdb.Get(ctx, key).Int64()
	if before != 1 {
		t.Fatalf("expected counter=1 after one incr, got %d", before)
	}

	// Rollback (abort path).
	enforcer.DecrMonthlyRunCount(ctx, "org-monthly-1")

	after, _ := rdb.Get(ctx, key).Int64()
	if after != 0 {
		t.Errorf("expected counter=0 after decr, got %d", after)
	}
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
	if err == nil {
		t.Fatal("expected monthly run check to fail closed when payment status cannot be loaded")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
	}
}

func TestCheckMonthlyRunLimit_RedisErrorFailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	if err := rdb.Close(); err != nil {
		t.Fatalf("close redis client: %v", err)
	}
	orgID := "org-monthly-redis-error"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			orgID: {OrgID: orgID, PlanTier: "starter", Status: "active"},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	err := enforcer.CheckMonthlyRunLimit(context.Background(), orgID)
	if err == nil {
		t.Fatal("expected monthly run check to fail closed when Redis is unavailable")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
	}
}

func TestCheckMonthlyRunLimit_RequiredNilRedisFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{}, nil, slog.Default(), WithRequireRedis())

	err := enforcer.CheckMonthlyRunLimit(context.Background(), "org-monthly-nil-redis")
	if err == nil {
		t.Fatal("expected monthly run check to fail closed when required Redis is not configured")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
	}
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
	if err := rdb.Set(ctx, monthlyRunKey(orgID, time.Now()), int64(limits.MaxRunsPerMonth), 0).Err(); err != nil {
		t.Fatalf("seed monthly counter: %v", err)
	}

	err := enforcer.CheckMonthlyRunLimitForRun(ctx, orgID, "run-paid-disabled")
	if err == nil {
		t.Fatal("expected paid plan with overage disabled to hard-cap at monthly allowance")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("error = %T %v, want *LimitError", err, err)
	}
	if le.Code != "plan_cap_reached" {
		t.Fatalf("limit code = %q, want plan_cap_reached", le.Code)
	}
	if store.pausedOrgID != orgID || store.pausedReason != "quota_exceeded" {
		t.Fatalf("quota pause = org %q reason %q, want %q quota_exceeded", store.pausedOrgID, store.pausedReason, orgID)
	}
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
	if err := rdb.Set(ctx, monthlyRunKey(orgID, time.Now()), int64(limits.MaxRunsPerMonth), 0).Err(); err != nil {
		t.Fatalf("seed monthly counter: %v", err)
	}

	if err := enforcer.CheckMonthlyRunLimitForRun(ctx, orgID, runID); err != nil {
		t.Fatalf("paid plan with overage enabled should allow over allowance: %v", err)
	}
	if got := enforcer.IsRunOverage(ctx, runID); !got {
		t.Fatal("over-allowance run should be marked for Stripe overage metering")
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
	if err := baseRedis.Set(ctx, monthlyRunKey(orgID, time.Now()), int64(limits.MaxRunsPerMonth), 0).Err(); err != nil {
		t.Fatalf("seed monthly counter: %v", err)
	}
	enforcer := NewEnforcer(store, overageMarkerFailRedis{Cmdable: baseRedis}, slog.Default())

	err := enforcer.CheckMonthlyRunLimitForRun(ctx, orgID, runID)
	if err == nil {
		t.Fatal("expected paid overage run to fail closed when overage marker cannot be persisted")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
	}
	if got := enforcer.IsRunOverage(ctx, runID); got {
		t.Fatal("failed marker write must not leave a run marked as overage")
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
	if err := rdb.Set(ctx, monthlyRunKey(orgID, time.Now()), int64(limits.MaxRunsPerMonth), 0).Err(); err != nil {
		t.Fatalf("seed monthly counter: %v", err)
	}

	if err := enforcer.CheckMonthlyRunLimitForRun(ctx, orgID, "run-free-card"); err != nil {
		t.Fatalf("free plan with card-backed overage enabled should allow over allowance: %v", err)
	}
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
	if err == nil && val < 0 {
		t.Errorf("counter went negative: %d", val)
	}
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
	if err == nil && val < 0 {
		t.Errorf("counter went negative after parallel ops: %d", val)
	}
}
