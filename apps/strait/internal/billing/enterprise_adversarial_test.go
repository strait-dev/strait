package billing

import (
	"math"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
)

// Contract validation adversarial tests.

func TestEnterpriseContract_NegativeCommitment(t *testing.T) {
	t.Parallel()
	c := &EnterpriseContract{
		OrgID:                 "org-1",
		EnterpriseTier:        EnterpriseTierStarter,
		AnnualCommitmentCents: -100,
		ContractStartDate:     time.Now(),
		ContractEndDate:       time.Now().AddDate(1, 0, 0),
		BillingCadence:        "annual",
	}
	if err := ValidateEnterpriseContract(c); err == nil {
		t.Fatal("expected error for negative commitment")
	}
}

func TestEnterpriseContract_NegativeCredit(t *testing.T) {
	t.Parallel()
	c := &EnterpriseContract{
		OrgID:                  "org-1",
		EnterpriseTier:         EnterpriseTierStarter,
		AnnualCommitmentCents:  1_800_000,
		IncludedCreditMicrousd: -1,
		ContractStartDate:      time.Now(),
		ContractEndDate:        time.Now().AddDate(1, 0, 0),
		BillingCadence:         "annual",
	}
	if err := ValidateEnterpriseContract(c); err == nil {
		t.Fatal("expected error for negative credit")
	}
}

func TestEnterpriseContract_NegativeDiscount(t *testing.T) {
	t.Parallel()
	c := &EnterpriseContract{
		OrgID:                 "org-1",
		EnterpriseTier:        EnterpriseTierStarter,
		AnnualCommitmentCents: 1_800_000,
		ComputeDiscountPct:    -5,
		ContractStartDate:     time.Now(),
		ContractEndDate:       time.Now().AddDate(1, 0, 0),
		BillingCadence:        "annual",
	}
	if err := ValidateEnterpriseContract(c); err == nil {
		t.Fatal("expected error for negative discount")
	}
}

func TestEnterpriseContract_DiscountOver100(t *testing.T) {
	t.Parallel()
	c := &EnterpriseContract{
		OrgID:                 "org-1",
		EnterpriseTier:        EnterpriseTierStarter,
		AnnualCommitmentCents: 1_800_000,
		ComputeDiscountPct:    150,
		ContractStartDate:     time.Now(),
		ContractEndDate:       time.Now().AddDate(1, 0, 0),
		BillingCadence:        "annual",
	}
	if err := ValidateEnterpriseContract(c); err == nil {
		t.Fatal("expected error for discount > 100")
	}
}

func TestEnterpriseContract_ZeroLengthContract(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c := &EnterpriseContract{
		OrgID:                 "org-1",
		EnterpriseTier:        EnterpriseTierStarter,
		AnnualCommitmentCents: 1_800_000,
		ContractStartDate:     now,
		ContractEndDate:       now, // same as start
		BillingCadence:        "annual",
	}
	if err := ValidateEnterpriseContract(c); err == nil {
		t.Fatal("expected error for zero-length contract")
	}
}

func TestEnterpriseContract_InvalidBillingCadences(t *testing.T) {
	t.Parallel()
	invalid := []string{"weekly", "daily", "", "monthly", "ANNUAL", "Quarterly"}
	for _, cadence := range invalid {
		c := &EnterpriseContract{
			OrgID:                 "org-1",
			EnterpriseTier:        EnterpriseTierStarter,
			AnnualCommitmentCents: 1_800_000,
			ContractStartDate:     time.Now(),
			ContractEndDate:       time.Now().AddDate(1, 0, 0),
			BillingCadence:        cadence,
		}
		if err := ValidateEnterpriseContract(c); err == nil {
			t.Errorf("expected error for cadence %q", cadence)
		}
	}
}

// ApplyComputeDiscount adversarial tests.

func TestApplyComputeDiscount_NegativeCost(t *testing.T) {
	t.Parallel()
	got := ApplyComputeDiscount(-1_000_000, 10)
	if got != 0 {
		t.Errorf("ApplyComputeDiscount(-1000000, 10) = %d, want 0", got)
	}
}

func TestApplyComputeDiscount_OverflowCost(t *testing.T) {
	t.Parallel()
	// Should not panic with very large values.
	got := ApplyComputeDiscount(math.MaxInt64, 10)
	// The exact value depends on overflow behavior, but it should not be negative
	// or panic. With int64 arithmetic: MaxInt64 * 90 / 100 is within bounds.
	if got < 0 {
		t.Errorf("ApplyComputeDiscount(MaxInt64, 10) = %d, should be non-negative", got)
	}
}

// EnterpriseTierForPrice adversarial tests.

func TestEnterpriseTierForPrice_NullBytes(t *testing.T) {
	t.Parallel()
	tier, ok := EnterpriseTierForPrice("price\x00id")
	if ok {
		t.Errorf("expected false for null bytes, got tier=%q", tier)
	}
}

func TestEnterpriseTierForPrice_VeryLongString(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 100_000)
	tier, ok := EnterpriseTierForPrice(long)
	if ok {
		t.Errorf("expected false for very long string, got tier=%q", tier)
	}
}

func TestEnterpriseTierForPrice_SQLInjection(t *testing.T) {
	t.Parallel()
	malicious := "'; DROP TABLE enterprise_contracts; --"
	tier, ok := EnterpriseTierForPrice(malicious)
	if ok {
		t.Errorf("expected false for SQL injection, got tier=%q", tier)
	}
}

// IsDowngrade enterprise transitions.

func TestIsDowngrade_EnterpriseToScale(t *testing.T) {
	t.Parallel()
	if !IsDowngrade(domain.PlanEnterprise, domain.PlanScale) {
		t.Error("Enterprise -> Scale should be a downgrade")
	}
}

func TestIsDowngrade_ScaleToEnterprise(t *testing.T) {
	t.Parallel()
	if IsDowngrade(domain.PlanScale, domain.PlanEnterprise) {
		t.Error("Scale -> Enterprise should not be a downgrade")
	}
}

func TestIsDowngrade_EnterpriseToEnterprise(t *testing.T) {
	t.Parallel()
	if IsDowngrade(domain.PlanEnterprise, domain.PlanEnterprise) {
		t.Error("Enterprise -> Enterprise should not be a downgrade")
	}
}

func TestIsDowngrade_EnterpriseToFree(t *testing.T) {
	t.Parallel()
	if !IsDowngrade(domain.PlanEnterprise, domain.PlanFree) {
		t.Error("Enterprise -> Free should be a downgrade")
	}
}

// SLA credit boundary tests.

func TestCalculateSLACredit_ExactBoundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		uptime float64
		want   int
	}{
		{99.9, 0},   // at threshold
		{99.89, 10}, // just below
		{99.0, 10},  // at 99.0-99.9 boundary
		{98.99, 20}, // just below 99.0
		{95.0, 20},  // at 95.0-99.0 boundary
		{94.99, 30}, // just below 95.0
		{90.0, 30},  // at 90.0-95.0 boundary
		{89.99, 50}, // just below 90.0
	}
	for _, tt := range tests {
		got := CalculateSLACredit(tt.uptime)
		if got != tt.want {
			t.Errorf("CalculateSLACredit(%.2f) = %d, want %d", tt.uptime, got, tt.want)
		}
	}
}

func TestCalculateSLACredit_NegativeUptime(t *testing.T) {
	t.Parallel()
	got := CalculateSLACredit(-10.0)
	if got != 50 {
		t.Errorf("CalculateSLACredit(-10.0) = %d, want 50", got)
	}
}
