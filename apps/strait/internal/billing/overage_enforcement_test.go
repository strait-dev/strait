package billing

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc"
)

func setupSpendingEnforcer(t *testing.T, orgID, planTier string, spendingLimit int64, periodSpend int64) (*Enforcer, *mockBillingStore) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			orgID: {
				OrgID:                 orgID,
				PlanTier:              planTier,
				Status:                "active",
				SpendingLimitMicrousd: spendingLimit,
				LimitAction:           "reject",
				CurrentPeriodStart:    &periodStart,
				CurrentPeriodEnd:      &periodEnd,
			},
		},
		periodSpendByOrg: map[string]int64{
			orgID: periodSpend,
		},
	}

	enforcer := NewEnforcer(store, rdb, slog.Default())
	return enforcer, store
}

func TestOverage_FreeTier_HardCap(t *testing.T) {
	t.Parallel()

	// Free tier: $1.00 credit (1,000,000 micro-USD). Should block when exhausted.
	enforcer, _ := setupSpendingEnforcer(t, "org-free", "free", 0, CreditFreeMicrousd+1)

	err := enforcer.CheckSpendingLimit(context.Background(), "org-free")
	if err == nil {
		t.Fatal("expected spending limit error for free tier over credit")
	}

	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "spending_limit_reached" {
		t.Errorf("Code = %q, want spending_limit_reached", le.Code)
	}
}

func TestOverage_FreeTier_UnderCredit_Passes(t *testing.T) {
	t.Parallel()

	enforcer, _ := setupSpendingEnforcer(t, "org-free", "free", 0, CreditFreeMicrousd-1)

	err := enforcer.CheckSpendingLimit(context.Background(), "org-free")
	if err != nil {
		t.Fatalf("expected pass under free credit: %v", err)
	}
}

func TestOverage_PaidTier_NoSpendingLimit_Allows(t *testing.T) {
	t.Parallel()

	// Pro with no spending limit (-1): overage is allowed indefinitely.
	// Spend $200 on a $49.99 plan -- no block.
	enforcer, _ := setupSpendingEnforcer(t, "org-pro", "pro", -1, 200_000_000)

	err := enforcer.CheckSpendingLimit(context.Background(), "org-pro")
	if err != nil {
		t.Fatalf("expected no block with unlimited spending: %v", err)
	}
}

func TestOverage_PaidTier_WithSpendingLimit_Blocks(t *testing.T) {
	t.Parallel()

	// Pro with $50 spending limit. Credit is $49.99, so total allowed = $99.99.
	// Spend = $100 -> overage = $50.01 > $50 limit -> block.
	enforcer, _ := setupSpendingEnforcer(t, "org-pro", "pro", 50_000_000, 100_000_000)

	err := enforcer.CheckSpendingLimit(context.Background(), "org-pro")
	if err == nil {
		t.Fatal("expected spending limit block at $50 overage cap")
	}
}

func TestOverage_PaidTier_ZeroSpendingLimit_Blocks(t *testing.T) {
	t.Parallel()

	// Pro with $0 spending limit -> hard cap at included credit.
	enforcer, _ := setupSpendingEnforcer(t, "org-pro", "pro", 0, CreditProMicrousd+1)

	err := enforcer.CheckSpendingLimit(context.Background(), "org-pro")
	if err == nil {
		t.Fatal("expected spending limit block with $0 cap")
	}
}

func TestOverage_ScaleTier_NoSpendingLimit(t *testing.T) {
	t.Parallel()

	// Scale with no spending limit. $200 usage on $99 credit -> $101 overage.
	enforcer, _ := setupSpendingEnforcer(t, "org-scale", "scale", -1, 200_000_000)

	err := enforcer.CheckSpendingLimit(context.Background(), "org-scale")
	if err != nil {
		t.Fatalf("expected no block for Scale with unlimited spending: %v", err)
	}
}

func TestOverage_NoSubscription_FreeTierFallback(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		periodSpendByOrg: map[string]int64{
			"org-none": CreditFreeMicrousd + 1,
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	// No subscription -> falls back to free tier -> hard cap at $1.
	err := enforcer.CheckSpendingLimit(context.Background(), "org-none")
	if err == nil {
		t.Fatal("expected free tier fallback to block over $1 credit")
	}
}

func TestOverage_StarterTier_UnderCredit_Passes(t *testing.T) {
	t.Parallel()

	enforcer, _ := setupSpendingEnforcer(t, "org-starter", "starter", -1, CreditStarterMicrousd-1)

	err := enforcer.CheckSpendingLimit(context.Background(), "org-starter")
	if err != nil {
		t.Fatalf("expected pass under starter credit: %v", err)
	}
}

func TestOverage_ConcurrentSpendChecks(t *testing.T) {
	t.Parallel()

	// Pro with $10 spending limit, currently at $59.98 spend ($49.99 credit + $9.99 overage).
	// This is just under the limit. Many concurrent checks should all pass.
	enforcer, _ := setupSpendingEnforcer(t, "org-race", "pro", 10_000_000, 59_980_000)

	var wg conc.WaitGroup
	errs := make(chan error, 100)

	for range 100 {
		wg.Go(func() {
			if err := enforcer.CheckSpendingLimit(context.Background(), "org-race"); err != nil {
				errs <- err
			}
		})
	}

	wg.Wait()
	close(errs)

	// All should pass since we're under the limit.
	for err := range errs {
		t.Errorf("unexpected error in concurrent check: %v", err)
	}
}

func TestOverage_AllTiers_CorrectBehavior(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		tier          string
		spendingLimit int64
		periodSpend   int64
		wantBlock     bool
	}{
		{"free_under", "free", 0, 500_000, false},
		{"free_over", "free", 0, 1_500_000, true},
		{"starter_no_limit_over", "starter", -1, 50_000_000, false},
		{"pro_no_limit_over", "pro", -1, 100_000_000, false},
		{"scale_no_limit_over", "scale", -1, 200_000_000, false},
		{"pro_with_limit_under", "pro", 50_000_000, 80_000_000, false},
		{"pro_with_limit_over", "pro", 50_000_000, 100_000_000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			orgID := "org-" + tt.name
			enforcer, _ := setupSpendingEnforcer(t, orgID, tt.tier, tt.spendingLimit, tt.periodSpend)

			err := enforcer.CheckSpendingLimit(context.Background(), orgID)
			blocked := err != nil
			if blocked != tt.wantBlock {
				t.Errorf("blocked = %v, want %v (err: %v)", blocked, tt.wantBlock, err)
			}
		})
	}
}

func FuzzSpendingLimitEnforcement(f *testing.F) {
	f.Add("free", int64(0), int64(0))
	f.Add("starter", int64(-1), int64(50000000))
	f.Add("pro", int64(50000000), int64(100000000))
	f.Add("scale", int64(-1), int64(200000000))
	f.Add("enterprise", int64(0), int64(0))
	f.Add("unknown", int64(-1), int64(999999999))

	f.Fuzz(func(t *testing.T, tier string, spendingLimit, periodSpend int64) {
		if !domain.PlanTier(tier).IsValid() {
			tier = "free"
		}
		if periodSpend < 0 {
			periodSpend = 0
		}

		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		now := time.Now()
		ps := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		pe := ps.AddDate(0, 1, 0)

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-fuzz": {
					OrgID:                 "org-fuzz",
					PlanTier:              tier,
					Status:                "active",
					SpendingLimitMicrousd: spendingLimit,
					LimitAction:           "reject",
					CurrentPeriodStart:    &ps,
					CurrentPeriodEnd:      &pe,
				},
			},
			periodSpendByOrg: map[string]int64{"org-fuzz": periodSpend},
		}

		enforcer := NewEnforcer(store, rdb, slog.Default())
		// Should never panic.
		_ = enforcer.CheckSpendingLimit(context.Background(), "org-fuzz")
	})
}
