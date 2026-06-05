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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// Orchestration-only: free tier has no included compute credit.
	// Any non-zero spend triggers the budget cap.
	enforcer, _ := setupSpendingEnforcer(t, "org-free", "free", 0, 1)

	err := enforcer.CheckSpendingLimit(context.Background(), "org-free")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(
		err, &le))
	assert.Equal(t, "spending_limit_reached",

		le.Code,
	)
}

func TestOverage_FreeTier_ZeroSpend_Passes(t *testing.T) {
	t.Parallel()

	// Free tier passes when there is no spend.
	enforcer, _ := setupSpendingEnforcer(t, "org-free", "free", 0, 0)

	err := enforcer.CheckSpendingLimit(context.Background(), "org-free")
	require.NoError(t,
		err)
}

func TestOverage_PaidTier_NoSpendingLimit_Allows(t *testing.T) {
	t.Parallel()

	// Pro with no spending limit (-1): overage is allowed indefinitely.
	// Spend $200 on a $49.99 plan -- no block.
	enforcer, _ := setupSpendingEnforcer(t, "org-pro", "pro", -1, 200_000_000)

	err := enforcer.CheckSpendingLimit(context.Background(), "org-pro")
	require.NoError(t,
		err)
}

func TestOverage_PaidTier_WithSpendingLimit_Blocks(t *testing.T) {
	t.Parallel()

	// Pro with $50 spending limit. Orchestration-only: no included credit.
	// Total spend = $100 > $50 limit -> block.
	enforcer, _ := setupSpendingEnforcer(t, "org-pro", "pro", 50_000_000, 100_000_000)

	err := enforcer.CheckSpendingLimit(context.Background(), "org-pro")
	require.Error(t,
		err)
}

func TestOverage_PaidTier_ZeroSpendingLimit_Blocks(t *testing.T) {
	t.Parallel()

	// Pro with $0 spending limit: any spend triggers the cap.
	enforcer, _ := setupSpendingEnforcer(t, "org-pro", "pro", 0, 1)

	err := enforcer.CheckSpendingLimit(context.Background(), "org-pro")
	require.Error(t,
		err)
}

func TestOverage_ScaleTier_NoSpendingLimit(t *testing.T) {
	t.Parallel()

	// Scale with no spending limit: high spend is allowed.
	enforcer, _ := setupSpendingEnforcer(t, "org-scale", "scale", -1, 200_000_000)

	err := enforcer.CheckSpendingLimit(context.Background(), "org-scale")
	require.NoError(t,
		err)
}

func TestOverage_NoSubscription_FreeTierFallback(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		periodSpendByOrg: map[string]int64{
			// Any spend triggers the free-tier cap (no included credit in orchestration-only mode).
			"org-none": 1,
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	err := enforcer.CheckSpendingLimit(context.Background(), "org-none")
	require.Error(t,
		err)
}

func TestOverage_StarterTier_NoLimit_LargeSpend_Passes(t *testing.T) {
	t.Parallel()

	// Starter with no spending limit (-1): unlimited spend is allowed.
	enforcer, _ := setupSpendingEnforcer(t, "org-starter", "starter", -1, 50_000_000)

	err := enforcer.CheckSpendingLimit(context.Background(), "org-starter")
	require.NoError(t,
		err)
}

func TestOverage_ConcurrentSpendChecks(t *testing.T) {
	t.Parallel()

	// Pro with $100 spending limit, spend = $9.99 (well under). Concurrent checks should all pass.
	enforcer, _ := setupSpendingEnforcer(t, "org-race", "pro", 100_000_000, 9_990_000)

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
		assert.Failf(t, "test failure",

			"unexpected error in concurrent check: %v", err)
	}
}

func TestOverage_AllTiers_CorrectBehavior(t *testing.T) {
	t.Parallel()

	// Orchestration-only: no included compute credit. SpendingLimitMicrousd is
	// compared against total period spend, not overage above included credit.
	tests := []struct {
		name          string
		tier          string
		spendingLimit int64
		periodSpend   int64
		wantBlock     bool
	}{
		{"free_zero", "free", 0, 0, false},
		{"free_any_spend", "free", 0, 1, true},
		{"free_over", "free", 0, 1_500_000, true},
		{"starter_no_limit_over", "starter", -1, 50_000_000, false},
		{"pro_no_limit_over", "pro", -1, 100_000_000, false},
		{"scale_no_limit_over", "scale", -1, 200_000_000, false},
		{"pro_with_limit_under", "pro", 50_000_000, 40_000_000, false},
		{"pro_with_limit_over", "pro", 50_000_000, 100_000_000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			orgID := "org-" + tt.name
			enforcer, _ := setupSpendingEnforcer(t, orgID, tt.tier, tt.spendingLimit, tt.periodSpend)

			err := enforcer.CheckSpendingLimit(context.Background(), orgID)
			blocked := err != nil
			assert.Equal(t, tt.
				wantBlock,
				blocked)
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
