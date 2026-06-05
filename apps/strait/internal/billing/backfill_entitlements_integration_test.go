//go:build integration

package billing_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedSubsForBackfill creates n org_subscriptions rows with mixed tiers and
// blanks the entitlements column on each so the backfill has work to do.
// Returns the slice of orgIDs in seeded order.
func seedSubsForBackfill(t *testing.T, ctx context.Context, pgStore *billing.PgStore, n int) []string {
	t.Helper()
	tiers := []domain.PlanTier{
		domain.PlanFree, domain.PlanStarter, domain.PlanPro,
		domain.PlanScale, domain.PlanBusiness,
	}
	ids := make([]string, n)
	for i := range n {
		id := "org-bf-" + newID()
		ids[i] = id
		require.NoError(t, pgStore.
			EnsureOrgSubscription(ctx,
				id))

		tier := tiers[i%len(tiers)]
		if tier != domain.PlanFree {
			require.NoError(t, pgStore.
				UpdateOrgSubscriptionPlan(ctx,
					id, string(
						tier), "active",
				))

		}
		// Seed an addon on every third row to exercise the addon path.
		if i%3 == 0 && tier != domain.PlanFree {
			require.NoError(t, pgStore.
				CreateAddon(ctx, &billing.
					Addon{ID: newID(), OrgID: id,

					AddonType: billing.AddonConcurrency100,
					Quantity:  1,

					Active: true}))

		}
	}
	// Blank entitlements on all rows so the backfill has work to do.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE organization_subscriptions SET entitlements = '{}'::jsonb`); err != nil {
		require.Failf(t, "test failure",

			"blank entitlements: %v", err)
	}
	return ids
}

func TestBackfillEntitlements_PopulatesAllRows(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	n := 50
	ids := seedSubsForBackfill(t, ctx, pgStore, n)

	stats, err := billing.BackfillEntitlements(ctx, testDB.Pool, pgStore, 7, false, "", nil)
	require.NoError(t, err)
	assert.Equal(t, int64(n),

		stats.Scanned,
	)
	assert.Equal(t, int64(n),

		stats.Updated,
	)

	// Every row's snapshot must equal ComputeEntitlements over its current state.
	for _, id := range ids {
		sub, err := pgStore.GetOrgSubscription(ctx, id)
		require.NoError(t, err)

		addons, err := pgStore.ListActiveAddons(ctx, id)
		require.NoError(t, err)

		want := billing.ComputeEntitlements(sub, addons)
		got := readEntitlements(t, ctx, id)
		mustEqualLimits(t, got, want, "after backfill: "+id)
	}
}

func TestBackfillEntitlements_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	seedSubsForBackfill(t, ctx, pgStore, 20)

	// First run writes everything.
	first, err := billing.BackfillEntitlements(ctx, testDB.Pool, pgStore, 5, false, "", nil)
	require.NoError(t, err)
	assert.EqualValues(t, 20, first.
		Updated)

	// Second run is a no-op because the snapshots already match.
	second, err := billing.BackfillEntitlements(ctx, testDB.Pool, pgStore, 5, false, "", nil)
	require.NoError(t, err)
	assert.EqualValues(t, 0, second.
		Updated)
	assert.EqualValues(t, 20, second.
		Scanned)

}

func TestBackfillEntitlements_DryRunWritesNothing(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	ids := seedSubsForBackfill(t, ctx, pgStore, 10)

	stats, err := billing.BackfillEntitlements(ctx, testDB.Pool, pgStore, 5, true, "", nil)
	require.NoError(t, err)
	assert.EqualValues(t, 10, stats.
		Updated)

	// Column must still be blank.
	for _, id := range ids {
		var raw []byte
		require.NoError(t, testDB.
			Pool.QueryRow(ctx, `SELECT entitlements FROM organization_subscriptions WHERE org_id = $1`,

			id).Scan(&raw))
		assert.Equal(t, "{}", string(raw))

	}
}

func TestBackfillEntitlements_SingleOrgScope(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	ids := seedSubsForBackfill(t, ctx, pgStore, 5)

	target := ids[2]
	stats, err := billing.BackfillEntitlements(ctx, testDB.Pool, pgStore, 100, false, target, nil)
	require.NoError(t, err)
	assert.False(t, stats.Scanned !=
		1 ||
		stats.Updated !=
			1)

	// Target row populated; others still blank.
	for i, id := range ids {
		var raw []byte
		require.NoError(t, testDB.
			Pool.QueryRow(ctx, `SELECT entitlements FROM organization_subscriptions WHERE org_id = $1`,

			id).Scan(&raw))

		if i == 2 {
			assert.NotEqual(t, "{}",
				string(raw))

		} else if string(raw) != "{}" {
			assert.Failf(t, "test failure",

				"non-target row %s was touched: %q", id, string(raw))
		}
	}
}

func TestUpdateEntitlementsIfUnchanged_SkipsStaleBackfillWrite(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-backfill-freshness-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx,
			orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID, string(domain.PlanPro), "active"))

	observed, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)

	staleProSnapshot := billing.GetPlanLimits(domain.PlanPro)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID, string(domain.PlanFree), "active"))

	updated, err := billing.UpdateEntitlementsIfUnchanged(ctx, testDB.Pool, orgID, staleProSnapshot, observed.UpdatedAt)
	require.NoError(t, err)
	require.False(t, updated)

	got := readEntitlements(t, ctx, orgID)
	require.Equal(t, domain.PlanFree,

		got.
			PlanTier,
	)

}

// TestBackfillEntitlements_AdversarialConcurrentWebhookWriter races the
// backfill against a concurrent UpdateOrgSubscriptionPlan loop that keeps
// flipping plan tiers on the same orgs. Final state must equal
// ComputeEntitlements over whatever state landed last — no torn writes.
func TestBackfillEntitlements_AdversarialConcurrentWebhookWriter(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	ids := seedSubsForBackfill(t, ctx, pgStore, 30)

	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	concWG.Go(func() {
		defer wg.Done()
		tiers := []domain.PlanTier{domain.PlanStarter, domain.PlanPro, domain.PlanScale}
		i := 0
		for !stop.Load() {
			id := ids[i%len(ids)]
			tier := tiers[i%len(tiers)]
			_ = pgStore.UpdateOrgSubscriptionPlan(ctx, id, string(tier), "active")
			i++
		}
	})

	// Run the backfill while writers churn.
	if _, err := billing.BackfillEntitlements(ctx, testDB.Pool, pgStore, 5, false, "", nil); err != nil {
		stop.Store(true)
		wg.Wait()
		require.Failf(t, "test failure",

			"backfill: %v", err)
	}
	stop.Store(true)
	wg.Wait()

	// One final consistent backfill pass to settle everything.
	if _, err := billing.BackfillEntitlements(ctx, testDB.Pool, pgStore, 5, false, "", nil); err != nil {
		require.Failf(t, "test failure",

			"final backfill: %v", err)
	}

	// Now every row's snapshot must equal what ComputeEntitlements returns
	// for its current state.
	for _, id := range ids {
		sub, err := pgStore.GetOrgSubscription(ctx, id)
		require.NoError(t, err)

		addons, err := pgStore.ListActiveAddons(ctx, id)
		require.NoError(t, err)

		want := billing.ComputeEntitlements(sub, addons)
		got := readEntitlements(t, ctx, id)
		mustEqualLimits(t, got, want, "post-race: "+id)
	}
}
