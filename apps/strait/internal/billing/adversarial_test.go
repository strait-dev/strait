package billing

import (
	"testing"

	"strait/internal/domain"
)

// TestSpendingLimit_BoundaryValues verifies spending limit boundary checks for each plan tier.
func TestSpendingLimit_BoundaryValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tier     domain.PlanTier
		expected int64
	}{
		{
			name:     "free tier has zero limit",
			tier:     domain.PlanFree,
			expected: 0,
		},
		{
			name:     "starter tier limit",
			tier:     domain.PlanStarter,
			expected: 500000000,
		},
		{
			name:     "pro tier limit",
			tier:     domain.PlanPro,
			expected: 2000000000,
		},
		{
			name:     "enterprise tier has custom limit",
			tier:     domain.PlanEnterprise,
			expected: -1,
		},
		{
			name:     "unknown tier defaults to zero",
			tier:     domain.PlanTier("nonexistent"),
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := MaxSpendingLimit(tt.tier)
			if got != tt.expected {
				t.Fatalf("MaxSpendingLimit(%q) = %d, want %d", tt.tier, got, tt.expected)
			}
		})
	}
}
