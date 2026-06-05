package billing

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEffectiveLimits_NoAddons(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	result := EffectiveLimits(base, nil)
	assert.Equal(t, base.
		MaxConcurrentRuns, result.
		MaxConcurrentRuns)
	assert.Equal(t, base.
		MaxMembersPerOrg, result.
		MaxMembersPerOrg,
	)
	assert.Equal(t, base.
		RetentionDays, result.RetentionDays,
	)
}

func TestLaunchActiveAddonTypesExcludeRoadmapAddons(t *testing.T) {
	t.Parallel()

	active := []AddonType{
		AddonConcurrency100,
		AddonHistory30d,
		AddonEnvironments5,
	}
	for _, addonType := range active {
		require.True(t, IsLaunchActiveAddonType(addonType))
	}

	roadmap := []AddonType{
		AddonComplianceArchive,
		AddonDedicatedWorkers,
	}
	for _, addonType := range roadmap {
		require.False(t,
			IsLaunchActiveAddonType(addonType))
	}
}

func TestAddonPacksDerivedFromGeneratedCatalog(t *testing.T) {
	t.Parallel()
	require.Len(t, AddonPacks,

		len(AddonCatalogs))

	for _, addonType := range AddonCatalogOrder {
		catalog, ok := AddonCatalogs[addonType]
		require.True(t, ok)

		pack, ok := AddonPacks[addonType]
		require.True(t, ok)
		require.False(t,
			pack.
				DisplayName != catalog.
				DisplayName ||
				pack.LookupKey != catalog.LookupKey ||
				pack.PackSize !=
					catalog.PackSize || pack.PriceCents !=
				catalog.PriceCents || pack.MaxTotal !=
				catalog.MaxTotal)
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
	assert.Equal(t, want,

		result.MaxConcurrentRuns,
	)
}

func TestEffectiveLimits_MultiplePacksStack(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrency100, Quantity: 3, Active: true},
	}

	result := EffectiveLimits(base, addons)
	want := base.MaxConcurrentRuns + 300
	assert.Equal(t, want,

		result.MaxConcurrentRuns,
	)

	// 3 packs x 100
}

func TestEffectiveLimits_Environments5Pack(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonEnvironments5, Quantity: 1, Active: true},
	}

	result := EffectiveLimits(base, addons)
	want := base.MaxEnvironments + 5
	assert.Equal(t, want,

		result.MaxEnvironments,
	)
}

func TestEffectiveLimits_History30dPack(t *testing.T) {
	t.Parallel()

	// Scale has RetentionScale days. One pack adds 30 days.
	scale := GetPlanLimits(domain.PlanScale)
	result := EffectiveLimits(scale, []Addon{
		{AddonType: AddonHistory30d, Quantity: 1, Active: true},
	})
	want1Pack := RetentionScale + 30
	assert.Equal(t, want1Pack,

		result.RetentionDays,
	)

	// Two packs stack additively.
	business := GetPlanLimits(domain.PlanBusiness)
	result = EffectiveLimits(business, []Addon{
		{AddonType: AddonHistory30d, Quantity: 2, Active: true},
	})
	want2Pack := business.RetentionDays + 60
	assert.Equal(t, want2Pack,

		result.RetentionDays,
	)
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
	require.Equal(t, 365,
		maxTotal)
	require.LessOrEqual(t, result.RetentionDays,
		maxTotal,
	)

	want := scale.RetentionDays + ((maxTotal-scale.RetentionDays)/AddonPacks[AddonHistory30d].PackSize)*AddonPacks[AddonHistory30d].PackSize
	require.Equal(t,
		want,
		result.RetentionDays)
}

func TestEffectiveLimits_ComplianceArchiveLaunchRoadmapNoEffect(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanScale)
	require.False(t,
		base.
			HasSIEMExport)

	result := EffectiveLimits(base, []Addon{
		{AddonType: AddonComplianceArchive, Quantity: 1, Active: true},
	})
	assert.False(t, result.
		HasSIEMExport)
}

func TestEffectiveLimits_DedicatedWorkersLaunchRoadmapNoEffect(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanScale)
	require.False(t,
		base.
			HasDedicatedCompute)

	result := EffectiveLimits(base, []Addon{
		{AddonType: AddonDedicatedWorkers, Quantity: 1, Active: true},
	})
	assert.False(t, result.
		HasDedicatedCompute)
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
	assert.Equal(t, base.
		MaxConcurrentRuns+200,

		result.
			MaxConcurrentRuns)
	assert.Equal(t, base.
		MaxEnvironments+15, result.
		MaxEnvironments)
	assert.Equal(t, base.
		RetentionDays+30, result.
		RetentionDays)
}

func TestEffectiveLimits_NegativeQuantity_Ignored(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrency100, Quantity: -5, Active: true},
	}

	result := EffectiveLimits(base, addons)
	assert.Equal(t, base.
		MaxConcurrentRuns, result.
		MaxConcurrentRuns)
}

func TestEffectiveLimits_UnknownAddonType_Ignored(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonType("nonexistent"), Quantity: 10, Active: true},
	}

	result := EffectiveLimits(base, addons)
	assert.False(t, result.
		MaxConcurrentRuns !=
		base.
			MaxConcurrentRuns || result.MaxEnvironments != base.
		MaxEnvironments ||
		result.RetentionDays != base.RetentionDays,
	)
}

func TestEffectiveLimits_InactiveAddons_NotApplied(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrency100, Quantity: 5, Active: false},
		{AddonType: AddonEnvironments5, Quantity: 10, Active: false},
	}

	result := EffectiveLimits(base, addons)
	assert.Equal(t, base.
		MaxConcurrentRuns, result.
		MaxConcurrentRuns)
	assert.Equal(t, base.
		MaxEnvironments, result.
		MaxEnvironments,
	)
}

func TestEffectiveLimits_UnlimitedNotModified(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanEnterprise)
	addons := []Addon{
		{AddonType: AddonConcurrency100, Quantity: 10, Active: true},
		{AddonType: AddonEnvironments5, Quantity: 10, Active: true},
	}

	result := EffectiveLimits(base, addons)
	assert.Equal(t, -1,
		result.MaxConcurrentRuns,
	)
	assert.Equal(t, -1,
		result.MaxEnvironments)
}

func TestIsValidAddonType(t *testing.T) {
	t.Parallel()

	for _, at := range AllAddonTypes() {
		assert.True(t, IsValidAddonType(at))
	}
	assert.False(t, IsValidAddonType(AddonType("nonexistent")))
	assert.False(t, IsValidAddonType(AddonType("")))
}

func TestAllAddonTypes_Count(t *testing.T) {
	t.Parallel()
	types := AllAddonTypes()
	assert.Len(t, types,

		5)
}

func TestAddonPacks_AllDefined(t *testing.T) {
	t.Parallel()
	for _, at := range AllAddonTypes() {
		if _, ok := AddonPacks[at]; !ok {
			assert.Failf(t, "test failure",

				"missing AddonPacks entry for %q", at)
		}
	}
}

func TestAddonPacks_SellableMetadataMatchesLaunchStatus(t *testing.T) {
	t.Parallel()
	for at, pack := range AddonPacks {
		assert.Positive(t, pack.
			PackSize)
		assert.Equal(t, at,

			pack.Type)
		assert.NotEmpty(t,

			pack.DisplayName)

		if IsLaunchActiveAddonType(at) {
			assert.Positive(t, pack.
				PriceCents)
			assert.NotEmpty(t,

				pack.LookupKey)

			continue
		}
		assert.Equal(t, 0,

			pack.PriceCents)
		assert.Empty(t, pack.LookupKey)
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
		assert.False(t, result.
			MaxConcurrentRuns < base.
			MaxConcurrentRuns && base.MaxConcurrentRuns != -1,
		)

		// Base limits should never decrease.
	})
}
