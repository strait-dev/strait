package health

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealthScore_ExtremeValues verifies registry behavior when checkers return extreme latencies.
func TestHealthScore_ExtremeValues(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	// Register a checker that always succeeds but simulates extreme conditions.
	r.Register(NewChecker("extreme", func(_ context.Context) error {
		return nil
	}))

	result := r.CheckAll(context.Background())
	require.Equal(t, StatusUp, result.Status)
	require.Len(t, result.Components, 1)

	// Verify the component result fields are sane even with very fast execution.
	comp := result.Components[0]
	assert.GreaterOrEqual(t, comp.Latency, time.Duration(0))
	assert.GreaterOrEqual(t, comp.LatencyMs, int64(0))

	// Test with a checker that returns an error message containing MaxFloat64.
	r2 := NewRegistry()
	r2.Register(NewChecker("max-float-error", func(_ context.Context) error {
		return fmt.Errorf("threshold exceeded: %f", math.MaxFloat64)
	}))

	result2 := r2.CheckAll(context.Background())
	require.Equal(t, StatusDown, result2.Status)
	require.Len(t, result2.Components, 1)
	assert.NotEmpty(t, result2.Components[0].Error)
}

// TestHealthScore_NaN verifies registry handles NaN-related edge cases gracefully.
func TestHealthScore_NaN(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	// Register a checker whose error includes NaN.
	r.Register(NewChecker("nan-check", func(_ context.Context) error {
		val := math.NaN()
		if math.IsNaN(val) {
			return fmt.Errorf("metric is NaN: %f", val)
		}
		return nil
	}))

	result := r.CheckAll(context.Background())
	require.Equal(t, StatusDown, result.Status)
	require.Len(t, result.Components, 1)
	assert.NotEmpty(t, result.Components[0].Error)
}

// TestRegistry_ManyCheckers verifies the registry handles 1000 concurrent checkers.
func TestRegistry_ManyCheckers(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	const count = 1000
	for i := range count {
		idx := i
		r.Register(NewChecker(fmt.Sprintf("checker-%d", idx), func(_ context.Context) error {
			return nil
		}))
	}

	result := r.CheckAll(context.Background())
	require.Equal(t, StatusUp, result.Status)
	require.Len(t, result.Components, count)

	// Verify all components reported up.
	for _, comp := range result.Components {
		assert.Equal(t, StatusUp, comp.Status, "component %q", comp.Name)
	}
}

// TestRegistry_DuplicateNames verifies behavior when two checkers share the same name.
func TestRegistry_DuplicateNames(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(NewChecker("db", func(_ context.Context) error { return nil }))
	r.Register(NewChecker("db", func(_ context.Context) error { return errors.New("second db failed") }))

	result := r.CheckAll(context.Background())
	require.Len(t, result.Components, 2)

	// Both should have the same name.
	for _, comp := range result.Components {
		assert.Equal(t, "db", comp.Name)
	}

	// Overall status should be down because one checker failed and defaults to critical.
	require.Equal(t, StatusDown, result.Status)
}

// TestChecker_Timeout verifies that a never-returning checker respects context cancellation.
func TestChecker_Timeout(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(NewChecker("stuck", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := r.CheckAll(ctx)
	elapsed := time.Since(start)

	require.Equal(t, StatusDown, result.Status)
	// Should complete in roughly the timeout period, not hang forever.
	require.LessOrEqual(t, elapsed, 2*time.Second)
	require.Len(t, result.Components, 1)
	assert.NotEmpty(t, result.Components[0].Error)
}

// FuzzHealthScoreComputation fuzzes checker registration and execution with varied inputs.
func FuzzHealthScoreComputation(f *testing.F) {
	f.Add("checker-name", true, "some error")
	f.Add("", false, "")
	f.Add("x", true, "")
	f.Add("critical-check", false, "connection refused")
	f.Add("\x00null\x00name", true, "null\x00error")

	f.Fuzz(func(t *testing.T, name string, shouldFail bool, errMsg string) {
		r := NewRegistry()
		r.Register(NewChecker(name, func(_ context.Context) error {
			if shouldFail && errMsg != "" {
				return errors.New(errMsg)
			}
			return nil
		}))

		// Must not panic.
		result := r.CheckAll(context.Background())

		require.Len(t, result.Components, 1)
		comp := result.Components[0]
		assert.Equal(t, name, comp.Name)
		if shouldFail && errMsg != "" {
			assert.Equal(t, StatusDown, comp.Status)
		} else {
			assert.Equal(t, StatusUp, comp.Status)
		}
	})
}
