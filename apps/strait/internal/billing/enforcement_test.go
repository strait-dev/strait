package billing

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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

	// Free plan: 5000 runs/day, no subscription = free
	for range 5000 {
		if err := enforcer.CheckDailyRunLimit(context.Background(), "org_free"); err != nil {
			t.Fatalf("unexpected limit error at run: %v", err)
		}
	}

	// Run 5001 should fail
	err := enforcer.CheckDailyRunLimit(context.Background(), "org_free")
	if err == nil {
		t.Fatal("expected limit error at 5001 runs")
	}

	var le *LimitError
	if ok := isLimitError(err, &le); !ok {
		t.Fatalf("expected LimitError, got %T", err)
	}
	if le.Code != "org_daily_run_limit_exceeded" {
		t.Errorf("code = %q, want org_daily_run_limit_exceeded", le.Code)
	}
}

func TestEnforcer_CheckDailyRunLimit_Starter(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org_starter": {OrgID: "org_starter", PlanTier: "starter", Status: "active"},
	}

	limits := GetPlanLimits(domain.PlanStarter)
	for range limits.MaxRunsPerDay {
		if err := enforcer.CheckDailyRunLimit(context.Background(), "org_starter"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	err := enforcer.CheckDailyRunLimit(context.Background(), "org_starter")
	if err == nil {
		t.Fatal("expected limit error")
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

	// Use up all runs
	for range 5000 {
		_ = enforcer.CheckDailyRunLimit(ctx, "org_rollback")
	}

	// Decrement (simulating a failed run)
	enforcer.DecrDailyRunCount(ctx, "org_rollback")

	// Should now allow one more
	if err := enforcer.CheckDailyRunLimit(ctx, "org_rollback"); err != nil {
		t.Fatalf("should allow after decrement: %v", err)
	}
}

func TestEnforcer_CheckConcurrentRunLimit(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	ctx := context.Background()
	// Free plan: 5 concurrent runs max.
	for range 5 {
		if err := enforcer.CheckConcurrentRunLimit(ctx, "org_conc"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Run 6 should fail.
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

func TestEnforcer_CheckProjectLimit(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.projects = map[string][]string{
		"org_full": {"p1", "p2"},
	}

	// Free: 2 projects max
	err := enforcer.CheckProjectLimit(context.Background(), "org_full")
	if err == nil {
		t.Fatal("expected project limit error")
	}

	store.projects["org_one"] = []string{"p1"}
	if err := enforcer.CheckProjectLimit(context.Background(), "org_one"); err != nil {
		t.Fatalf("should pass with 1 project: %v", err)
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
	enforcer, _, mr := setupEnforcer(t)

	ctx := context.Background()

	// Simulate 5 runs started (increment without decrement = crash scenario)
	for range 5 {
		if err := enforcer.CheckConcurrentRunLimit(ctx, "org_crash"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Reconcile: actual executing count is 2 (3 crashed)
	if err := enforcer.ReconcileConcurrentRunCount(ctx, "org_crash", 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify counter is now 2
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

func TestReconcileAll_ContinuesOnSingleOrgError(t *testing.T) {
	t.Parallel()
	enforcer, _, mr := setupEnforcer(t)
	ctx := context.Background()

	counter := &mockExecutingRunCounter{
		orgCounts: map[string]int{"org-Y": 2},
		listOrgs:  []string{"org-X", "org-Y"},
		countErr:  map[string]error{"org-X": errors.New("db error")},
	}

	if err := enforcer.ReconcileAllConcurrentCounts(ctx, counter); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// org-Y should still be reconciled.
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	val, err := rdb.Get(ctx, "strait:org_concurrent:org-Y").Int64()
	if err != nil {
		t.Fatalf("key for org-Y should exist: %v", err)
	}
	if val != 2 {
		t.Errorf("org-Y counter = %d, want 2", val)
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
