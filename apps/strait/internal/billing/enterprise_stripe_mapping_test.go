package billing

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseMapping_StarterPrice(t *testing.T) {
	t.Parallel()
	m := NewStripeMappingFromOptions(
		WithEnterpriseStarterPrice("price_ent_starter"),
	)
	tier, ok := m.TierForPrice("price_ent_starter")
	require.True(t, ok)
	assert.Equal(t, domain.
		PlanEnterprise, tier)
}

func TestEnterpriseMapping_GrowthPrice(t *testing.T) {
	t.Parallel()
	m := NewStripeMappingFromOptions(
		WithEnterpriseGrowthPrice("price_ent_growth"),
	)
	tier, ok := m.TierForPrice("price_ent_growth")
	require.True(t, ok)
	assert.Equal(t, domain.
		PlanEnterprise, tier)
}

func TestEnterpriseMapping_LargePrice(t *testing.T) {
	t.Parallel()
	m := NewStripeMappingFromOptions(
		WithEnterpriseLargePrice("price_ent_large"),
	)
	tier, ok := m.TierForPrice("price_ent_large")
	require.True(t, ok)
	assert.Equal(t, domain.
		PlanEnterprise, tier)
}

func TestEnterpriseMapping_AllThreePrices(t *testing.T) {
	t.Parallel()
	m := NewStripeMappingFromOptions(
		WithEnterpriseStarterPrice("price_s"),
		WithEnterpriseGrowthPrice("price_g"),
		WithEnterpriseLargePrice("price_l"),
	)
	for _, priceID := range []string{"price_s", "price_g", "price_l"} {
		tier, ok := m.TierForPrice(priceID)
		assert.True(t, ok)
		assert.Equal(t, domain.
			PlanEnterprise, tier)
	}
}

func TestEnterpriseMapping_EmptyPriceIDs(t *testing.T) {
	t.Parallel()
	m := NewStripeMappingFromOptions(
		WithEnterpriseStarterPrice(""),
		WithEnterpriseGrowthPrice(""),
		WithEnterpriseLargePrice(""),
	)
	assert.Equal(t, 0,

		m.PriceCount())
}

func TestEnterpriseMapping_MixedWithOtherPlans(t *testing.T) {
	t.Parallel()
	m := NewStripeMappingFromOptions(
		WithStarterPrices("starter_m", "starter_y"),
		WithProPrices("pro_m", "pro_y"),
		WithScalePrices("scale_m", "scale_y"),
		WithEnterpriseStarterPrice("ent_s"),
		WithEnterpriseGrowthPrice("ent_g"),
		WithEnterpriseLargePrice("ent_l"),
	)

	// Check starter.
	if tier, ok := m.TierForPrice("starter_m"); !ok || tier != domain.PlanStarter {
		assert.Failf(t, "test failure",

			"starter_m: ok=%v, tier=%s", ok, tier)
	}
	// Check pro.
	if tier, ok := m.TierForPrice("pro_m"); !ok || tier != domain.PlanPro {
		assert.Failf(t, "test failure",

			"pro_m: ok=%v, tier=%s", ok, tier)
	}
	// Check scale.
	if tier, ok := m.TierForPrice("scale_m"); !ok || tier != domain.PlanScale {
		assert.Failf(t, "test failure",

			"scale_m: ok=%v, tier=%s", ok, tier)
	}
	// Check enterprise.
	if tier, ok := m.TierForPrice("ent_g"); !ok || tier != domain.PlanEnterprise {
		assert.Failf(t, "test failure",

			"ent_g: ok=%v, tier=%s", ok, tier)
	}
}

func TestEnterpriseMapping_PriceCount(t *testing.T) {
	t.Parallel()
	m := NewStripeMappingFromOptions(
		WithStarterPrices("s_m", "s_y"),
		WithEnterpriseStarterPrice("e_s"),
		WithEnterpriseGrowthPrice("e_g"),
	)
	assert.Equal(t, 4,

		m.PriceCount())
}

func TestEnterpriseTierForPrice_Mapping(t *testing.T) {
	t.Parallel()
	// Register test prices.
	RegisterEnterprisePriceTier("test_starter", EnterpriseTierStarter)
	RegisterEnterprisePriceTier("test_growth", EnterpriseTierGrowth)
	RegisterEnterprisePriceTier("test_large", EnterpriseTierLarge)

	tests := []struct {
		priceID string
		want    EnterpriseTier
		wantOK  bool
	}{
		{"test_starter", EnterpriseTierStarter, true},
		{"test_growth", EnterpriseTierGrowth, true},
		{"test_large", EnterpriseTierLarge, true},
		{"unknown", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		tier, ok := EnterpriseTierForPrice(tt.priceID)
		assert.Equal(t, tt.
			wantOK, ok)
		assert.Equal(t, tt.
			want, tier)
	}
}
