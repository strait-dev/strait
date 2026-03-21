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

// Member limit tests.

func TestCheckMemberLimit_FreeAt3_Passes(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.memberCounts = map[string]int{"org_free": 2}

	if err := enforcer.CheckMemberLimit(context.Background(), "org_free"); err != nil {
		t.Fatalf("expected pass with 2 members on free (limit 3): %v", err)
	}
}

func TestCheckMemberLimit_FreeAt3_Blocked(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.memberCounts = map[string]int{"org_free": 3}

	err := enforcer.CheckMemberLimit(context.Background(), "org_free")
	if err == nil {
		t.Fatal("expected member limit error at 3 members on free plan")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T", err)
	}
	if le.Code != "member_limit_reached" {
		t.Errorf("code = %q, want member_limit_reached", le.Code)
	}
	if le.Limit != 3 {
		t.Errorf("limit = %d, want 3", le.Limit)
	}
}

func TestCheckMemberLimit_StarterAt10_Passes(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.subscriptions = map[string]*OrgSubscription{
		"org_starter": {OrgID: "org_starter", PlanTier: "starter", Status: "active"},
	}
	store.memberCounts = map[string]int{"org_starter": 9}

	if err := enforcer.CheckMemberLimit(context.Background(), "org_starter"); err != nil {
		t.Fatalf("expected pass with 9 members on starter (limit 10): %v", err)
	}
}

func TestCheckMemberLimit_StarterAt10_Blocked(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)
	store.subscriptions = map[string]*OrgSubscription{
		"org_starter": {OrgID: "org_starter", PlanTier: "starter", Status: "active"},
	}
	store.memberCounts = map[string]int{"org_starter": 10}

	err := enforcer.CheckMemberLimit(context.Background(), "org_starter")
	if err == nil {
		t.Fatal("expected member limit error at 10 members on starter plan")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T", err)
	}
	if le.Code != "member_limit_reached" {
		t.Errorf("code = %q, want member_limit_reached", le.Code)
	}
	if le.Limit != 10 {
		t.Errorf("limit = %d, want 10", le.Limit)
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

func TestCheck80PercentWarning_Below80_False(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	// Free plan: 5000 runs/day, 80% = 4000. Set count to 3999.
	ctx := context.Background()
	for range 3999 {
		_ = enforcer.CheckDailyRunLimit(ctx, "org_warn_below")
	}

	warned, err := enforcer.Check80PercentDailyRunWarning(ctx, "org_warn_below")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warned {
		t.Error("expected false at 3999/5000 (79.98%)")
	}
}

func TestCheck80PercentWarning_At80_True(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	ctx := context.Background()
	for range 4000 {
		_ = enforcer.CheckDailyRunLimit(ctx, "org_warn_at80")
	}

	warned, err := enforcer.Check80PercentDailyRunWarning(ctx, "org_warn_at80")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !warned {
		t.Error("expected true at 4000/5000 (80%)")
	}
}

func TestCheck80PercentWarning_Above80_True(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	ctx := context.Background()
	for range 4500 {
		_ = enforcer.CheckDailyRunLimit(ctx, "org_warn_above")
	}

	warned, err := enforcer.Check80PercentDailyRunWarning(ctx, "org_warn_above")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !warned {
		t.Error("expected true at 4500/5000 (90%)")
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
