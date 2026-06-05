package billing

import (
	"testing"
	"time"
)

// Enterprise tier constants.

func TestEnterpriseTierConstants(t *testing.T) {
	t.Parallel()
	tiers := AllEnterpriseTiers()
	if len(tiers) != 3 {
		t.Fatalf("expected 3 enterprise tiers, got %d", len(tiers))
	}
	if tiers[0] != EnterpriseTierStarter {
		t.Errorf("tier[0] = %q, want %q", tiers[0], EnterpriseTierStarter)
	}
	if tiers[1] != EnterpriseTierGrowth {
		t.Errorf("tier[1] = %q, want %q", tiers[1], EnterpriseTierGrowth)
	}
	if tiers[2] != EnterpriseTierLarge {
		t.Errorf("tier[2] = %q, want %q", tiers[2], EnterpriseTierLarge)
	}
}

func TestIsValidEnterpriseTier_ValidTiers(t *testing.T) {
	t.Parallel()
	for _, tier := range AllEnterpriseTiers() {
		if !IsValidEnterpriseTier(tier) {
			t.Errorf("IsValidEnterpriseTier(%q) = false, want true", tier)
		}
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
		if IsValidEnterpriseTier(tier) {
			t.Errorf("IsValidEnterpriseTier(%q) = true, want false", tier)
		}
	}
}

// Enterprise config completeness.

func TestEnterpriseConfigCompleteness(t *testing.T) {
	t.Parallel()
	for _, tier := range AllEnterpriseTiers() {
		cfg, ok := EnterpriseConfigs[tier]
		if !ok {
			t.Fatalf("missing config for tier %q", tier)
		}
		if cfg.Tier != tier {
			t.Errorf("config.Tier = %q, want %q", cfg.Tier, tier)
		}
		if cfg.DisplayName == "" {
			t.Errorf("config for %q has empty DisplayName", tier)
		}
		if cfg.AnnualCommitmentCents <= 0 {
			t.Errorf("config for %q has non-positive AnnualCommitmentCents", tier)
		}
		if cfg.MonthlyEquivalentCents <= 0 {
			t.Errorf("config for %q has non-positive MonthlyEquivalentCents", tier)
		}
		if cfg.OverageDiscountPct <= 0 {
			t.Errorf("config for %q has non-positive OverageDiscountPct", tier)
		}
		if cfg.UptimeSLAPct < 99.0 {
			t.Errorf("config for %q has SLA below 99%%: %.2f", tier, cfg.UptimeSLAPct)
		}
		if cfg.MaxDowntimeMinutes <= 0 {
			t.Errorf("config for %q has non-positive MaxDowntimeMinutes", tier)
		}
		if cfg.SupportResponseP1 == "" || cfg.SupportResponseP2 == "" || cfg.SupportResponseP3 == "" {
			t.Errorf("config for %q has empty support response time", tier)
		}
	}
}

func TestEnterpriseConfigValues_StarterTier(t *testing.T) {
	t.Parallel()
	cfg := EnterpriseConfigs[EnterpriseTierStarter]

	if cfg.AnnualCommitmentCents != 1_800_000 {
		t.Errorf("Starter annual = %d, want 1800000", cfg.AnnualCommitmentCents)
	}
	if cfg.MonthlyEquivalentCents != 150_000 {
		t.Errorf("Starter monthly = %d, want 150000", cfg.MonthlyEquivalentCents)
	}
	if cfg.PlatformFeeMicrousd != 1_500_000_000 {
		t.Errorf("Starter platform fee = %d, want 1500000000", cfg.PlatformFeeMicrousd)
	}
	if cfg.OverageDiscountPct != 10 {
		t.Errorf("Starter discount = %d%%, want 10%%", cfg.OverageDiscountPct)
	}
	if cfg.UptimeSLAPct != 99.9 {
		t.Errorf("Starter SLA = %.2f, want 99.9", cfg.UptimeSLAPct)
	}
	if cfg.MaxDowntimeMinutes != 43.8 {
		t.Errorf("Starter max downtime = %.1f, want 43.8", cfg.MaxDowntimeMinutes)
	}
}

func TestEnterpriseConfigValues_GrowthTier(t *testing.T) {
	t.Parallel()
	cfg := EnterpriseConfigs[EnterpriseTierGrowth]

	if cfg.AnnualCommitmentCents != 4_800_000 {
		t.Errorf("Growth annual = %d, want 4800000", cfg.AnnualCommitmentCents)
	}
	if cfg.MonthlyEquivalentCents != 400_000 {
		t.Errorf("Growth monthly = %d, want 400000", cfg.MonthlyEquivalentCents)
	}
	if cfg.PlatformFeeMicrousd != 4_000_000_000 {
		t.Errorf("Growth platform fee = %d, want 4000000000", cfg.PlatformFeeMicrousd)
	}
	if cfg.OverageDiscountPct != 15 {
		t.Errorf("Growth discount = %d%%, want 15%%", cfg.OverageDiscountPct)
	}
	if cfg.UptimeSLAPct != 99.95 {
		t.Errorf("Growth SLA = %.2f, want 99.95", cfg.UptimeSLAPct)
	}
	if cfg.MaxDowntimeMinutes != 21.9 {
		t.Errorf("Growth max downtime = %.1f, want 21.9", cfg.MaxDowntimeMinutes)
	}
}

func TestEnterpriseConfigValues_LargeTier(t *testing.T) {
	t.Parallel()
	cfg := EnterpriseConfigs[EnterpriseTierLarge]

	if cfg.AnnualCommitmentCents != 9_600_000 {
		t.Errorf("Large annual = %d, want 9600000", cfg.AnnualCommitmentCents)
	}
	if cfg.OverageDiscountPct != 20 {
		t.Errorf("Large discount = %d%%, want 20%%", cfg.OverageDiscountPct)
	}
	if cfg.UptimeSLAPct != 99.95 {
		t.Errorf("Large SLA = %.2f, want 99.95", cfg.UptimeSLAPct)
	}
}

func TestPlatformFee_ConsistentWithCommitment(t *testing.T) {
	t.Parallel()
	starter := EnterpriseConfigs[EnterpriseTierStarter]
	if platformDollars := starter.PlatformFeeMicrousd / 1_000_000; platformDollars != starter.MonthlyEquivalentCents/100 {
		t.Errorf("Starter: platform fee $%d, want monthly equivalent $%d",
			platformDollars, starter.MonthlyEquivalentCents/100)
	}

	growth := EnterpriseConfigs[EnterpriseTierGrowth]
	if platformDollars := growth.PlatformFeeMicrousd / 1_000_000; platformDollars != growth.MonthlyEquivalentCents/100 {
		t.Errorf("Growth: platform fee $%d, want monthly equivalent $%d",
			platformDollars, growth.MonthlyEquivalentCents/100)
	}
}

func TestGetEnterpriseConfig_ValidTiers(t *testing.T) {
	t.Parallel()
	for _, tier := range AllEnterpriseTiers() {
		cfg := GetEnterpriseConfig(tier)
		if cfg.Tier != tier {
			t.Errorf("GetEnterpriseConfig(%q).Tier = %q", tier, cfg.Tier)
		}
	}
}

func TestGetEnterpriseConfig_UnknownTierReturnsFallback(t *testing.T) {
	t.Parallel()
	cfg := GetEnterpriseConfig("unknown")
	if cfg.Tier != EnterpriseTierStarter {
		t.Errorf("GetEnterpriseConfig(unknown).Tier = %q, want %q", cfg.Tier, EnterpriseTierStarter)
	}
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
			if got != tt.want {
				t.Errorf("ApplyOverageDiscount(%d, %d) = %d, want %d", cost, tt.discount, got, tt.want)
			}
		})
	}
}

func TestApplyOverageDiscount_ZeroCost(t *testing.T) {
	t.Parallel()
	if got := ApplyOverageDiscount(0, 10); got != 0 {
		t.Errorf("ApplyOverageDiscount(0, 10) = %d, want 0", got)
	}
}

func TestApplyOverageDiscount_ZeroDiscount(t *testing.T) {
	t.Parallel()
	cost := int64(500_000)
	if got := ApplyOverageDiscount(cost, 0); got != cost {
		t.Errorf("ApplyOverageDiscount(%d, 0) = %d, want %d", cost, got, cost)
	}
}

func TestApplyOverageDiscount_FullDiscount(t *testing.T) {
	t.Parallel()
	if got := ApplyOverageDiscount(1_000_000, 100); got != 0 {
		t.Errorf("ApplyOverageDiscount(1000000, 100) = %d, want 0", got)
	}
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
	if err := ValidateEnterpriseContract(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	if err := ValidateEnterpriseContract(c); err == nil {
		t.Fatal("expected error for empty org_id")
	}
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
	if err := ValidateEnterpriseContract(c); err == nil {
		t.Fatal("expected error for invalid tier")
	}
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
	if err := ValidateEnterpriseContract(c); err == nil {
		t.Fatal("expected error for commitment below minimum")
	}
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
	if err := ValidateEnterpriseContract(c); err == nil {
		t.Fatal("expected error for end before start")
	}
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
	if err := ValidateEnterpriseContract(c); err == nil {
		t.Fatal("expected error for invalid cadence")
	}
}

func TestIsValidBillingCadence(t *testing.T) {
	t.Parallel()
	valid := []string{"annual", "quarterly"}
	for _, c := range valid {
		if !IsValidBillingCadence(c) {
			t.Errorf("IsValidBillingCadence(%q) = false, want true", c)
		}
	}
	invalid := []string{"", "monthly", "weekly", "daily", "ANNUAL"}
	for _, c := range invalid {
		if IsValidBillingCadence(c) {
			t.Errorf("IsValidBillingCadence(%q) = true, want false", c)
		}
	}
}

// SLA credit calculation tests.

func TestCalculateSLACredit_AboveThreshold(t *testing.T) {
	t.Parallel()
	if got := CalculateSLACredit(99.95, EnterpriseStarterSLAPct); got != 0 {
		t.Errorf("CalculateSLACredit(99.95, 99.9) = %d, want 0", got)
	}
	if got := CalculateSLACredit(100.0, EnterpriseStarterSLAPct); got != 0 {
		t.Errorf("CalculateSLACredit(100.0, 99.9) = %d, want 0", got)
	}
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
		if got := CalculateSLACredit(tt.uptime, EnterpriseStarterSLAPct); got != tt.want {
			t.Errorf("CalculateSLACredit(%.1f, 99.9) = %d, want %d", tt.uptime, got, tt.want)
		}
	}
}

func TestCalculateSLACredit_ExactBoundary999(t *testing.T) {
	t.Parallel()
	// Exactly 99.9 is at the SLA threshold for Starter, so no credit.
	if got := CalculateSLACredit(99.9, EnterpriseStarterSLAPct); got != 0 {
		t.Errorf("CalculateSLACredit(99.9, 99.9) = %d, want 0", got)
	}
}

func TestCalculateSLACredit_PerTierSLATarget(t *testing.T) {
	t.Parallel()
	// Growth/Large tiers have 99.95% SLA, so 99.92% is below target and should get credit.
	if got := CalculateSLACredit(99.92, EnterpriseGrowthSLAPct); got != 10 {
		t.Errorf("CalculateSLACredit(99.92, 99.95) = %d, want 10 (below Growth SLA)", got)
	}
	// But 99.92% with Starter SLA (99.9%) should get 0 credit (above target).
	if got := CalculateSLACredit(99.92, EnterpriseStarterSLAPct); got != 0 {
		t.Errorf("CalculateSLACredit(99.92, 99.9) = %d, want 0 (above Starter SLA)", got)
	}
	// 99.95% exactly at Growth SLA target -- no credit.
	if got := CalculateSLACredit(99.95, EnterpriseGrowthSLAPct); got != 0 {
		t.Errorf("CalculateSLACredit(99.95, 99.95) = %d, want 0", got)
	}
	// 99.94% just below Growth SLA -- should get credit.
	if got := CalculateSLACredit(99.94, EnterpriseGrowthSLAPct); got != 10 {
		t.Errorf("CalculateSLACredit(99.94, 99.95) = %d, want 10", got)
	}
	// Large tier with same SLA as Growth.
	if got := CalculateSLACredit(99.93, EnterpriseLargeSLAPct); got != 10 {
		t.Errorf("CalculateSLACredit(99.93, 99.95) = %d, want 10", got)
	}
}

func TestCalculateSLACredit_CustomSLATarget(t *testing.T) {
	t.Parallel()
	// Verify arbitrary SLA targets work (e.g. custom enterprise contracts).
	if got := CalculateSLACredit(99.98, 99.99); got != 10 {
		t.Errorf("CalculateSLACredit(99.98, 99.99) = %d, want 10", got)
	}
	if got := CalculateSLACredit(99.99, 99.99); got != 0 {
		t.Errorf("CalculateSLACredit(99.99, 99.99) = %d, want 0", got)
	}
}

func TestApplyOverageDiscount_NegativeCostReturnsZero(t *testing.T) {
	t.Parallel()
	if got := ApplyOverageDiscount(-100, 10); got != 0 {
		t.Errorf("ApplyOverageDiscount(-100, 10) = %d, want 0", got)
	}
}

func TestApplyOverageDiscount_BoundaryDiscount1(t *testing.T) {
	t.Parallel()
	got := ApplyOverageDiscount(1000, 1)
	if got != 990 {
		t.Errorf("ApplyOverageDiscount(1000, 1) = %d, want 990", got)
	}
}

func TestApplyOverageDiscount_BoundaryDiscount99(t *testing.T) {
	t.Parallel()
	got := ApplyOverageDiscount(1000, 99)
	if got != 10 {
		t.Errorf("ApplyOverageDiscount(1000, 99) = %d, want 10", got)
	}
}

func TestApplyOverageDiscount_Over100(t *testing.T) {
	t.Parallel()
	if got := ApplyOverageDiscount(1000, 150); got != 0 {
		t.Errorf("ApplyOverageDiscount(1000, 150) = %d, want 0 (>= 100 returns 0)", got)
	}
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
	if err := ValidateEnterpriseContract(c); err != nil {
		t.Errorf("discount=0 should be valid: %v", err)
	}

	// Discount at 100 should pass.
	c = base()
	c.OverageDiscountPct = 100
	if err := ValidateEnterpriseContract(c); err != nil {
		t.Errorf("discount=100 should be valid: %v", err)
	}

	// Discount at -1 should fail.
	c = base()
	c.OverageDiscountPct = -1
	if err := ValidateEnterpriseContract(c); err == nil {
		t.Error("discount=-1 should be invalid")
	}

	// Discount at 101 should fail.
	c = base()
	c.OverageDiscountPct = 101
	if err := ValidateEnterpriseContract(c); err == nil {
		t.Error("discount=101 should be invalid")
	}
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
	if err := ValidateEnterpriseContract(c); err != nil {
		t.Errorf("exact min commitment should be valid: %v", err)
	}
}
