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

// TestSpendingLimitIntegration_AtCapRejects seeds a real Pro-tier org
// with usage_records summing to the configured cap and asserts the
// enforcer returns *LimitError for that org. This is the bedrock
// integration check for dispatch wiring that relies on CheckSpendingLimit
// reading real Postgres state and producing a typed rejection that worker code
// can pattern-match.
func TestSpendingLimitIntegration_AtCapRejects(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	orgID := "org-spend-cap-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID, string(domain.
				PlanPro),

			"active"))

	const cap = int64(2_500_000)
	require.NoError(t, pgStore.
		UpdateSpendingLimit(ctx, orgID,
			cap, "block",
		))

	// $2.50

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
		require.NoError(t, pgStore.
			UpsertUsageRecord(ctx,
				rec))

	}

	enforcer := billing.NewEnforcer(pgStore, rdb, slog.Default())

	err := enforcer.CheckSpendingLimit(ctx, orgID)
	require.Error(t, err)

	var lim *billing.LimitError
	require.True(t, errors.As(
		err, &lim,
	))
	assert.Equal(t, "spending_limit_reached",

		lim.
			Code)
	assert.Equal(t, cap, lim.
		Limit,
	)

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
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID, string(domain.
				PlanPro),

			"active"))

	const cap = int64(10_000_000)
	require.NoError(t, pgStore.
		UpdateSpendingLimit(ctx, orgID,
			cap, "block",
		))

	// $10

	rec := &billing.UsageRecord{
		ID:               newID(),
		OrgID:            orgID,
		ProjectID:        newID(),
		PeriodDate:       time.Now().UTC(),
		RunsCount:        1,
		ComputeCostMicro: cap / 4,
	}
	require.NoError(t, pgStore.
		UpsertUsageRecord(ctx,
			rec))

	enforcer := billing.NewEnforcer(pgStore, rdb, slog.Default())
	assert.NoError(t, enforcer.
		CheckSpendingLimit(
			ctx, orgID,
		))

}
