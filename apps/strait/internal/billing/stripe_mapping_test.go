package billing

import (
	"testing"

	"strait/internal/domain"
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
			if tier != tt.wantTier {
				t.Errorf("TierForPrice(%q) tier = %q, want %q", tt.productID, tier, tt.wantTier)
			}
			if ok != tt.wantOK {
				t.Errorf("TierForPrice(%q) ok = %v, want %v", tt.productID, ok, tt.wantOK)
			}
		})
	}
}

func TestStripeMapping_EmptyIDs(t *testing.T) {
	t.Parallel()

	m := NewStripeMapping("", "", "", "")
	if m.PriceCount() != 0 {
		t.Errorf("expected empty mapping, got %d entries", m.PriceCount())
	}
	if m.HasPrices() {
		t.Error("HasPrices() = true, want false for empty mapping")
	}
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
			if tier != tt.wantTier {
				t.Errorf("TierForPrice(%q) tier = %q, want %q", tt.productID, tier, tt.wantTier)
			}
			if ok != tt.wantOK {
				t.Errorf("TierForPrice(%q) ok = %v, want %v", tt.productID, ok, tt.wantOK)
			}
		})
	}

	if m.PriceCount() != 6 {
		t.Errorf("PriceCount() = %d, want 6", m.PriceCount())
	}
	if !m.HasPrices() {
		t.Error("HasPrices() = false, want true")
	}
}

func TestStripeMappingFromOptions_EmptyIDs(t *testing.T) {
	t.Parallel()

	m := NewStripeMappingFromOptions(
		WithStarterPrices("", ""),
		WithProPrices("", ""),
		WithScalePrices("", ""),
	)

	if m.PriceCount() != 0 {
		t.Errorf("expected empty mapping, got %d entries", m.PriceCount())
	}
}

func TestStripeMappingFromOptions_PartialIDs(t *testing.T) {
	t.Parallel()

	m := NewStripeMappingFromOptions(
		WithStarterPrices("s-m", ""),
		WithScalePrices("", "sc-y"),
	)

	if m.PriceCount() != 2 {
		t.Errorf("PriceCount() = %d, want 2", m.PriceCount())
	}

	tier, ok := m.TierForPrice("s-m")
	if !ok || tier != domain.PlanStarter {
		t.Errorf("expected starter for s-m, got %q/%v", tier, ok)
	}

	tier, ok = m.TierForPrice("sc-y")
	if !ok || tier != domain.PlanScale {
		t.Errorf("expected scale for sc-y, got %q/%v", tier, ok)
	}
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
		if lt != ot || lok != ook {
			t.Errorf("product %q: legacy=(%q,%v) opts=(%q,%v)", id, lt, lok, ot, ook)
		}
	}
}
