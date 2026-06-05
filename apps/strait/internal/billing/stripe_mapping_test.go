package billing

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestStripeMapping(t *testing.T) {
	t.Parallel()

	m := NewStripeMapping(
		"starter-month-id", "starter-year-id",
		"pro-month-id", "pro-year-id",
	)

	tests := []struct {
		name      string
		productID string
		wantTier  domain.PlanTier
		wantOK    bool
	}{
		{"starter_monthly", "starter-month-id", domain.PlanStarter, true},
		{"starter_yearly", "starter-year-id", domain.PlanStarter, true},
		{"pro_monthly", "pro-month-id", domain.PlanPro, true},
		{"pro_yearly", "pro-year-id", domain.PlanPro, true},
		{"unknown", "unknown-id", domain.PlanFree, false},
		{"empty", "", domain.PlanFree, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tier, ok := m.TierForPrice(tt.productID)
			assert.Equal(t, tt.
				wantTier, tier)
			assert.Equal(t, tt.
				wantOK, ok)

		})
	}
}

func TestStripeMapping_EmptyIDs(t *testing.T) {
	t.Parallel()

	m := NewStripeMapping("", "", "", "")
	assert.EqualValues(t, 0,

		m.PriceCount())
	assert.False(t, m.
		HasPrices())

}

func TestStripeMappingFromOptions(t *testing.T) {
	t.Parallel()

	m := NewStripeMappingFromOptions(
		WithStarterPrices("s-m", "s-y"),
		WithProPrices("p-m", "p-y"),
		WithScalePrices("sc-m", "sc-y"),
	)

	tests := []struct {
		name      string
		productID string
		wantTier  domain.PlanTier
		wantOK    bool
	}{
		{"starter_monthly", "s-m", domain.PlanStarter, true},
		{"starter_yearly", "s-y", domain.PlanStarter, true},
		{"pro_monthly", "p-m", domain.PlanPro, true},
		{"pro_yearly", "p-y", domain.PlanPro, true},
		{"scale_monthly", "sc-m", domain.PlanScale, true},
		{"scale_yearly", "sc-y", domain.PlanScale, true},
		{"unknown", "unknown", domain.PlanFree, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tier, ok := m.TierForPrice(tt.productID)
			assert.Equal(t, tt.
				wantTier, tier)
			assert.Equal(t, tt.
				wantOK, ok)

		})
	}
	assert.EqualValues(t, 6,

		m.PriceCount())
	assert.True(t, m.
		HasPrices())

}

func TestStripeMappingFromOptions_EmptyIDs(t *testing.T) {
	t.Parallel()

	m := NewStripeMappingFromOptions(
		WithStarterPrices("", ""),
		WithProPrices("", ""),
		WithScalePrices("", ""),
	)
	assert.EqualValues(t, 0,

		m.PriceCount())

}

func TestStripeMappingFromOptions_PartialIDs(t *testing.T) {
	t.Parallel()

	m := NewStripeMappingFromOptions(
		WithStarterPrices("s-m", ""),
		WithScalePrices("", "sc-y"),
	)
	assert.EqualValues(t, 2,

		m.PriceCount())

	tier, ok := m.TierForPrice("s-m")
	assert.False(t, !ok ||
		tier != domain.PlanStarter,
	)

	tier, ok = m.TierForPrice("sc-y")
	assert.False(t, !ok ||
		tier != domain.PlanScale,
	)

}

func TestNewStripeMapping_BackwardCompatible(t *testing.T) {
	t.Parallel()

	// Verify legacy constructor still works identically.
	legacy := NewStripeMapping("s-m", "s-y", "p-m", "p-y")
	opts := NewStripeMappingFromOptions(
		WithStarterPrices("s-m", "s-y"),
		WithProPrices("p-m", "p-y"),
	)

	for _, id := range []string{"s-m", "s-y", "p-m", "p-y", "unknown"} {
		lt, lok := legacy.TierForPrice(id)
		ot, ook := opts.TierForPrice(id)
		assert.False(t, lt !=
			ot || lok != ook)

	}
}
