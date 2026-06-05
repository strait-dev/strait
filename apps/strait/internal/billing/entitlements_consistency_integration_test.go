//go:build integration

package billing_test

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"reflect"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snapshotMatchesCompute is the central equality check for entitlements: at
// any observable state, the persisted entitlements snapshot for an org must be
// exactly what billing.ComputeEntitlements would produce given the same
// subscription row and active addons. Drift here means a writer somewhere
// forgot to call refreshEntitlements. Returning the result lets the meta
// drift-injection test verify the check actually catches drift.
func snapshotMatchesCompute(
	t *testing.T,
	ctx context.Context,
	pgStore *billing.PgStore,
	orgID string,
) (matches bool, got, want billing.OrgPlanLimits) {
	t.Helper()
	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)

	addons, err := pgStore.ListActiveAddons(ctx, orgID)
	require.NoError(t, err)

	want = billing.ComputeEntitlements(sub, addons)

	var raw []byte
	require.NoError(t, testDB.
		Pool.QueryRow(ctx, `SELECT entitlements FROM organization_subscriptions WHERE org_id = $1`,

		orgID).Scan(&raw))
	require.NoError(t, json.Unmarshal(raw,
		&got))

	return reflect.DeepEqual(got, want), got, want
}

func assertSnapshotMatches(
	t *testing.T,
	ctx context.Context,
	pgStore *billing.PgStore,
	orgID, label string,
) {
	t.Helper()
	ok, _, _ := snapshotMatchesCompute(t, ctx, pgStore, orgID)
	assert.True(t, ok)

}

// TestEntitlementsConsistency_FixtureChain runs the canonical lifecycle
// chain: created -> upgraded -> addon_added -> upgraded_again -> addon_removed
// -> restricted -> resumed. After every step the persisted snapshot must
// equal ComputeEntitlements for the resulting state.
func TestEntitlementsConsistency_FixtureChain(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-cons-chain-" + newID()
	addonID := newID()

	type step struct {
		name string
		do   func() error
	}
	steps := []step{
		{"created", func() error { return pgStore.EnsureOrgSubscription(ctx, orgID) }},
		{"upgraded_to_pro", func() error {
			return pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active")
		}},
		{"addon_added", func() error {
			return pgStore.CreateAddon(ctx, &billing.Addon{
				ID: addonID, OrgID: orgID, AddonType: billing.AddonConcurrency100,
				Quantity: 2, Active: true,
			})
		}},
		{"upgraded_to_scale", func() error {
			return pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanScale), "active")
		}},
		{"addon_removed", func() error { return pgStore.DeactivateAddon(ctx, addonID) }},
		{"restricted", func() error {
			grace := time.Now().Add(7 * 24 * time.Hour)
			return billing.RestrictOrgTx(ctx, testDB.Pool, orgID, &grace)
		}},
		{"resumed", func() error {
			return pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active")
		}},
	}

	for _, s := range steps {
		require.NoError(t, s.do())

		assertSnapshotMatches(t, ctx, pgStore, orgID, s.name)
	}
}

// TestEntitlementsConsistency_RandomWalk performs 50 random mutations on a
// single org, exercising the cartesian product of writers we have. The
// invariant must hold after every step. The seed is fixed so failures are
// reproducible.
func TestEntitlementsConsistency_RandomWalk(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-cons-walk-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))

	tiers := []domain.PlanTier{
		domain.PlanFree, domain.PlanStarter, domain.PlanPro,
		domain.PlanScale, domain.PlanBusiness,
	}
	addonTypes := []billing.AddonType{
		billing.AddonConcurrency100,
		billing.AddonHistory30d,
		billing.AddonEnvironments5,
	}

	r := rand.New(rand.NewPCG(0xC0FFEE, 0xDEADBEEF))
	activeAddons := map[string]struct{}{}

	const steps = 50
	for i := range steps {
		var label string
		choice := r.IntN(10)
		switch {
		case choice < 3:
			tier := tiers[r.IntN(len(tiers))]
			label = "plan_change=" + string(tier)
			if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(tier), "active"); err != nil {
				require.Failf(t, "test failure",

					"step %d %s: %v", i, label, err)
			}
		case choice < 6:
			at := addonTypes[r.IntN(len(addonTypes))]
			id := newID()
			label = "addon_create=" + string(at)
			if err := pgStore.CreateAddon(ctx, &billing.Addon{
				ID: id, OrgID: orgID, AddonType: at,
				Quantity: 1 + r.IntN(3), Active: true,
			}); err != nil {
				require.Failf(t, "test failure",

					"step %d %s: %v", i, label, err)
			}
			activeAddons[id] = struct{}{}
		case choice < 8 && len(activeAddons) > 0:
			var id string
			for k := range activeAddons {
				id = k
				break
			}
			delete(activeAddons, id)
			label = "addon_deactivate"
			if err := pgStore.DeactivateAddon(ctx, id); err != nil {
				require.Failf(t, "test failure",

					"step %d %s: %v", i, label, err)
			}
		case choice < 9:
			label = "restrict"
			grace := time.Now().Add(7 * 24 * time.Hour)
			if err := billing.RestrictOrgTx(ctx, testDB.Pool, orgID, &grace); err != nil {
				require.Failf(t, "test failure",

					"step %d %s: %v", i, label, err)
			}
		default:
			tier := tiers[1+r.IntN(len(tiers)-1)] // skip Free
			label = "resume_to=" + string(tier)
			if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(tier), "active"); err != nil {
				require.Failf(t, "test failure",

					"step %d %s: %v", i, label, err)
			}
		}
		assertSnapshotMatches(t, ctx, pgStore, orgID, label)
	}
}

// TestEntitlementsConsistency_GuardCatchesInjectedDrift deliberately corrupts
// the persisted snapshot to simulate a future writer that forgot to refresh
// entitlements, and asserts the consistency check would have flagged it.
// Without this, the green CI signal from the other consistency tests is
// meaningless because we never proved the equality check itself can fail.
func TestEntitlementsConsistency_GuardCatchesInjectedDrift(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-cons-drift-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx, orgID,
			string(domain.
				PlanPro), "active",
		))

	// Sanity: snapshot is consistent for Pro before the injection.
	if ok, _, _ := snapshotMatchesCompute(t, ctx, pgStore, orgID); !ok {
		require.Failf(t, "test failure",

			"baseline mismatch — fixture pollution invalidates the drift test")
	}

	// Inject drift: hand-write Enterprise limits without going through any
	// mutator. A future writer that "forgets" to call refreshEntitlements
	// leaves the column in exactly this state.
	enterprise := billing.GetPlanLimits(domain.PlanEnterprise)
	raw, err := json.Marshal(enterprise)
	require.NoError(t, err)

	writeRawEntitlements(t, ctx, orgID, raw)

	ok, _, _ := snapshotMatchesCompute(t, ctx, pgStore, orgID)
	assert.False(t, ok)

}
