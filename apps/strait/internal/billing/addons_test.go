package billing

import (
	"testing"

	"strait/internal/domain"
)

func TestEffectiveLimits_NoAddons(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	result := EffectiveLimits(base, nil)

	if result.MaxConcurrentRuns != base.MaxConcurrentRuns {
		t.Errorf("MaxConcurrentRuns = %d, want %d", result.MaxConcurrentRuns, base.MaxConcurrentRuns)
	}
	if result.MaxMembersPerOrg != base.MaxMembersPerOrg {
		t.Errorf("MaxMembersPerOrg = %d, want %d", result.MaxMembersPerOrg, base.MaxMembersPerOrg)
	}
	if result.RetentionDays != base.RetentionDays {
		t.Errorf("RetentionDays = %d, want %d", result.RetentionDays, base.RetentionDays)
	}
}

func TestLaunchActiveAddonTypesExcludeRoadmapAddons(t *testing.T) {
	t.Parallel()

	active := []AddonType{
		AddonConcurrency100,
		AddonHistory30d,
		AddonEnvironments5,
	}
	for _, addonType := range active {
		if !IsLaunchActiveAddonType(addonType) {
			t.Fatalf("%s should be launch-active", addonType)
		}
	}

	roadmap := []AddonType{
		AddonComplianceArchive,
		AddonDedicatedWorkers,
	}
	for _, addonType := range roadmap {
		if IsLaunchActiveAddonType(addonType) {
			t.Fatalf("%s should remain roadmap-only at launch", addonType)
		}
	}
}

func TestAddonPacksDerivedFromGeneratedCatalog(t *testing.T) {
	t.Parallel()

	if len(AddonPacks) != len(AddonCatalogs) {
		t.Fatalf("AddonPacks len = %d, want generated catalog len %d", len(AddonPacks), len(AddonCatalogs))
	}
	for _, addonType := range AddonCatalogOrder {
		catalog, ok := AddonCatalogs[addonType]
		if !ok {
			t.Fatalf("generated catalog missing %s", addonType)
		}
		pack, ok := AddonPacks[addonType]
		if !ok {
			t.Fatalf("AddonPacks missing %s", addonType)
		}
		if pack.DisplayName != catalog.DisplayName ||
			pack.LookupKey != catalog.LookupKey ||
			pack.PackSize != catalog.PackSize ||
			pack.PriceCents != catalog.PriceCents ||
			pack.MaxTotal != catalog.MaxTotal {
			t.Fatalf("AddonPacks[%s] = %+v, want generated catalog %+v", addonType, pack, catalog)
		}
	}
}

func TestEffectiveLimits_Concurrency100Pack(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrency100, Quantity: 1, Active: true},
	}

	result := EffectiveLimits(base, addons)
	want := base.MaxConcurrentRuns + 100
	if result.MaxConcurrentRuns != want {
		t.Errorf("MaxConcurrentRuns = %d, want %d", result.MaxConcurrentRuns, want)
	}
}

func TestEffectiveLimits_MultiplePacksStack(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrency100, Quantity: 3, Active: true},
	}

	result := EffectiveLimits(base, addons)
	want := base.MaxConcurrentRuns + 300 // 3 packs x 100
	if result.MaxConcurrentRuns != want {
		t.Errorf("MaxConcurrentRuns = %d, want %d", result.MaxConcurrentRuns, want)
	}
}

func TestEffectiveLimits_Environments5Pack(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonEnvironments5, Quantity: 1, Active: true},
	}

	result := EffectiveLimits(base, addons)
	want := base.MaxEnvironments + 5
	if result.MaxEnvironments != want {
		t.Errorf("MaxEnvironments = %d, want %d", result.MaxEnvironments, want)
	}
}

func TestEffectiveLimits_History30dPack(t *testing.T) {
	t.Parallel()

	// Scale has RetentionScale days. One pack adds 30 days.
	scale := GetPlanLimits(domain.PlanScale)
	result := EffectiveLimits(scale, []Addon{
		{AddonType: AddonHistory30d, Quantity: 1, Active: true},
	})
	want1Pack := RetentionScale + 30
	if result.RetentionDays != want1Pack {
		t.Errorf("Scale + 1 history pack = %d, want %d", result.RetentionDays, want1Pack)
	}

	// Two packs stack additively.
	business := GetPlanLimits(domain.PlanBusiness)
	result = EffectiveLimits(business, []Addon{
		{AddonType: AddonHistory30d, Quantity: 2, Active: true},
	})
	want2Pack := business.RetentionDays + 60
	if result.RetentionDays != want2Pack {
		t.Errorf("Business + 2 history packs = %d, want %d", result.RetentionDays, want2Pack)
	}
}

func TestEffectiveLimits_History30dClampedToCatalogMaxTotal(t *testing.T) {
	t.Parallel()

	scale := GetPlanLimits(domain.PlanScale)
	scale.MaxAddonPacks = map[AddonType]int{
		AddonHistory30d: -1,
	}
	result := EffectiveLimits(scale, []Addon{
		{AddonType: AddonHistory30d, Quantity: 1000, Active: true},
	})
	maxTotal := AddonPacks[AddonHistory30d].MaxTotal
	if maxTotal != 365 {
		t.Fatalf("history add-on catalog MaxTotal = %d, want 365", maxTotal)
	}
	if result.RetentionDays > maxTotal {
		t.Fatalf("retention with excessive history packs = %d, want <= %d", result.RetentionDays, maxTotal)
	}
	want := scale.RetentionDays + ((maxTotal-scale.RetentionDays)/AddonPacks[AddonHistory30d].PackSize)*AddonPacks[AddonHistory30d].PackSize
	if result.RetentionDays != want {
		t.Fatalf("retention with excessive history packs = %d, want %d", result.RetentionDays, want)
	}
}

func TestEffectiveLimits_ComplianceArchiveLaunchRoadmapNoEffect(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanScale)
	if base.HasSIEMExport {
		t.Fatalf("precondition: Scale should not have SIEM export")
	}

	result := EffectiveLimits(base, []Addon{
		{AddonType: AddonComplianceArchive, Quantity: 1, Active: true},
	})
	if result.HasSIEMExport {
		t.Error("ComplianceArchive is roadmap at launch and must not enable HasSIEMExport")
	}
}

func TestEffectiveLimits_DedicatedWorkersLaunchRoadmapNoEffect(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanScale)
	if base.HasDedicatedCompute {
		t.Fatalf("precondition: Pro should not have dedicated compute")
	}

	result := EffectiveLimits(base, []Addon{
		{AddonType: AddonDedicatedWorkers, Quantity: 1, Active: true},
	})
	if result.HasDedicatedCompute {
		t.Error("DedicatedWorkers is roadmap at launch and must not enable HasDedicatedCompute")
	}
}

func TestEffectiveLimits_MixedAddons(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanScale)
	addons := []Addon{
		{AddonType: AddonConcurrency100, Quantity: 2, Active: true},
		{AddonType: AddonEnvironments5, Quantity: 3, Active: true},
		{AddonType: AddonHistory30d, Quantity: 1, Active: true},
	}

	result := EffectiveLimits(base, addons)

	if result.MaxConcurrentRuns != base.MaxConcurrentRuns+200 {
		t.Errorf("MaxConcurrentRuns = %d, want %d", result.MaxConcurrentRuns, base.MaxConcurrentRuns+200)
	}
	if result.MaxEnvironments != base.MaxEnvironments+15 {
		t.Errorf("MaxEnvironments = %d, want %d", result.MaxEnvironments, base.MaxEnvironments+15)
	}
	if result.RetentionDays != base.RetentionDays+30 {
		t.Errorf("RetentionDays = %d, want %d", result.RetentionDays, base.RetentionDays+30)
	}
}

func TestEffectiveLimits_NegativeQuantity_Ignored(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrency100, Quantity: -5, Active: true},
	}

	result := EffectiveLimits(base, addons)
	if result.MaxConcurrentRuns != base.MaxConcurrentRuns {
		t.Errorf("negative quantity should be ignored: got %d, want %d",
			result.MaxConcurrentRuns, base.MaxConcurrentRuns)
	}
}

func TestEffectiveLimits_UnknownAddonType_Ignored(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonType("nonexistent"), Quantity: 10, Active: true},
	}

	result := EffectiveLimits(base, addons)
	if result.MaxConcurrentRuns != base.MaxConcurrentRuns ||
		result.MaxEnvironments != base.MaxEnvironments ||
		result.RetentionDays != base.RetentionDays {
		t.Error("unknown addon type should have no effect on limits")
	}
}

func TestEffectiveLimits_InactiveAddons_NotApplied(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrency100, Quantity: 5, Active: false},
		{AddonType: AddonEnvironments5, Quantity: 10, Active: false},
	}

	result := EffectiveLimits(base, addons)
	if result.MaxConcurrentRuns != base.MaxConcurrentRuns {
		t.Errorf("inactive addon applied: MaxConcurrentRuns = %d, want %d",
			result.MaxConcurrentRuns, base.MaxConcurrentRuns)
	}
	if result.MaxEnvironments != base.MaxEnvironments {
		t.Errorf("inactive addon applied: MaxEnvironments = %d, want %d",
			result.MaxEnvironments, base.MaxEnvironments)
	}
}

func TestEffectiveLimits_UnlimitedNotModified(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanEnterprise)
	addons := []Addon{
		{AddonType: AddonConcurrency100, Quantity: 10, Active: true},
		{AddonType: AddonEnvironments5, Quantity: 10, Active: true},
	}

	result := EffectiveLimits(base, addons)
	if result.MaxConcurrentRuns != -1 {
		t.Errorf("enterprise concurrent runs should stay unlimited, got %d", result.MaxConcurrentRuns)
	}
	if result.MaxEnvironments != -1 {
		t.Errorf("enterprise environments should stay unlimited, got %d", result.MaxEnvironments)
	}
}

func TestIsValidAddonType(t *testing.T) {
	t.Parallel()

	for _, at := range AllAddonTypes() {
		if !IsValidAddonType(at) {
			t.Errorf("IsValidAddonType(%q) = false, want true", at)
		}
	}

	if IsValidAddonType(AddonType("nonexistent")) {
		t.Error("IsValidAddonType(nonexistent) = true, want false")
	}
	if IsValidAddonType(AddonType("")) {
		t.Error("IsValidAddonType(\"\") = true, want false")
	}
}

func TestAllAddonTypes_Count(t *testing.T) {
	t.Parallel()
	types := AllAddonTypes()
	if len(types) != 5 {
		t.Errorf("AllAddonTypes() count = %d, want 5", len(types))
	}
}

func TestAddonPacks_AllDefined(t *testing.T) {
	t.Parallel()
	for _, at := range AllAddonTypes() {
		if _, ok := AddonPacks[at]; !ok {
			t.Errorf("missing AddonPacks entry for %q", at)
		}
	}
}

func TestAddonPacks_SellableMetadataMatchesLaunchStatus(t *testing.T) {
	t.Parallel()
	for at, pack := range AddonPacks {
		if pack.PackSize <= 0 {
			t.Errorf("AddonPacks[%q].PackSize = %d, want > 0", at, pack.PackSize)
		}
		if pack.Type != at {
			t.Errorf("AddonPacks[%q].Type = %q, want %q", at, pack.Type, at)
		}
		if pack.DisplayName == "" {
			t.Errorf("AddonPacks[%q].DisplayName is empty", at)
		}
		if IsLaunchActiveAddonType(at) {
			if pack.PriceCents <= 0 {
				t.Errorf("active AddonPacks[%q].PriceCents = %d, want > 0", at, pack.PriceCents)
			}
			if pack.LookupKey == "" {
				t.Errorf("active AddonPacks[%q].LookupKey is empty", at)
			}
			continue
		}
		if pack.PriceCents != 0 {
			t.Errorf("roadmap AddonPacks[%q].PriceCents = %d, want 0", at, pack.PriceCents)
		}
		if pack.LookupKey != "" {
			t.Errorf("roadmap AddonPacks[%q].LookupKey = %q, want empty", at, pack.LookupKey)
		}
	}
}

func FuzzEffectiveLimits(f *testing.F) {
	f.Add("concurrency_100", 1, true)
	f.Add("environments_5", 5, true)
	f.Add("history_30d", 3, true)
	f.Add("nonexistent", 10, true)
	f.Add("", 0, false)
	f.Add("concurrency_100", -1, true)

	f.Fuzz(func(t *testing.T, addonType string, quantity int, active bool) {
		base := GetPlanLimits(domain.PlanPro)
		addons := []Addon{
			{AddonType: AddonType(addonType), Quantity: quantity, Active: active},
		}
		// Should never panic.
		result := EffectiveLimits(base, addons)
		// Base limits should never decrease.
		if result.MaxConcurrentRuns < base.MaxConcurrentRuns && base.MaxConcurrentRuns != -1 {
			t.Errorf("MaxConcurrentRuns decreased: %d < %d", result.MaxConcurrentRuns, base.MaxConcurrentRuns)
		}
	})
}
