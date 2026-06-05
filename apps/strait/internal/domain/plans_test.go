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
