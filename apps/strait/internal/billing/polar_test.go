package billing

import (
	"testing"

	"strait/internal/domain"
)

func TestPolarMapping(t *testing.T) {
	t.Parallel()

	m := NewPolarMapping(
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
			tier, ok := m.TierForProduct(tt.productID)
			if tier != tt.wantTier {
				t.Errorf("TierForProduct(%q) tier = %q, want %q", tt.productID, tier, tt.wantTier)
			}
			if ok != tt.wantOK {
				t.Errorf("TierForProduct(%q) ok = %v, want %v", tt.productID, ok, tt.wantOK)
			}
		})
	}
}

func TestPolarMapping_EmptyIDs(t *testing.T) {
	t.Parallel()

	m := NewPolarMapping("", "", "", "")
	if m.ProductCount() != 0 {
		t.Errorf("expected empty mapping, got %d entries", m.ProductCount())
	}
	if m.HasProducts() {
		t.Error("HasProducts() = true, want false for empty mapping")
	}
}

func TestPolarMappingFromOptions(t *testing.T) {
	t.Parallel()

	m := NewPolarMappingFromOptions(
		WithStarterProducts("s-m", "s-y"),
		WithProProducts("p-m", "p-y"),
		WithScaleProducts("sc-m", "sc-y"),
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
			tier, ok := m.TierForProduct(tt.productID)
			if tier != tt.wantTier {
				t.Errorf("TierForProduct(%q) tier = %q, want %q", tt.productID, tier, tt.wantTier)
			}
			if ok != tt.wantOK {
				t.Errorf("TierForProduct(%q) ok = %v, want %v", tt.productID, ok, tt.wantOK)
			}
		})
	}

	if m.ProductCount() != 6 {
		t.Errorf("ProductCount() = %d, want 6", m.ProductCount())
	}
	if !m.HasProducts() {
		t.Error("HasProducts() = false, want true")
	}
}

func TestPolarMappingFromOptions_EmptyIDs(t *testing.T) {
	t.Parallel()

	m := NewPolarMappingFromOptions(
		WithStarterProducts("", ""),
		WithProProducts("", ""),
		WithScaleProducts("", ""),
	)

	if m.ProductCount() != 0 {
		t.Errorf("expected empty mapping, got %d entries", m.ProductCount())
	}
}

func TestPolarMappingFromOptions_PartialIDs(t *testing.T) {
	t.Parallel()

	m := NewPolarMappingFromOptions(
		WithStarterProducts("s-m", ""),
		WithScaleProducts("", "sc-y"),
	)

	if m.ProductCount() != 2 {
		t.Errorf("ProductCount() = %d, want 2", m.ProductCount())
	}

	tier, ok := m.TierForProduct("s-m")
	if !ok || tier != domain.PlanStarter {
		t.Errorf("expected starter for s-m, got %q/%v", tier, ok)
	}

	tier, ok = m.TierForProduct("sc-y")
	if !ok || tier != domain.PlanScale {
		t.Errorf("expected scale for sc-y, got %q/%v", tier, ok)
	}
}

func TestNewPolarMapping_BackwardCompatible(t *testing.T) {
	t.Parallel()

	// Verify legacy constructor still works identically.
	legacy := NewPolarMapping("s-m", "s-y", "p-m", "p-y")
	opts := NewPolarMappingFromOptions(
		WithStarterProducts("s-m", "s-y"),
		WithProProducts("p-m", "p-y"),
	)

	for _, id := range []string{"s-m", "s-y", "p-m", "p-y", "unknown"} {
		lt, lok := legacy.TierForProduct(id)
		ot, ook := opts.TierForProduct(id)
		if lt != ot || lok != ook {
			t.Errorf("product %q: legacy=(%q,%v) opts=(%q,%v)", id, lt, lok, ot, ook)
		}
	}
}
