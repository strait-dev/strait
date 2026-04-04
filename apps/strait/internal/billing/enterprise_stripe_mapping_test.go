package billing

import (
	"testing"

	"strait/internal/domain"
)

func TestEnterpriseMapping_StarterPrice(t *testing.T) {
	t.Parallel()
	m := NewStripeMappingFromOptions(
		WithEnterpriseStarterPrice("price_ent_starter"),
	)
	tier, ok := m.TierForPrice("price_ent_starter")
	if !ok {
		t.Fatal("expected true for enterprise starter price")
	}
	if tier != domain.PlanEnterprise {
		t.Errorf("tier = %s, want enterprise", tier)
	}
}

func TestEnterpriseMapping_GrowthPrice(t *testing.T) {
	t.Parallel()
	m := NewStripeMappingFromOptions(
		WithEnterpriseGrowthPrice("price_ent_growth"),
	)
	tier, ok := m.TierForPrice("price_ent_growth")
	if !ok {
		t.Fatal("expected true for enterprise growth price")
	}
	if tier != domain.PlanEnterprise {
		t.Errorf("tier = %s, want enterprise", tier)
	}
}

func TestEnterpriseMapping_LargePrice(t *testing.T) {
	t.Parallel()
	m := NewStripeMappingFromOptions(
		WithEnterpriseLargePrice("price_ent_large"),
	)
	tier, ok := m.TierForPrice("price_ent_large")
	if !ok {
		t.Fatal("expected true for enterprise large price")
	}
	if tier != domain.PlanEnterprise {
		t.Errorf("tier = %s, want enterprise", tier)
	}
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
		if !ok {
			t.Errorf("expected true for %q", priceID)
		}
		if tier != domain.PlanEnterprise {
			t.Errorf("tier for %q = %s, want enterprise", priceID, tier)
		}
	}
}

func TestEnterpriseMapping_EmptyPriceIDs(t *testing.T) {
	t.Parallel()
	m := NewStripeMappingFromOptions(
		WithEnterpriseStarterPrice(""),
		WithEnterpriseGrowthPrice(""),
		WithEnterpriseLargePrice(""),
	)
	if m.PriceCount() != 0 {
		t.Errorf("expected 0 prices, got %d", m.PriceCount())
	}
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
		t.Errorf("starter_m: ok=%v, tier=%s", ok, tier)
	}
	// Check pro.
	if tier, ok := m.TierForPrice("pro_m"); !ok || tier != domain.PlanPro {
		t.Errorf("pro_m: ok=%v, tier=%s", ok, tier)
	}
	// Check scale.
	if tier, ok := m.TierForPrice("scale_m"); !ok || tier != domain.PlanScale {
		t.Errorf("scale_m: ok=%v, tier=%s", ok, tier)
	}
	// Check enterprise.
	if tier, ok := m.TierForPrice("ent_g"); !ok || tier != domain.PlanEnterprise {
		t.Errorf("ent_g: ok=%v, tier=%s", ok, tier)
	}
}

func TestEnterpriseMapping_PriceCount(t *testing.T) {
	t.Parallel()
	m := NewStripeMappingFromOptions(
		WithStarterPrices("s_m", "s_y"),
		WithEnterpriseStarterPrice("e_s"),
		WithEnterpriseGrowthPrice("e_g"),
	)
	if got := m.PriceCount(); got != 4 {
		t.Errorf("PriceCount = %d, want 4", got)
	}
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
		if ok != tt.wantOK {
			t.Errorf("EnterpriseTierForPrice(%q) ok = %v, want %v", tt.priceID, ok, tt.wantOK)
		}
		if tier != tt.want {
			t.Errorf("EnterpriseTierForPrice(%q) = %q, want %q", tt.priceID, tier, tt.want)
		}
	}
}
