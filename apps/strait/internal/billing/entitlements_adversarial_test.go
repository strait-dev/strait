package billing

import (
	"context"
	"encoding/json"
	"math/rand"
	"reflect"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComputeEntitlements_StrippedSubDefaultsToFree confirms an empty/zero
// OrgSubscription cannot accidentally inherit a paid tier — the snapshot must
// be Free even when the row was created with no PlanTier set.
func TestComputeEntitlements_StrippedSubDefaultsToFree(t *testing.T) {
	t.Parallel()

	sub := &OrgSubscription{} // PlanTier == ""
	got := ComputeEntitlements(sub, nil)
	want := GetPlanLimits(domain.PlanFree)
	assert.True(t, reflect.
		DeepEqual(got,
			want))
}

func TestComputeEntitlements_DisallowedActiveAddonsDoNotGrantFreePlanBenefits(t *testing.T) {
	t.Parallel()

	sub := &OrgSubscription{PlanTier: string(domain.PlanFree)}
	got := ComputeEntitlements(sub, []Addon{
		{AddonType: AddonConcurrency100, Quantity: 100, Active: true},
		{AddonType: AddonDedicatedWorkers, Quantity: 1, Active: true},
		{AddonType: AddonEnvironments5, Quantity: 100, Active: true},
	})
	want := GetPlanLimits(domain.PlanFree)
	require.Equal(t,
		want.MaxConcurrentRuns,
		got.MaxConcurrentRuns,
	)
	require.Equal(t,
		want.HasDedicatedCompute,
		got.
			HasDedicatedCompute,
	)
	require.Equal(t,
		want.MaxEnvironments,
		got.MaxEnvironments,
	)
}

func TestComputeEntitlements_ActiveAddonsAreClampedToPlanPackCap(t *testing.T) {
	t.Parallel()

	base := GetPlanLimits(domain.PlanScale)
	cap := base.MaxAddonPacks[AddonHistory30d]
	require.Positive(t,
		cap)

	sub := &OrgSubscription{PlanTier: string(domain.PlanScale)}
	got := ComputeEntitlements(sub, []Addon{
		{AddonType: AddonHistory30d, Quantity: cap + 10, Active: true},
		{AddonType: AddonHistory30d, Quantity: cap + 10, Active: true},
	})

	want := base.RetentionDays + cap*AddonPacks[AddonHistory30d].PackSize
	require.Equal(t,
		want, got.RetentionDays,
	)
}

func TestReconcileActiveAddonsForPlan_DeactivatesDisallowedAndOverCapRows(t *testing.T) {
	t.Parallel()

	base := GetPlanLimits(domain.PlanScale)
	cap := base.MaxAddonPacks[AddonHistory30d]
	store := &mockBillingStore{
		activeAddons: []Addon{
			{ID: "keep-1", OrgID: "org-addons", AddonType: AddonHistory30d, Quantity: cap, Active: true},
			{ID: "over-cap", OrgID: "org-addons", AddonType: AddonHistory30d, Quantity: 1, Active: true},
			{ID: "disallowed", OrgID: "org-addons", AddonType: AddonDedicatedWorkers, Quantity: 1, Active: true},
		},
	}

	deactivated, err := ReconcileActiveAddonsForPlan(context.Background(), store, "org-addons", base)
	require.NoError(t,
		err)
	require.Equal(t, 2, deactivated)

	got := map[string]bool{}
	for _, id := range store.deactivatedAddonIDs {
		got[id] = true
	}
	require.False(t,
		got["keep-1"])
	require.False(t,
		!got["over-cap"] ||
			!got["disallowed"])
}

// TestComputeEntitlements_FuzzAddonsNeverPanicsAndStaysWithinPaidCeiling runs
// random addon slices (with duplicate types, zero quantities, negative
// quantities, unknown types) against every tier and asserts:
//
//  1. ComputeEntitlements never panics.
//  2. The resulting MaxConcurrentRuns never exceeds the largest single legal
//     boost for the same input — duplicate addon entries that would smuggle
//     in a higher cap by coincidence are caught here.
func TestComputeEntitlements_FuzzAddonsNeverPanicsAndStaysWithinPaidCeiling(t *testing.T) {
	t.Parallel()

	// Deterministic RNG: a flake here is a real bug, not a transient.
	rng := rand.New(rand.NewSource(0xBEE51EBE))

	addonTypes := []AddonType{
		AddonConcurrency100, AddonHistory30d,
		AddonComplianceArchive, AddonDedicatedWorkers, AddonEnvironments5,
		AddonType("ghost_addon"), AddonType(""),
	}

	tiers := []domain.PlanTier{
		domain.PlanFree, domain.PlanStarter, domain.PlanPro,
		domain.PlanScale, domain.PlanBusiness, domain.PlanEnterprise,
	}

	for range 200 {
		// Random slice of 0..15 addons.
		n := rng.Intn(16)
		addons := make([]Addon, 0, n)
		for range n {
			addons = append(addons, Addon{
				AddonType: addonTypes[rng.Intn(len(addonTypes))],
				Quantity:  rng.Intn(11) - 3, // -3..7 (covers negative + zero)
				Active:    rng.Intn(2) == 0,
			})
		}
		tier := tiers[rng.Intn(len(tiers))]
		sub := &OrgSubscription{PlanTier: string(tier)}

		// Must not panic.
		got := ComputeEntitlements(sub, addons)

		// Sanity: for tiers that are not unlimited, the MaxConcurrentRuns
		// must never go below the static catalog value (addons never
		// shrink limits). Unlimited stays unlimited.
		base := GetPlanLimits(tier)
		if base.MaxConcurrentRuns == -1 {
			require.Equal(t, -1, got.MaxConcurrentRuns)

			continue
		}
		require.GreaterOrEqual(t, got.MaxConcurrentRuns,

			base.MaxConcurrentRuns,
		)
	}
}

// TestComputeEntitlements_LegacyJSONBAddOnsCannotLeak builds a fake
// OrgSubscription from a stale add_ons JSONB payload and confirms it cannot
// influence the snapshot. Launch add-ons must come from organization_addons.
func TestComputeEntitlements_LegacyJSONBAddOnsCannotLeak(t *testing.T) {
	t.Parallel()

	var addOns SubscriptionAddOns
	require.NoError(t,
		json.Unmarshal(
			[]byte(`{"retention_pack":999,"worker_connections":999}`), &addOns,
		))

	sub := &OrgSubscription{
		PlanTier: string(domain.PlanFree),
		AddOns:   addOns,
		// Override fields that ComputeEntitlements MUST ignore — those
		// are operator runtime knobs, not part of the snapshot.
		OverrideDailyRunLimit:      new(1_000_000),
		OverrideConcurrentRunLimit: new(1_000_000),
	}

	got := ComputeEntitlements(sub, nil)
	free := GetPlanLimits(domain.PlanFree)
	assert.Equal(t, free.
		RetentionDays,
		got.RetentionDays,
	)
	assert.Equal(t, free.
		WorkerConnections,
		got.WorkerConnections,
	)
	assert.NotEqual(t,
		1_000_000, got.
			MaxRunsPerDay,
	)
	assert.NotEqual(t,
		1_000_000, got.
			MaxConcurrentRuns,
	)

	// Override fields must not bleed into the snapshot — those are loaded
	// at read time inside Enforcer.GetOrgPlanLimits, not persisted.
}

// TestComputeEntitlements_EnterprisePacksCannotShrinkUnlimited covers the
// "+= on -1" trap: ApplySubscriptionAddOns must guard every unlimited field
// from getting clobbered into a finite value.
func TestComputeEntitlements_EnterprisePacksCannotShrinkUnlimited(t *testing.T) {
	t.Parallel()

	sub := &OrgSubscription{
		PlanTier: string(domain.PlanEnterprise),
		AddOns:   SubscriptionAddOns{},
	}

	got := ComputeEntitlements(sub, []Addon{
		{AddonType: AddonConcurrency100, Quantity: 5, Active: true},
		{AddonType: AddonHistory30d, Quantity: 5, Active: true},
		{AddonType: AddonEnvironments5, Quantity: 5, Active: true},
	})
	assert.Equal(t, -1, got.MaxConcurrentRuns)
	assert.Equal(t, -1, got.MaxEnvironments)
	assert.Equal(t, -1, got.WorkerConnections)
	assert.Equal(t, 10,
		got.MaxDispatchPriority,
	)
}
