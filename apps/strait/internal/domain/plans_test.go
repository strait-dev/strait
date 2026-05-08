package domain

import (
	"testing"
)

func TestPlanTier_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier PlanTier
		want bool
	}{
		{PlanFree, true},
		{PlanStarter, true},
		{PlanPro, true},
		{PlanScale, true},
		{PlanEnterprise, true},
		{PlanTier("unknown"), false},
		{PlanTier(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			t.Parallel()
			if got := tt.tier.IsValid(); got != tt.want {
				t.Errorf("PlanTier(%q).IsValid() = %v, want %v", tt.tier, got, tt.want)
			}
		})
	}
}

func TestGetPlanConfig(t *testing.T) {
	t.Parallel()

	t.Run("free_plan", func(t *testing.T) {
		t.Parallel()
		cfg := GetPlanConfig(PlanFree)
		if cfg.Tier != PlanFree {
			t.Errorf("expected tier=free, got %q", cfg.Tier)
		}
		if cfg.MaxRegions != 1 {
			t.Errorf("expected MaxRegions=1, got %d", cfg.MaxRegions)
		}
		if cfg.MultiRegion {
			t.Error("expected MultiRegion=false for free plan")
		}
		if len(cfg.AllowedRegions) != 1 || cfg.AllowedRegions[0] != "iad" {
			t.Errorf("expected AllowedRegions=[iad], got %v", cfg.AllowedRegions)
		}
	})

	t.Run("starter_plan", func(t *testing.T) {
		t.Parallel()
		cfg := GetPlanConfig(PlanStarter)
		if cfg.MaxRegions != 1 {
			t.Errorf("expected MaxRegions=1, got %d", cfg.MaxRegions)
		}
		if cfg.MultiRegion {
			t.Error("expected MultiRegion=false for starter plan")
		}
		if len(cfg.AllowedRegions) != 6 {
			t.Errorf("expected 6 AllowedRegions, got %d", len(cfg.AllowedRegions))
		}
	})

	t.Run("pro_plan", func(t *testing.T) {
		t.Parallel()
		cfg := GetPlanConfig(PlanPro)
		if cfg.MaxRegions != 3 {
			t.Errorf("expected MaxRegions=3, got %d", cfg.MaxRegions)
		}
		if !cfg.MultiRegion {
			t.Error("expected MultiRegion=true for pro plan")
		}
		if cfg.AllowedRegions != nil {
			t.Errorf("expected nil AllowedRegions (all regions), got %v", cfg.AllowedRegions)
		}
	})

	t.Run("scale_plan", func(t *testing.T) {
		t.Parallel()
		cfg := GetPlanConfig(PlanScale)
		if cfg.MaxRegions != 5 {
			t.Errorf("expected MaxRegions=5, got %d", cfg.MaxRegions)
		}
		if !cfg.MultiRegion {
			t.Error("expected MultiRegion=true for scale plan")
		}
		if cfg.AllowedRegions != nil {
			t.Errorf("expected nil AllowedRegions (all regions), got %v", cfg.AllowedRegions)
		}
	})

	t.Run("enterprise_plan", func(t *testing.T) {
		t.Parallel()
		cfg := GetPlanConfig(PlanEnterprise)
		if cfg.MaxRegions != 5 {
			t.Errorf("expected MaxRegions=5, got %d", cfg.MaxRegions)
		}
		if !cfg.MultiRegion {
			t.Error("expected MultiRegion=true for enterprise plan")
		}
	})

	t.Run("unknown_falls_back_to_free", func(t *testing.T) {
		t.Parallel()
		cfg := GetPlanConfig(PlanTier("unknown"))
		if cfg.Tier != PlanFree {
			t.Errorf("expected fallback to free, got %q", cfg.Tier)
		}
	})
}

func TestIsRegionAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tier   PlanTier
		region string
		want   bool
	}{
		{"free_iad", PlanFree, "iad", true},
		{"free_lhr", PlanFree, "lhr", false},
		{"free_nrt", PlanFree, "nrt", false},
		{"starter_iad", PlanStarter, "iad", true},
		{"starter_ord", PlanStarter, "ord", true},
		{"starter_lax", PlanStarter, "lax", true},
		{"starter_lhr", PlanStarter, "lhr", true},
		{"starter_fra", PlanStarter, "fra", true},
		{"starter_sin", PlanStarter, "sin", true},
		{"starter_nrt", PlanStarter, "nrt", false},
		{"starter_syd", PlanStarter, "syd", false},
		{"starter_hkg", PlanStarter, "hkg", false},
		{"pro_iad", PlanPro, "iad", true},
		{"pro_hkg", PlanPro, "hkg", true},
		{"pro_any", PlanPro, "jnb", true},
		{"scale_iad", PlanScale, "iad", true},
		{"scale_any", PlanScale, "hkg", true},
		{"enterprise_iad", PlanEnterprise, "iad", true},
		{"enterprise_any", PlanEnterprise, "bog", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsRegionAllowed(tt.tier, tt.region); got != tt.want {
				t.Errorf("IsRegionAllowed(%q, %q) = %v, want %v", tt.tier, tt.region, got, tt.want)
			}
		})
	}
}

func TestAllPlanTiers(t *testing.T) {
	t.Parallel()
	tiers := AllPlanTiers()
	if len(tiers) != 6 {
		t.Fatalf("expected 6 plan tiers, got %d", len(tiers))
	}
	expected := []PlanTier{PlanFree, PlanStarter, PlanPro, PlanScale, PlanBusiness, PlanEnterprise}
	for i, tier := range tiers {
		if tier != expected[i] {
			t.Errorf("AllPlanTiers()[%d] = %q, want %q", i, tier, expected[i])
		}
	}
}

func TestPlanTierRank(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier PlanTier
		want int
	}{
		{PlanFree, 0},
		{PlanStarter, 1},
		{PlanPro, 2},
		{PlanScale, 3},
		{PlanBusiness, 4},
		{PlanEnterprise, 5},
		{PlanTier("unknown"), 0},
		{PlanTier(""), 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			t.Parallel()
			if got := tt.tier.Rank(); got != tt.want {
				t.Errorf("PlanTier(%q).Rank() = %d, want %d", tt.tier, got, tt.want)
			}
		})
	}

	// Verify monotonically increasing ranks.
	tiers := AllPlanTiers()
	for i := 1; i < len(tiers); i++ {
		if tiers[i].Rank() <= tiers[i-1].Rank() {
			t.Errorf("Rank(%q)=%d should be > Rank(%q)=%d",
				tiers[i], tiers[i].Rank(), tiers[i-1], tiers[i-1].Rank())
		}
	}
}

func TestAllPlanConfigs_IncludesScale(t *testing.T) {
	t.Parallel()
	configs := AllPlanConfigs()
	if _, ok := configs[PlanScale]; !ok {
		t.Error("AllPlanConfigs() missing Scale plan config")
	}
	if len(configs) != 6 {
		t.Errorf("expected 6 plan configs, got %d", len(configs))
	}
}
