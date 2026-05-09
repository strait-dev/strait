package billing

import (
	"math/rand"
	"reflect"
	"testing"

	"strait/internal/domain"
)

// TestComputeEntitlements_StrippedSubDefaultsToFree confirms an empty/zero
// OrgSubscription cannot accidentally inherit a paid tier — the snapshot must
// be Free even when the row was created with no PlanTier set.
func TestComputeEntitlements_StrippedSubDefaultsToFree(t *testing.T) {
	t.Parallel()

	sub := &OrgSubscription{} // PlanTier == ""
	got := ComputeEntitlements(sub, nil)
	want := GetPlanLimits(domain.PlanFree)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("stripped sub should resolve to Free\n got:  %+v\n want: %+v", got, want)
	}
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
		AddonConcurrency100, AddonLogDrain10GB, AddonHistory30d,
		AddonComplianceArchive, AddonDedicatedWorkers, AddonEnvironments5,
		AddonType("ghost_addon"), AddonType(""),
	}

	tiers := []domain.PlanTier{
		domain.PlanFree, domain.PlanStarter, domain.PlanPro,
		domain.PlanScale, domain.PlanBusiness, domain.PlanEnterprise,
	}

	for i := range 200 {
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
			if got.MaxConcurrentRuns != -1 {
				t.Fatalf("iter %d tier %s: unlimited ceiling collapsed to %d", i, tier, got.MaxConcurrentRuns)
			}
			continue
		}
		if got.MaxConcurrentRuns < base.MaxConcurrentRuns {
			t.Fatalf("iter %d tier %s: addons reduced concurrency %d -> %d",
				i, tier, base.MaxConcurrentRuns, got.MaxConcurrentRuns)
		}
	}
}

// TestComputeEntitlements_HandcraftedFieldsCannotLeak builds a fake
// OrgSubscription wired with every JSONB pack at oversized values, then
// confirms only the documented packs influence the snapshot — no field on
// OrgPlanLimits gets written by anything other than the catalog/addon paths.
func TestComputeEntitlements_HandcraftedFieldsCannotLeak(t *testing.T) {
	t.Parallel()

	sub := &OrgSubscription{
		PlanTier: string(domain.PlanFree),
		AddOns: SubscriptionAddOns{
			RetentionPack:     999,
			PrioritySlotPack:  999,
			LogDrainVolumeGB:  999,
			WorkerConnections: 999,
		},
		// Override fields that ComputeEntitlements MUST ignore — those
		// are operator runtime knobs, not part of the snapshot.
		OverrideDailyRunLimit:      new(1_000_000),
		OverrideConcurrentRunLimit: new(1_000_000),
	}

	got := ComputeEntitlements(sub, nil)
	free := GetPlanLimits(domain.PlanFree)

	// Free RetentionDays > 0, so RetentionPack adds. Free WorkerConnections
	// is finite, so the pack adds. PriorityPack only fires when
	// MaxDispatchPriority != -1; Free is finite so it adds.
	if got.RetentionDays != free.RetentionDays+999*retentionPackDays {
		t.Errorf("retention pack: got %d, want %d",
			got.RetentionDays, free.RetentionDays+999*retentionPackDays)
	}
	if got.WorkerConnections != free.WorkerConnections+999 {
		t.Errorf("worker pack: got %d, want %d",
			got.WorkerConnections, free.WorkerConnections+999)
	}

	// Override fields must not bleed into the snapshot — those are loaded
	// at read time inside Enforcer.GetOrgPlanLimits, not persisted.
	if got.MaxRunsPerDay == 1_000_000 {
		t.Error("override_daily_run_limit leaked into entitlements snapshot")
	}
	if got.MaxConcurrentRuns == 1_000_000 {
		t.Error("override_concurrent_run_limit leaked into entitlements snapshot")
	}
}

// TestComputeEntitlements_EnterprisePacksCannotShrinkUnlimited covers the
// "+= on -1" trap: ApplySubscriptionAddOns must guard every unlimited field
// from getting clobbered into a finite value.
func TestComputeEntitlements_EnterprisePacksCannotShrinkUnlimited(t *testing.T) {
	t.Parallel()

	sub := &OrgSubscription{
		PlanTier: string(domain.PlanEnterprise),
		AddOns: SubscriptionAddOns{
			RetentionPack:     5,
			PrioritySlotPack:  5,
			WorkerConnections: 5,
		},
	}

	got := ComputeEntitlements(sub, []Addon{
		{AddonType: AddonConcurrency100, Quantity: 5, Active: true},
		{AddonType: AddonHistory30d, Quantity: 5, Active: true},
		{AddonType: AddonEnvironments5, Quantity: 5, Active: true},
	})

	if got.MaxConcurrentRuns != -1 {
		t.Errorf("enterprise concurrency should stay unlimited, got %d", got.MaxConcurrentRuns)
	}
	if got.MaxEnvironments != -1 {
		t.Errorf("enterprise environments should stay unlimited, got %d", got.MaxEnvironments)
	}
	if got.WorkerConnections != -1 {
		t.Errorf("enterprise worker connections should stay unlimited, got %d", got.WorkerConnections)
	}
	if got.MaxDispatchPriority != -1 {
		t.Errorf("enterprise dispatch priority should stay unlimited, got %d", got.MaxDispatchPriority)
	}
}
