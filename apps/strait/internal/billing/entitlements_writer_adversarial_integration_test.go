//go:build integration

package billing_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdversarialWriter_DuplicateAddonReplay simulates Stripe redelivery of
// the same addon-create event. Idempotency is upstream (processed_webhook_messages)
// so at the store layer a duplicate ID surfaces as an INSERT conflict — but the
// already-persisted snapshot must remain valid (matching the active addon set)
// regardless of the replay outcome.
func TestAdversarialWriter_DuplicateAddonReplay(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-adv-replay-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx,
			orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID,
			string(domain.
				PlanPro), "active"))

	addon := &billing.Addon{
		ID:        newID(),
		OrgID:     orgID,
		AddonType: billing.AddonConcurrency100,
		Quantity:  1,
		Active:    true,
	}
	require.NoError(t, pgStore.
		CreateAddon(ctx, addon))

	first := readEntitlements(t, ctx, orgID)
	require.Error(t, pgStore.
		CreateAddon(ctx, addon))

	// Replay: same ID again. Expect a primary-key violation; the snapshot
	// must NOT be torn — it should still equal the post-first-create state.

	second := readEntitlements(t, ctx, orgID)
	assert.Equal(t, second.MaxConcurrentRuns,

		first.
			MaxConcurrentRuns,
	)

	// And the resolved snapshot must equal what ComputeEntitlements
	// computes from the actual addon set (one active row, not two).
	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)

	addons, err := pgStore.ListActiveAddons(ctx, orgID)
	require.NoError(t, err)

	want := billing.ComputeEntitlements(sub, addons)
	mustEqualLimits(t, second, want, "after replay")
}

// TestAdversarialWriter_ConcurrentFullUpdates fires N concurrent
// UpdateOrgSubscriptionFull calls for the same org with different plan tiers.
// The final persisted entitlements must equal a valid serialization of one
// of the inputs — never a torn write blending tiers.
func TestAdversarialWriter_ConcurrentFullUpdates(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-adv-race-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx,
			orgID))

	tiers := []domain.PlanTier{
		domain.PlanStarter,
		domain.PlanPro,
		domain.PlanScale,
		domain.PlanBusiness,
	}
	now := time.Now().UTC()
	pe := now.Add(30 * 24 * time.Hour)

	var wg sync.WaitGroup
	for _, tier := range tiers {
		wg.Add(1)
		{
			tier := tier
			concWG.Go(func() {
				defer wg.Done()
				_ = pgStore.UpdateOrgSubscriptionFull(ctx, orgID, string(tier), "active", &now, &pe)
			})
		}
	}
	wg.Wait()

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)

	addons, err := pgStore.ListActiveAddons(ctx, orgID)
	require.NoError(t, err)

	want := billing.ComputeEntitlements(sub, addons)
	got := readEntitlements(t, ctx, orgID)

	// Final snapshot must equal ComputeEntitlements over the final row.
	// (Refresh-after-write means whichever UPDATE landed last sets both
	// the row and the snapshot; even if a slow refresh fires after a
	// later mutator's refresh, both reads come from a coherent state.)
	mustEqualLimits(t, got, want, "after concurrent full updates")

	// And the row-resolved tier must be one of the candidates, not zero.
	matched := false
	for _, tier := range tiers {
		if sub.PlanTier == string(tier) {
			matched = true
			break
		}
	}
	assert.True(t, matched)

}

// TestAdversarialWriter_DeactivateThenCreateRefreshes covers the addon
// deactivate -> create cycle. The snapshot must reflect each transition;
// a stale snapshot from before the deactivate must not survive.
func TestAdversarialWriter_DeactivateThenCreateRefreshes(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-adv-cycle-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx,
			orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID,
			string(domain.
				PlanPro), "active"))

	a := &billing.Addon{ID: newID(), OrgID: orgID, AddonType: billing.AddonConcurrency100, Quantity: 3, Active: true}
	require.NoError(t, pgStore.
		CreateAddon(ctx, a),
	)

	withA := readEntitlements(t, ctx, orgID)
	require.NoError(t, pgStore.
		DeactivateAddon(ctx,
			a.ID))

	postDeactivate := readEntitlements(t, ctx, orgID)
	assert.False(t, postDeactivate.
		MaxConcurrentRuns >=
		withA.
			MaxConcurrentRuns,
	)

	b := &billing.Addon{ID: newID(), OrgID: orgID, AddonType: billing.AddonConcurrency100, Quantity: 1, Active: true}
	require.NoError(t, pgStore.
		CreateAddon(ctx, b),
	)

	withB := readEntitlements(t, ctx, orgID)
	assert.Equal(t, postDeactivate.
		MaxConcurrentRuns+
		100,
		withB.MaxConcurrentRuns,
	)

}

// TestAdversarialWriter_RestrictThenResume covers the trust-boundary
// case where an org gets restricted (snapshot collapses to Free) and is
// later resumed via UpdateOrgSubscriptionFull. The post-resume snapshot
// must reflect the new tier, not the stale Free snapshot.
func TestAdversarialWriter_RestrictThenResume(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-adv-resume-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx,
			orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID,
			string(domain.
				PlanPro), "active"))

	graceEnd := time.Now().Add(7 * 24 * time.Hour)
	require.NoError(t, billing.
		RestrictOrgTx(ctx,
			testDB.Pool,
			orgID,
			&graceEnd,
		))

	mustEqualLimits(t, readEntitlements(t, ctx, orgID),
		billing.GetPlanLimits(domain.PlanFree), "after restrict")

	// Resume: customer pays, plan goes back to Pro.
	now := time.Now().UTC()
	pe := now.Add(30 * 24 * time.Hour)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionFull(ctx,
			orgID,
			string(domain.
				PlanPro), "active", &now,
			&pe))

	mustEqualLimits(t, readEntitlements(t, ctx, orgID),
		billing.GetPlanLimits(domain.PlanPro), "after resume")
}

func TestAdversarialWriter_PaymentRestrictionRefreshesEntitlements(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-payment-restrict-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx,
			orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID,
			string(domain.
				PlanPro), "active"))

	mustEqualLimits(t, readEntitlements(t, ctx, orgID),
		billing.GetPlanLimits(domain.PlanPro), "before payment restriction")
	require.NoError(t, pgStore.
		UpdatePaymentStatus(ctx, orgID,
			"restricted",

			nil,
		))

	mustEqualLimits(t, readEntitlements(t, ctx, orgID),
		billing.GetPlanLimits(domain.PlanFree), "restricted payment status")
	require.NoError(t, pgStore.
		UpdatePaymentStatus(ctx, orgID,
			"ok",
			nil))

	mustEqualLimits(t, readEntitlements(t, ctx, orgID),
		billing.GetPlanLimits(domain.PlanPro), "payment status restored")
}
