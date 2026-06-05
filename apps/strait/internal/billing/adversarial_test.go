package billing

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
			name:     "free tier limit",
			tier:     domain.PlanFree,
			expected: 50000000,
		},
		{
			name:     "starter tier limit",
			tier:     domain.PlanStarter,
			expected: 100000000,
		},
		{
			name:     "pro tier limit",
			tier:     domain.PlanPro,
			expected: 200000000,
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
			require.Equal(t,
				tt.
					expected, got)

		})
	}
}
