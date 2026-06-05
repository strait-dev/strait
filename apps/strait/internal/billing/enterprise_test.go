package billing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Enterprise tier constants.

func TestEnterpriseTierConstants(t *testing.T) {
	t.Parallel()
	tiers := AllEnterpriseTiers()
	require.Len(t, tiers,
		3)
	assert.Equal(t, EnterpriseTierStarter,

		tiers[0])
	assert.Equal(t, EnterpriseTierGrowth,

		tiers[1])
	assert.Equal(t, EnterpriseTierLarge,

		tiers[2])

}

func TestIsValidEnterpriseTier_ValidTiers(t *testing.T) {
	t.Parallel()
	for _, tier := range AllEnterpriseTiers() {
		assert.True(t, IsValidEnterpriseTier(tier))

	}
}

func TestIsValidEnterpriseTier_InvalidTiers(t *testing.T) {
	t.Parallel()
	invalid := []EnterpriseTier{
		"", "free", "starter", "pro", "scale", "enterprise",
		"ENTERPRISE_STARTER", "Enterprise_Starter",
		"enterprise-starter", "random",
	}
	for _, tier := range invalid {
		assert.False(t, IsValidEnterpriseTier(tier))

	}
}

// Enterprise config completeness.

func TestEnterpriseConfigCompleteness(t *testing.T) {
	t.Parallel()
	for _, tier := range AllEnterpriseTiers() {
		cfg, ok := EnterpriseConfigs[tier]
		require.True(t, ok)
		assert.Equal(t, tier,
			cfg.
				Tier,
		)
		assert.NotEqual(t,
			"", cfg.
				DisplayName,
		)
		assert.False(t, cfg.
			AnnualCommitmentCents <=
			0)
		assert.False(t, cfg.
			MonthlyEquivalentCents <=
			0)
		assert.False(t, cfg.
			OverageDiscountPct <=
			0)
		assert.GreaterOrEqual(t,
			cfg.
				UptimeSLAPct, 99.0)
		assert.False(t, cfg.
			MaxDowntimeMinutes <=
			0)
		assert.False(t, cfg.
			SupportResponseP1 ==
			"" || cfg.SupportResponseP2 ==
			"" || cfg.SupportResponseP3 == "")

	}
}

func TestEnterpriseConfigValues_StarterTier(t *testing.T) {
	t.Parallel()
	cfg := EnterpriseConfigs[EnterpriseTierStarter]
	assert.EqualValues(t, 1_800_000,

		cfg.
			AnnualCommitmentCents)
	assert.EqualValues(t, 150_000,

		cfg.
			MonthlyEquivalentCents)
	assert.EqualValues(t, 1_500_000_000,

		cfg.PlatformFeeMicrousd)
	assert.EqualValues(t, 10,
		cfg.OverageDiscountPct,
	)
	assert.EqualValues(t, 99.9,
		cfg.
			UptimeSLAPct,
	)
	assert.EqualValues(t, 43.8,
		cfg.
			MaxDowntimeMinutes,
	)

}

func TestEnterpriseConfigValues_GrowthTier(t *testing.T) {
	t.Parallel()
	cfg := EnterpriseConfigs[EnterpriseTierGrowth]
	assert.EqualValues(t, 4_800_000,

		cfg.
			AnnualCommitmentCents)
	assert.EqualValues(t, 400_000,

		cfg.
			MonthlyEquivalentCents)
	assert.EqualValues(t, 4_000_000_000,

		cfg.PlatformFeeMicrousd)
	assert.EqualValues(t, 15,
		cfg.OverageDiscountPct,
	)
	assert.EqualValues(t, 99.95,
		cfg.
			UptimeSLAPct,
	)
	assert.EqualValues(t, 21.9,
		cfg.
			MaxDowntimeMinutes,
	)

}

func TestEnterpriseConfigValues_LargeTier(t *testing.T) {
	t.Parallel()
	cfg := EnterpriseConfigs[EnterpriseTierLarge]
	assert.EqualValues(t, 9_600_000,

		cfg.
			AnnualCommitmentCents)
	assert.EqualValues(t, 20,
		cfg.OverageDiscountPct,
	)
	assert.EqualValues(t, 99.95,
		cfg.
			UptimeSLAPct,
	)

}

func TestPlatformFee_ConsistentWithCommitment(t *testing.T) {
	t.Parallel()
	starter := EnterpriseConfigs[EnterpriseTierStarter]
	assert.Equal(t, starter.
		MonthlyEquivalentCents/
		100, starter.
		PlatformFeeMicrousd/1_000_000)

	growth := EnterpriseConfigs[EnterpriseTierGrowth]
	assert.Equal(t, growth.
		MonthlyEquivalentCents/
		100, growth.
		PlatformFeeMicrousd/1_000_000)

}

func TestGetEnterpriseConfig_ValidTiers(t *testing.T) {
	t.Parallel()
	for _, tier := range AllEnterpriseTiers() {
		cfg := GetEnterpriseConfig(tier)
		assert.Equal(t, tier,
			cfg.
				Tier,
		)

	}
}

func TestGetEnterpriseConfig_UnknownTierReturnsFallback(t *testing.T) {
	t.Parallel()
	cfg := GetEnterpriseConfig("unknown")
	assert.Equal(t, EnterpriseTierStarter,

		cfg.Tier)

}

// ApplyOverageDiscount tests.

func TestApplyOverageDiscount_AllTiers(t *testing.T) {
	t.Parallel()
	cost := int64(1_000_000_000) // $1,000

	tests := []struct {
		name     string
		discount int
		want     int64
	}{
		{"starter_10pct", 10, 900_000_000},
		{"growth_15pct", 15, 850_000_000},
		{"large_20pct", 20, 800_000_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ApplyOverageDiscount(cost, tt.discount)
			assert.Equal(t, tt.
				want,
				got,
			)

		})
	}
}

func TestApplyOverageDiscount_ZeroCost(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 0,
		ApplyOverageDiscount(0, 10))

}

func TestApplyOverageDiscount_ZeroDiscount(t *testing.T) {
	t.Parallel()
	cost := int64(500_000)
	assert.Equal(t, cost,
		ApplyOverageDiscount(cost, 0))

}

func TestApplyOverageDiscount_FullDiscount(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 0,
		ApplyOverageDiscount(1_000_000, 100))

}

// Contract validation tests.

func TestValidateEnterpriseContract_Valid(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c := &EnterpriseContract{
		OrgID:                 "org-1",
		EnterpriseTier:        EnterpriseTierStarter,
		AnnualCommitmentCents: 1_800_000,
		OverageDiscountPct:    10,
		ContractStartDate:     now,
		ContractEndDate:       now.AddDate(1, 0, 0),
		BillingCadence:        "annual",
	}
	require.NoError(t,
		ValidateEnterpriseContract(c))

}

func TestValidateEnterpriseContract_EmptyOrgID(t *testing.T) {
	t.Parallel()
	c := &EnterpriseContract{
		OrgID:                 "",
		EnterpriseTier:        EnterpriseTierStarter,
		AnnualCommitmentCents: 1_800_000,
		ContractStartDate:     time.Now(),
		ContractEndDate:       time.Now().AddDate(1, 0, 0),
		BillingCadence:        "annual",
	}
	require.Error(t,
		ValidateEnterpriseContract(c))

}

func TestValidateEnterpriseContract_InvalidTier(t *testing.T) {
	t.Parallel()
	c := &EnterpriseContract{
		OrgID:                 "org-1",
		EnterpriseTier:        "invalid",
		AnnualCommitmentCents: 1_800_000,
		ContractStartDate:     time.Now(),
		ContractEndDate:       time.Now().AddDate(1, 0, 0),
		BillingCadence:        "annual",
	}
	require.Error(t,
		ValidateEnterpriseContract(c))

}

func TestValidateEnterpriseContract_BelowMinCommitment(t *testing.T) {
	t.Parallel()
	c := &EnterpriseContract{
		OrgID:                 "org-1",
		EnterpriseTier:        EnterpriseTierStarter,
		AnnualCommitmentCents: 100_000, // $1K, below $18K min
		ContractStartDate:     time.Now(),
		ContractEndDate:       time.Now().AddDate(1, 0, 0),
		BillingCadence:        "annual",
	}
	require.Error(t,
		ValidateEnterpriseContract(c))

}

func TestValidateEnterpriseContract_EndBeforeStart(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c := &EnterpriseContract{
		OrgID:                 "org-1",
		EnterpriseTier:        EnterpriseTierStarter,
		AnnualCommitmentCents: 1_800_000,
		ContractStartDate:     now,
		ContractEndDate:       now.Add(-24 * time.Hour),
		BillingCadence:        "annual",
	}
	require.Error(t,
		ValidateEnterpriseContract(c))

}

func TestValidateEnterpriseContract_InvalidCadence(t *testing.T) {
	t.Parallel()
	c := &EnterpriseContract{
		OrgID:                 "org-1",
		EnterpriseTier:        EnterpriseTierStarter,
		AnnualCommitmentCents: 1_800_000,
		ContractStartDate:     time.Now(),
		ContractEndDate:       time.Now().AddDate(1, 0, 0),
		BillingCadence:        "monthly",
	}
	require.Error(t,
		ValidateEnterpriseContract(c))

}

func TestIsValidBillingCadence(t *testing.T) {
	t.Parallel()
	valid := []string{"annual", "quarterly"}
	for _, c := range valid {
		assert.True(t, IsValidBillingCadence(c))

	}
	invalid := []string{"", "monthly", "weekly", "daily", "ANNUAL"}
	for _, c := range invalid {
		assert.False(t, IsValidBillingCadence(c))

	}
}

// SLA credit calculation tests.

func TestCalculateSLACredit_AboveThreshold(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 0,
		CalculateSLACredit(99.95, EnterpriseStarterSLAPct))
	assert.EqualValues(t, 0,
		CalculateSLACredit(100.0, EnterpriseStarterSLAPct))

}

func TestCalculateSLACredit_AllTiers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		uptime float64
		want   int
	}{
		{99.5, 10},
		{99.0, 10},
		{97.0, 25},
		{95.0, 25},
		{92.0, 50},
		{90.0, 50},
		{85.0, 50},
		{0.0, 50},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.
			want,
			CalculateSLACredit(tt.uptime, EnterpriseStarterSLAPct))

	}
}

func TestCalculateSLACredit_ExactBoundary999(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 0,
		CalculateSLACredit(99.9, EnterpriseStarterSLAPct))

	// Exactly 99.9 is at the SLA threshold for Starter, so no credit.

}

func TestCalculateSLACredit_PerTierSLATarget(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 10,
		CalculateSLACredit(99.92, EnterpriseGrowthSLAPct))
	assert.EqualValues(t, 0,
		CalculateSLACredit(99.92, EnterpriseStarterSLAPct))
	assert.EqualValues(t, 0,
		CalculateSLACredit(99.95, EnterpriseGrowthSLAPct))
	assert.EqualValues(t, 10,
		CalculateSLACredit(99.94, EnterpriseGrowthSLAPct))
	assert.EqualValues(t, 10,
		CalculateSLACredit(99.93, EnterpriseLargeSLAPct))

	// Growth/Large tiers have 99.95% SLA, so 99.92% is below target and should get credit.

	// But 99.92% with Starter SLA (99.9%) should get 0 credit (above target).

	// 99.95% exactly at Growth SLA target -- no credit.

	// 99.94% just below Growth SLA -- should get credit.

	// Large tier with same SLA as Growth.

}

func TestCalculateSLACredit_CustomSLATarget(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 10,
		CalculateSLACredit(99.98, 99.99))
	assert.EqualValues(t, 0,
		CalculateSLACredit(99.99, 99.99))

	// Verify arbitrary SLA targets work (e.g. custom enterprise contracts).

}

func TestApplyOverageDiscount_NegativeCostReturnsZero(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 0,
		ApplyOverageDiscount(-100, 10))

}

func TestApplyOverageDiscount_BoundaryDiscount1(t *testing.T) {
	t.Parallel()
	got := ApplyOverageDiscount(1000, 1)
	assert.EqualValues(t, 990,
		got)

}

func TestApplyOverageDiscount_BoundaryDiscount99(t *testing.T) {
	t.Parallel()
	got := ApplyOverageDiscount(1000, 99)
	assert.EqualValues(t, 10,
		got)

}

func TestApplyOverageDiscount_Over100(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 0,
		ApplyOverageDiscount(1000, 150))

}

func TestValidateEnterpriseContract_DiscountBoundaries(t *testing.T) {
	t.Parallel()
	base := func() *EnterpriseContract {
		return &EnterpriseContract{
			OrgID:                 "org-1",
			EnterpriseTier:        EnterpriseTierStarter,
			AnnualCommitmentCents: 1_800_000,
			ContractStartDate:     time.Now(),
			ContractEndDate:       time.Now().AddDate(1, 0, 0),
			BillingCadence:        "annual",
		}
	}

	// Discount at 0 should pass.
	c := base()
	c.OverageDiscountPct = 0
	assert.NoError(t,
		ValidateEnterpriseContract(c))

	// Discount at 100 should pass.
	c = base()
	c.OverageDiscountPct = 100
	assert.NoError(t,
		ValidateEnterpriseContract(c))

	// Discount at -1 should fail.
	c = base()
	c.OverageDiscountPct = -1
	assert.Error(t, ValidateEnterpriseContract(c))

	// Discount at 101 should fail.
	c = base()
	c.OverageDiscountPct = 101
	assert.Error(t, ValidateEnterpriseContract(c))

}

func TestValidateEnterpriseContract_ExactMinCommitment(t *testing.T) {
	t.Parallel()
	c := &EnterpriseContract{
		OrgID:                 "org-1",
		EnterpriseTier:        EnterpriseTierStarter,
		AnnualCommitmentCents: EnterpriseStarterAnnualCents,
		ContractStartDate:     time.Now(),
		ContractEndDate:       time.Now().AddDate(1, 0, 0),
		BillingCadence:        "annual",
	}
	assert.NoError(t,
		ValidateEnterpriseContract(c))

}
