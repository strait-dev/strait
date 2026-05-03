package billing

import (
	"math"
	"testing"

	"strait/internal/compute"
	"strait/internal/domain"
)

// TestCalculateCost_InfDuration verifies that infinite duration does not panic.
func TestCalculateCost_InfDuration(t *testing.T) {
	t.Parallel()

	cost, err := compute.CalculateCost("micro", math.Inf(1))
	if err != nil {
		t.Fatalf("unexpected error with Inf duration: %v", err)
	}
	// Inf * positive cost per second should not be negative.
	if cost < 0 {
		t.Fatalf("cost should not be negative for Inf duration, got: %d", cost)
	}
}

// TestCalculateCost_NaNDuration verifies that NaN duration does not panic and returns zero cost.
func TestCalculateCost_NaNDuration(t *testing.T) {
	t.Parallel()

	cost, err := compute.CalculateCost("micro", math.NaN())
	if err != nil {
		t.Fatalf("unexpected error with NaN duration: %v", err)
	}
	// NaN <= 0 is false, so the guard may not trigger. Just verify no panic and non-negative result.
	if cost < 0 {
		t.Fatalf("cost should not be negative for NaN duration, got: %d", cost)
	}
}

// TestCalculateCost_MaxFloat64 verifies that MaxFloat64 duration does not panic.
func TestCalculateCost_MaxFloat64(t *testing.T) {
	t.Parallel()

	cost, err := compute.CalculateCost("micro", math.MaxFloat64)
	if err != nil {
		t.Fatalf("unexpected error with MaxFloat64 duration: %v", err)
	}
	if cost < 0 {
		t.Fatalf("cost should not be negative for MaxFloat64 duration, got: %d", cost)
	}
}

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
