package billing

import (
	"testing"

	"strait/internal/domain"
)

func TestMaxSpendingLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tier domain.PlanTier
		want int64
	}{
		{"free", domain.PlanFree, 0},
		{"starter", domain.PlanStarter, 500000000},
		{"pro", domain.PlanPro, 2000000000},
		{"enterprise", domain.PlanEnterprise, -1},
		{"unknown", domain.PlanTier("unknown"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MaxSpendingLimit(tt.tier)
			if got != tt.want {
				t.Errorf("MaxSpendingLimit(%q) = %d, want %d", tt.tier, got, tt.want)
			}
		})
	}
}

func TestSpendingLimitPresets(t *testing.T) {
	t.Parallel()

	if len(SpendingLimitPresets) == 0 {
		t.Fatal("expected non-empty presets")
	}

	// Verify presets are in ascending order
	for i := 1; i < len(SpendingLimitPresets); i++ {
		if SpendingLimitPresets[i] <= SpendingLimitPresets[i-1] {
			t.Errorf("presets not in ascending order at index %d: %d <= %d",
				i, SpendingLimitPresets[i], SpendingLimitPresets[i-1])
		}
	}
}
