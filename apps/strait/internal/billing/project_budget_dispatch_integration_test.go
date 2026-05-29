//go:build integration

package billing_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"strait/internal/billing"
	"strait/internal/domain"
)

// TestProjectBudgetIntegration_BlockAtCapRejects seeds a real project
// with monthly_budget_microusd=$2.50, budget_action='block', and
// usage_records summing to the cap. The enforcer must surface a
// *LimitError with code project_budget_reached. This is the bedrock
// integration check for the dispatch wiring's typed budget rejection.
func TestProjectBudgetIntegration_BlockAtCapRejects(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	orgID := "org-pb-block-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("plan: %v", err)
	}

	p := createProject(t, ctx, q, orgID, "PB-Block")
	if err := pgStore.SetProjectOrgID(ctx, p.ID, orgID); err != nil {
		t.Fatalf("set project org: %v", err)
	}

	const budget = int64(2_500_000) // $2.50
	if err := pgStore.SetProjectBudget(ctx, p.ID, budget, "block"); err != nil {
		t.Fatalf("set project budget: %v", err)
	}

	// Two days of $1.25 lands the SUM at the cap, and the period
	// window catches both rows (today and yesterday).
	now := time.Now().UTC()
	for _, ago := range []time.Duration{0, 24 * time.Hour} {
		rec := &billing.UsageRecord{
			ID:               newID(),
			OrgID:            orgID,
			ProjectID:        p.ID,
			PeriodDate:       now.Add(-ago),
			RunsCount:        1,
			ComputeCostMicro: budget / 2,
		}
		if err := pgStore.UpsertUsageRecord(ctx, rec); err != nil {
			t.Fatalf("seed usage_records: %v", err)
		}
	}

	enforcer := billing.NewEnforcer(pgStore, rdb, slog.Default())

	err := enforcer.CheckProjectBudgetLimit(ctx, p.ID)
	if err == nil {
		t.Fatalf("expected project budget rejection at cap, got nil")
	}
	var lim *billing.LimitError
	if !errors.As(err, &lim) {
		t.Fatalf("expected *billing.LimitError, got %T: %v", err, err)
	}
	if lim.Code != "project_budget_reached" {
		t.Errorf("LimitError.Code = %q, want project_budget_reached", lim.Code)
	}
	if lim.Limit != budget {
		t.Errorf("LimitError.Limit = %d, want %d", lim.Limit, budget)
	}
}

// TestProjectBudgetIntegration_NotifyAtCapAllows is the negative half:
// same shape as above, except budget_action='notify'. Even at the cap,
// the dispatch must proceed.
func TestProjectBudgetIntegration_NotifyAtCapAllows(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	orgID := "org-pb-notify-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("plan: %v", err)
	}

	p := createProject(t, ctx, q, orgID, "PB-Notify")
	if err := pgStore.SetProjectOrgID(ctx, p.ID, orgID); err != nil {
		t.Fatalf("set project org: %v", err)
	}

	const budget = int64(2_500_000)
	if err := pgStore.SetProjectBudget(ctx, p.ID, budget, "notify"); err != nil {
		t.Fatalf("set project budget: %v", err)
	}

	rec := &billing.UsageRecord{
		ID:               newID(),
		OrgID:            orgID,
		ProjectID:        p.ID,
		PeriodDate:       time.Now().UTC(),
		RunsCount:        2,
		ComputeCostMicro: budget,
	}
	if err := pgStore.UpsertUsageRecord(ctx, rec); err != nil {
		t.Fatalf("seed usage_records: %v", err)
	}

	enforcer := billing.NewEnforcer(pgStore, rdb, slog.Default())
	if err := enforcer.CheckProjectBudgetLimit(ctx, p.ID); err != nil {
		t.Errorf("notify-at-cap must allow dispatch, got %v", err)
	}
}

// TestProjectBudgetIntegration_NoQuotaRowAllows confirms that a
// project without any project_quotas row falls through cleanly.
func TestProjectBudgetIntegration_NoQuotaRowAllows(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	orgID := "org-pb-noquota-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("plan: %v", err)
	}

	p := createProject(t, ctx, q, orgID, "PB-NoQuota")
	if err := pgStore.SetProjectOrgID(ctx, p.ID, orgID); err != nil {
		t.Fatalf("set project org: %v", err)
	}

	enforcer := billing.NewEnforcer(pgStore, rdb, slog.Default())
	if err := enforcer.CheckProjectBudgetLimit(ctx, p.ID); err != nil {
		t.Errorf("no quota row must fall through; got %v", err)
	}
}
