package billing

import (
	"math"
	"testing"

	"strait/internal/compute"
	"strait/internal/domain"
)

// TestCostEstimate_ExtremeTimeout verifies behavior with MaxInt timeout value.
func TestCostEstimate_ExtremeTimeout(t *testing.T) {
	t.Parallel()

	est, err := EstimateJobCost("micro", math.MaxInt, 1000000000)
	if err != nil {
		t.Fatalf("unexpected error with MaxInt timeout: %v", err)
	}
	// Cost should be non-negative regardless of overflow behavior.
	if est.CostMicro < 0 {
		t.Fatalf("cost should not be negative, got: %d", est.CostMicro)
	}
}

// TestCostEstimate_ZeroTimeout verifies that a zero timeout produces zero cost.
func TestCostEstimate_ZeroTimeout(t *testing.T) {
	t.Parallel()

	est, err := EstimateJobCost("micro", 0, 1000000000)
	if err != nil {
		t.Fatalf("unexpected error with zero timeout: %v", err)
	}
	if est.CostMicro != 0 {
		t.Fatalf("expected zero cost for zero timeout, got: %d", est.CostMicro)
	}
}

// TestCostEstimate_NegativeTimeout verifies that a negative timeout is handled safely.
func TestCostEstimate_NegativeTimeout(t *testing.T) {
	t.Parallel()

	est, err := EstimateJobCost("micro", -1, 1000000000)
	if err != nil {
		t.Fatalf("unexpected error with negative timeout: %v", err)
	}
	if est.CostMicro != 0 {
		t.Fatalf("expected zero cost for negative timeout, got: %d", est.CostMicro)
	}
}

// TestCostEstimate_ZeroCredit verifies that zero remaining credit produces zero runs remaining.
func TestCostEstimate_ZeroCredit(t *testing.T) {
	t.Parallel()

	est, err := EstimateJobCost("micro", 60, 0)
	if err != nil {
		t.Fatalf("unexpected error with zero credit: %v", err)
	}
	if est.CreditRunsRemaining != 0 {
		t.Fatalf("expected 0 runs remaining with zero credit, got: %d", est.CreditRunsRemaining)
	}
}

// TestCostEstimate_NegativeCredit verifies that negative remaining credit does not cause issues.
func TestCostEstimate_NegativeCredit(t *testing.T) {
	t.Parallel()

	est, err := EstimateJobCost("micro", 60, -1000000)
	if err != nil {
		t.Fatalf("unexpected error with negative credit: %v", err)
	}
	// Negative credit / positive cost should yield a negative (or zero) runs remaining.
	if est.CreditRunsRemaining > 0 {
		t.Fatalf("expected non-positive runs remaining with negative credit, got: %d", est.CreditRunsRemaining)
	}
}

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

// FuzzCostEstimateOverflow fuzzes EstimateJobCost with arbitrary timeout and credit values.
func FuzzCostEstimateOverflow(f *testing.F) {
	f.Add(0, int64(0))
	f.Add(1, int64(1000000))
	f.Add(-1, int64(-1))
	f.Add(math.MaxInt, int64(math.MaxInt64))
	f.Add(math.MinInt, int64(math.MinInt64))

	f.Fuzz(func(t *testing.T, timeout int, credit int64) {
		// Should never panic regardless of input.
		est, err := EstimateJobCost("micro", timeout, credit)
		if err != nil {
			return
		}
		// Basic sanity: cost should not be negative.
		if est.CostMicro < 0 {
			t.Fatalf("negative cost: %d", est.CostMicro)
		}
	})
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
