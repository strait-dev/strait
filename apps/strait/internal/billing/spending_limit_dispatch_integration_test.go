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

// TestSpendingLimitIntegration_AtCapRejects seeds a real Pro-tier org
// with usage_records summing to the configured cap and asserts the
// enforcer returns *LimitError for that org. This is the bedrock
// integration check behind Phase 4.1: the dispatch wiring relies on
// CheckSpendingLimit reading real Postgres state and producing a typed
// rejection that worker code can pattern-match.
func TestSpendingLimitIntegration_AtCapRejects(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	orgID := "org-spend-cap-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("plan: %v", err)
	}
	const cap = int64(2_500_000) // $2.50
	if err := pgStore.UpdateSpendingLimit(ctx, orgID, cap, "block"); err != nil {
		t.Fatalf("set spending limit: %v", err)
	}

	// Seed usage records summing to exactly the cap. Two days of $1.25
	// ensures the SUM lands at the cap and the period window catches
	// both rows (we use today and yesterday).
	now := time.Now().UTC()
	for _, ago := range []time.Duration{0, 24 * time.Hour} {
		rec := &billing.UsageRecord{
			ID:               newID(),
			OrgID:            orgID,
			ProjectID:        newID(),
			PeriodDate:       now.Add(-ago),
			RunsCount:        1,
			ComputeCostMicro: cap / 2,
		}
		if err := pgStore.UpsertUsageRecord(ctx, rec); err != nil {
			t.Fatalf("seed usage_records: %v", err)
		}
	}

	enforcer := billing.NewEnforcer(pgStore, rdb, slog.Default())

	err := enforcer.CheckSpendingLimit(ctx, orgID)
	if err == nil {
		t.Fatalf("expected spending limit rejection at cap, got nil")
	}
	var lim *billing.LimitError
	if !errors.As(err, &lim) {
		t.Fatalf("expected *billing.LimitError, got %T: %v", err, err)
	}
	if lim.Code != "spending_limit_reached" {
		t.Errorf("LimitError.Code = %q, want spending_limit_reached", lim.Code)
	}
	if lim.Limit != cap {
		t.Errorf("LimitError.Limit = %d, want %d", lim.Limit, cap)
	}
}

// TestSpendingLimitIntegration_BelowCapAllows is the negative half of
// the above: with usage strictly under the cap, the enforcer must
// return nil so the dispatch path proceeds normally.
func TestSpendingLimitIntegration_BelowCapAllows(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	orgID := "org-spend-under-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("plan: %v", err)
	}
	const cap = int64(10_000_000) // $10
	if err := pgStore.UpdateSpendingLimit(ctx, orgID, cap, "block"); err != nil {
		t.Fatalf("set spending limit: %v", err)
	}

	rec := &billing.UsageRecord{
		ID:               newID(),
		OrgID:            orgID,
		ProjectID:        newID(),
		PeriodDate:       time.Now().UTC(),
		RunsCount:        1,
		ComputeCostMicro: cap / 4,
	}
	if err := pgStore.UpsertUsageRecord(ctx, rec); err != nil {
		t.Fatalf("seed usage_records: %v", err)
	}

	enforcer := billing.NewEnforcer(pgStore, rdb, slog.Default())
	if err := enforcer.CheckSpendingLimit(ctx, orgID); err != nil {
		t.Errorf("expected allow under cap, got %v", err)
	}
}
