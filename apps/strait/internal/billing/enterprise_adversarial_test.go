package billing

import (
	"math"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Error(
		t, ValidateEnterpriseContract(
			c,
		))
}

func TestEnterpriseContract_NegativeDiscount(t *testing.T) {
	t.Parallel()
	c := &EnterpriseContract{
		OrgID:                 "org-1",
		EnterpriseTier:        EnterpriseTierStarter,
		AnnualCommitmentCents: 1_800_000,
		OverageDiscountPct:    -5,
		ContractStartDate:     time.Now(),
		ContractEndDate:       time.Now().AddDate(1, 0, 0),
		BillingCadence:        "annual",
	}
	require.Error(
		t, ValidateEnterpriseContract(
			c,
		))
}

func TestEnterpriseContract_DiscountOver100(t *testing.T) {
	t.Parallel()
	c := &EnterpriseContract{
		OrgID:                 "org-1",
		EnterpriseTier:        EnterpriseTierStarter,
		AnnualCommitmentCents: 1_800_000,
		OverageDiscountPct:    150,
		ContractStartDate:     time.Now(),
		ContractEndDate:       time.Now().AddDate(1, 0, 0),
		BillingCadence:        "annual",
	}
	require.Error(
		t, ValidateEnterpriseContract(
			c,
		))
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
	require.Error(
		t, ValidateEnterpriseContract(
			c,
		))
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
		assert.Error(t,
			ValidateEnterpriseContract(c),
		)
	}
}

// ApplyOverageDiscount adversarial tests.

func TestApplyOverageDiscount_NegativeCost(t *testing.T) {
	t.Parallel()
	got := ApplyOverageDiscount(-1_000_000, 10)
	assert.EqualValues(t, 0, got)
}

func TestApplyOverageDiscount_OverflowCost(t *testing.T) {
	t.Parallel()
	// Should not panic with very large values.
	got := ApplyOverageDiscount(math.MaxInt64, 10)
	assert.GreaterOrEqual(t,
		got, int64(0))

	// The exact value depends on overflow behavior, but it should not be negative
	// or panic. With int64 arithmetic: MaxInt64 * 90 / 100 is within bounds.
}

// EnterpriseTierForPrice adversarial tests.

func TestEnterpriseTierForPrice_NullBytes(t *testing.T) {
	t.Parallel()
	_, ok := EnterpriseTierForPrice("price\x00id")
	assert.False(t,
		ok)
}

func TestEnterpriseTierForPrice_VeryLongString(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 100_000)
	_, ok := EnterpriseTierForPrice(long)
	assert.False(t,
		ok)
}

func TestEnterpriseTierForPrice_SQLInjection(t *testing.T) {
	t.Parallel()
	malicious := "'; DROP TABLE enterprise_contracts; --"
	_, ok := EnterpriseTierForPrice(malicious)
	assert.False(t,
		ok)
}

// IsDowngrade enterprise transitions.

func TestIsDowngrade_EnterpriseToScale(t *testing.T) {
	t.Parallel()
	assert.True(t,
		IsDowngrade(domain.PlanEnterprise,

			domain.PlanScale))
}

func TestIsDowngrade_ScaleToEnterprise(t *testing.T) {
	t.Parallel()
	assert.False(t,
		IsDowngrade(domain.PlanScale,

			domain.PlanEnterprise))
}

func TestIsDowngrade_EnterpriseToEnterprise(t *testing.T) {
	t.Parallel()
	assert.False(t,
		IsDowngrade(domain.PlanEnterprise,

			domain.PlanEnterprise,
		),
	)
}

func TestIsDowngrade_EnterpriseToFree(t *testing.T) {
	t.Parallel()
	assert.True(t,
		IsDowngrade(domain.PlanEnterprise,

			domain.PlanFree))
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
		{98.99, 25}, // just below 99.0
		{95.0, 25},  // at 95.0-99.0 boundary
		{94.99, 50}, // just below 95.0 (collapsed band)
		{90.0, 50},  // within 0-95 band
		{89.99, 50}, // still within 0-95 band
	}
	for _, tt := range tests {
		got := CalculateSLACredit(tt.uptime, EnterpriseStarterSLAPct)
		assert.Equal(t,
			tt.want,
			got)
	}
}

func TestCalculateSLACredit_NegativeUptime(t *testing.T) {
	t.Parallel()
	got := CalculateSLACredit(-10.0, EnterpriseStarterSLAPct)
	assert.Equal(t, 50, got)
}

func TestCalculateSLACredit_NaNUptimeDoesNotGrantCredit(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0, CalculateSLACredit(math.
		NaN(), EnterpriseStarterSLAPct,
	))
	require.Equal(t, 0, CalculateSLACredit(99.0,

		math.NaN()))
}
