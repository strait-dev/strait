package billing

import (
	"context"
	"errors"
	"log/slog"
	"testing"

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

func isLimitError(err error, target **LimitError) bool {
	var le *LimitError
	if errors.As(err, &le) {
		*target = le
		return true
	}
	return false
}
