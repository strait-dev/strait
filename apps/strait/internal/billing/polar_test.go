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
	if len(m.productToTier) != 0 {
		t.Errorf("expected empty mapping, got %d entries", len(m.productToTier))
	}
}
