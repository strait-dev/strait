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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID, string(domain.
				PlanPro),

			"active"))

	p := createProject(t, ctx, q, orgID, "PB-Block")
	require.NoError(t, pgStore.
		SetProjectOrgID(ctx,
			p.ID, orgID,
		))

	const budget = int64(2_500_000)
	require.NoError(t, pgStore.
		SetProjectBudget(ctx,
			p.ID, budget,
			"block",
		))

	// $2.50

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
		require.NoError(t, pgStore.
			UpsertUsageRecord(ctx,
				rec))

	}

	enforcer := billing.NewEnforcer(pgStore, rdb, slog.Default())

	err := enforcer.CheckProjectBudgetLimit(ctx, p.ID)
	require.Error(t, err)

	var lim *billing.LimitError
	require.True(t, errors.As(
		err, &lim,
	))
	assert.Equal(t, "project_budget_reached",

		lim.
			Code)
	assert.Equal(t, budget, lim.
		Limit)

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
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID, string(domain.
				PlanPro),

			"active"))

	p := createProject(t, ctx, q, orgID, "PB-Notify")
	require.NoError(t, pgStore.
		SetProjectOrgID(ctx,
			p.ID, orgID,
		))

	const budget = int64(2_500_000)
	require.NoError(t, pgStore.
		SetProjectBudget(ctx,
			p.ID, budget,
			"notify",
		))

	rec := &billing.UsageRecord{
		ID:               newID(),
		OrgID:            orgID,
		ProjectID:        p.ID,
		PeriodDate:       time.Now().UTC(),
		RunsCount:        2,
		ComputeCostMicro: budget,
	}
	require.NoError(t, pgStore.
		UpsertUsageRecord(ctx,
			rec))

	enforcer := billing.NewEnforcer(pgStore, rdb, slog.Default())
	assert.NoError(t, enforcer.
		CheckProjectBudgetLimit(ctx,
			p.ID))

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
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID, string(domain.
				PlanPro),

			"active"))

	p := createProject(t, ctx, q, orgID, "PB-NoQuota")
	require.NoError(t, pgStore.
		SetProjectOrgID(ctx,
			p.ID, orgID,
		))

	enforcer := billing.NewEnforcer(pgStore, rdb, slog.Default())
	assert.NoError(t, enforcer.
		CheckProjectBudgetLimit(ctx,
			p.ID))

}
