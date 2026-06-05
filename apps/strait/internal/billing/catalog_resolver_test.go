package billing

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogResolver_TierForLookupKey(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	cases := []struct {
		key  string
		want domain.PlanTier
	}{
		{"strait_free_monthly", domain.PlanFree},
		{"strait_starter_monthly", domain.PlanStarter},
		{"strait_starter_annual", domain.PlanStarter},
		{"strait_pro_monthly", domain.PlanPro},
		{"strait_pro_annual", domain.PlanPro},
		{"strait_scale_monthly", domain.PlanScale},
		{"strait_scale_annual", domain.PlanScale},
		{"strait_business_monthly", domain.PlanBusiness},
		{"strait_business_annual", domain.PlanBusiness},
	}

	for _, c := range cases {
		got, ok := r.TierForLookupKey(c.key)
		assert.False(t, !ok ||
			got != c.want)

	}
}

func TestCatalogResolver_TierForLookupKey_Unmapped(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	for _, key := range []string{"", "unknown_key", "strait_free_annual" /* free has no annual */} {
		if _, ok := r.TierForLookupKey(key); ok {
			assert.Failf(t, "test failure",

				"TierForLookupKey(%q) should be unmapped", key)
		}
	}
}

func TestCatalogResolver_TierForLookupKey_EnterpriseUnmapped(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	if _, ok := r.TierForLookupKey("strait_enterprise_monthly"); ok {
		assert.Fail(t,

			"Enterprise has no lookup keys (custom-quoted); should be unmapped")
	}
}

func TestCatalogResolver_AddonForLookupKey(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	cases := []struct {
		key  string
		want AddonType
	}{
		{"strait_addon_concurrency_100", AddonConcurrency100},
		{"strait_addon_history_30d", AddonHistory30d},
		{"strait_addon_environments_5", AddonEnvironments5},
	}

	for _, c := range cases {
		got, ok := r.AddonForLookupKey(c.key)
		assert.False(t, !ok ||
			got != c.want)

	}
}

func TestCatalogResolver_RoadmapAddonLookupKeysUnmapped(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	for _, key := range []string{
		"strait_addon_compliance_archive",
		"strait_addon_dedicated_pool",
	} {
		if _, ok := r.AddonForLookupKey(key); ok {
			assert.Failf(t, "test failure",

				"roadmap addon lookup key %q must not resolve as sellable addon", key)
		}
		assert.False(t, r.
			IsAddonLookupKey(key))

	}
}

func TestCatalogResolver_IsAddonLookupKey(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()
	assert.True(t, r.
		IsAddonLookupKey("strait_addon_concurrency_100"))
	assert.False(t, r.
		IsAddonLookupKey("strait_pro_monthly"))
	assert.False(t, r.
		IsAddonLookupKey("strait_addon_log_drain_10gb"))
	assert.False(t, r.
		IsAddonLookupKey(""))

}

func TestCatalogResolver_NilSafe(t *testing.T) {
	t.Parallel()
	var r *CatalogResolver

	if _, ok := r.TierForLookupKey("anything"); ok {
		assert.Fail(t,

			"nil resolver must not resolve a tier")
	}
	if _, ok := r.AddonForLookupKey("anything"); ok {
		assert.Fail(t,

			"nil resolver must not resolve an addon")
	}
	assert.False(t, r.
		IsAddonLookupKey("anything"),
	)
	assert.False(t, r.
		TierCount() != 0 || r.AddonCount() != 0)

}

func TestCatalogResolver_Counts(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()
	assert.EqualValues(t, 9,

		r.TierCount())
	assert.EqualValues(t, 3,

		r.AddonCount())

	// 4 tiers (Starter, Pro, Scale, Business) have monthly + annual = 8.
	// Free has only monthly = 1. Enterprise has none. Total = 9.

	// Only launch-active addons resolve as sellable Stripe addon lookup keys.

}

func TestCatalogResolver_BusinessTier(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	got, ok := r.TierForLookupKey("strait_business_monthly")
	require.True(t, ok)
	assert.Equal(t, domain.
		PlanBusiness, got)

	gotAnnual, ok := r.TierForLookupKey("strait_business_annual")
	require.True(t, ok)
	assert.Equal(t, domain.
		PlanBusiness, gotAnnual,
	)

}
