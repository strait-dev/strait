package billing

import (
	"reflect"
	"testing"

	"strait/internal/domain"
)

// TestComputeEntitlements_RoundTripMatchesPipeline locks ComputeEntitlements
// to the existing 3-step pipeline byte-for-byte across every tier × addon
// combination from TestAddonEnforcement. Drift between the two paths is the
// failure mode this guards against.
func TestComputeEntitlements_RoundTripMatchesPipeline(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		tier   domain.PlanTier
		addons []Addon
		addOns SubscriptionAddOns
	}{
		{"free_no_addons", domain.PlanFree, nil, SubscriptionAddOns{}},
		{"starter_no_addons", domain.PlanStarter, nil, SubscriptionAddOns{}},
		{"pro_no_addons", domain.PlanPro, nil, SubscriptionAddOns{}},
		{"scale_no_addons", domain.PlanScale, nil, SubscriptionAddOns{}},
		{"business_no_addons", domain.PlanBusiness, nil, SubscriptionAddOns{}},
		{"enterprise_no_addons", domain.PlanEnterprise, nil, SubscriptionAddOns{}},

		{"pro_concurrency_pack",
			domain.PlanPro,
			[]Addon{{AddonType: AddonConcurrency100, Quantity: 1, Active: true}},
			SubscriptionAddOns{}},
		{"pro_envs_pack",
			domain.PlanPro,
			[]Addon{{AddonType: AddonEnvironments5, Quantity: 3, Active: true}},
			SubscriptionAddOns{}},
		{"starter_history_pack",
			domain.PlanStarter,
			[]Addon{{AddonType: AddonHistory30d, Quantity: 1, Active: true}},
			SubscriptionAddOns{}},
		{"pro_compliance_archive",
			domain.PlanPro,
			[]Addon{{AddonType: AddonComplianceArchive, Quantity: 1, Active: true}},
			SubscriptionAddOns{}},
		{"pro_dedicated_workers",
			domain.PlanPro,
			[]Addon{{AddonType: AddonDedicatedWorkers, Quantity: 1, Active: true}},
			SubscriptionAddOns{}},

		{"pro_table_addons",
			domain.PlanPro,
			[]Addon{
				{AddonType: AddonConcurrency100, Quantity: 2, Active: true},
				{AddonType: AddonHistory30d, Quantity: 1, Active: true},
			},
			SubscriptionAddOns{}},

		{"enterprise_with_packs_stays_unlimited",
			domain.PlanEnterprise,
			[]Addon{{AddonType: AddonConcurrency100, Quantity: 10, Active: true}},
			SubscriptionAddOns{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sub := &OrgSubscription{
				PlanTier: string(tc.tier),
				AddOns:   tc.addOns,
			}

			pipeline := GetPlanLimits(tc.tier)
			pipeline = EffectiveLimits(pipeline, tc.addons)
			pipeline = ApplySubscriptionAddOns(pipeline, tc.addOns)

			got := ComputeEntitlements(sub, tc.addons)

			if !reflect.DeepEqual(got, pipeline) {
				t.Errorf("snapshot != pipeline\n got:  %+v\n want: %+v", got, pipeline)
			}
		})
	}
}

// TestComputeEntitlements_UnknownTierFallsBackToFree exercises the same
// unknown-tier fallback that GetPlanLimits provides — proves the snapshot
// never silently upgrades a misconfigured row.
func TestComputeEntitlements_UnknownTierFallsBackToFree(t *testing.T) {
	t.Parallel()

	sub := &OrgSubscription{PlanTier: "platinum_deluxe"}
	got := ComputeEntitlements(sub, nil)
	want := GetPlanLimits(domain.PlanFree)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("unknown tier should fall back to Free\n got:  %+v\n want: %+v", got, want)
	}
}

func TestApplySubscriptionAddOns_IgnoresLegacyJSONBPacks(t *testing.T) {
	t.Parallel()

	base := GetPlanLimits(domain.PlanPro)
	got := ApplySubscriptionAddOns(base, SubscriptionAddOns{})
	if !reflect.DeepEqual(got, base) {
		t.Errorf("legacy JSONB add-ons changed launch entitlements\n got:  %+v\n want: %+v", got, base)
	}
}

// TestComputeEntitlements_NilSubFallsBackToFree mirrors the unknown-tier
// behavior for a missing-subscription row.
func TestComputeEntitlements_NilSubFallsBackToFree(t *testing.T) {
	t.Parallel()

	got := ComputeEntitlements(nil, nil)
	want := GetPlanLimits(domain.PlanFree)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nil sub should fall back to Free\n got:  %+v\n want: %+v", got, want)
	}
}

// TestComputeEntitlements_EmptyAddonsMatchesPlanBaseline locks the no-addon
// path to the static catalog — guards against an empty []Addon slice
// accidentally mutating limits.
func TestComputeEntitlements_EmptyAddonsMatchesPlanBaseline(t *testing.T) {
	t.Parallel()

	for _, tier := range []domain.PlanTier{
		domain.PlanFree, domain.PlanStarter, domain.PlanPro,
		domain.PlanScale, domain.PlanBusiness, domain.PlanEnterprise,
	} {
		t.Run(string(tier), func(t *testing.T) {
			t.Parallel()
			sub := &OrgSubscription{PlanTier: string(tier)}
			got := ComputeEntitlements(sub, []Addon{})
			want := GetPlanLimits(tier)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("empty addons should match catalog baseline for %s\n got:  %+v\n want: %+v", tier, got, want)
			}
		})
	}
}

// TestComputeEntitlements_TableAddonsExtendRetention locks retention extension
// to the launch-active organization_addons path.
func TestComputeEntitlements_TableAddonsExtendRetention(t *testing.T) {
	t.Parallel()

	sub := &OrgSubscription{
		PlanTier: string(domain.PlanScale),
	}
	addons := []Addon{
		{AddonType: AddonHistory30d, Quantity: 1, Active: true},
	}

	got := ComputeEntitlements(sub, addons)

	// Scale retention = 60; +30 from table addon (history_30d) = 90.
	wantRetention := RetentionScale + 30
	if got.RetentionDays != wantRetention {
		t.Errorf("retention composition: got %d, want %d", got.RetentionDays, wantRetention)
	}
}
