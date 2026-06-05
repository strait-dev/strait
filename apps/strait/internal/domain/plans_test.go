package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			assert.Equal(t, tt.want, tt.tier.IsValid())
		})
	}
}

func TestAllPlanTiers(t *testing.T) {
	t.Parallel()
	tiers := AllPlanTiers()
	require.Len(t, tiers,

		6)

	expected := []PlanTier{PlanFree, PlanStarter, PlanPro, PlanScale, PlanBusiness, PlanEnterprise}
	for i, tier := range tiers {
		assert.Equal(t,
			expected[i], tier)
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
			assert.Equal(t, tt.want, tt.tier.Rank())
		})
	}

	// Verify monotonically increasing ranks.
	tiers := AllPlanTiers()
	for i := 1; i < len(tiers); i++ {
		assert.Greater(t,
			tiers[i].Rank(), tiers[i-1].Rank())
	}
}
