package health

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"
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
	if result.Status != StatusUp {
		t.Fatalf("status = %q, want %q", result.Status, StatusUp)
	}
	if len(result.Components) != 1 {
		t.Fatalf("components = %d, want 1", len(result.Components))
	}

	// Verify the component result fields are sane even with very fast execution.
	comp := result.Components[0]
	if comp.Latency < 0 {
		t.Errorf("latency = %v, should not be negative", comp.Latency)
	}
	if comp.LatencyMs < 0 {
		t.Errorf("latency_ms = %d, should not be negative", comp.LatencyMs)
	}

	// Test with a checker that returns an error message containing MaxFloat64.
	r2 := NewRegistry()
	r2.Register(NewChecker("max-float-error", func(_ context.Context) error {
		return fmt.Errorf("threshold exceeded: %f", math.MaxFloat64)
	}))

	result2 := r2.CheckAll(context.Background())
	if result2.Status != StatusDown {
		t.Fatalf("status = %q, want %q for error checker", result2.Status, StatusDown)
	}
	if result2.Components[0].Error == "" {
		t.Fatal("expected error message in component")
	}
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
	if result.Status != StatusDown {
		t.Fatalf("status = %q, want %q", result.Status, StatusDown)
	}
	if result.Components[0].Error == "" {
		t.Fatal("expected error message for NaN checker")
	}
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
	if result.Status != StatusUp {
		t.Fatalf("status = %q, want %q with %d checkers", result.Status, StatusUp, count)
	}
	if len(result.Components) != count {
		t.Fatalf("components = %d, want %d", len(result.Components), count)
	}

	// Verify all components reported up.
	for _, comp := range result.Components {
		if comp.Status != StatusUp {
			t.Errorf("component %q status = %q, want %q", comp.Name, comp.Status, StatusUp)
		}
	}
}

// TestRegistry_DuplicateNames verifies behavior when two checkers share the same name.
func TestRegistry_DuplicateNames(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(NewChecker("db", func(_ context.Context) error { return nil }))
	r.Register(NewChecker("db", func(_ context.Context) error { return errors.New("second db failed") }))

	result := r.CheckAll(context.Background())
	if len(result.Components) != 2 {
		t.Fatalf("components = %d, want 2 (duplicate names allowed)", len(result.Components))
	}

	// Both should have the same name.
	for _, comp := range result.Components {
		if comp.Name != "db" {
			t.Errorf("component name = %q, want %q", comp.Name, "db")
		}
	}

	// Overall status should be down because one checker failed and defaults to critical.
	if result.Status != StatusDown {
		t.Fatalf("status = %q, want %q (one duplicate failed)", result.Status, StatusDown)
	}
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

	if result.Status != StatusDown {
		t.Fatalf("status = %q, want %q for timed-out checker", result.Status, StatusDown)
	}
	// Should complete in roughly the timeout period, not hang forever.
	if elapsed > 2*time.Second {
		t.Fatalf("elapsed = %v, expected completion near the 100ms timeout", elapsed)
	}
	if result.Components[0].Error == "" {
		t.Fatal("expected error message for timed-out checker")
	}
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

		if len(result.Components) != 1 {
			t.Fatalf("components = %d, want 1", len(result.Components))
		}
		comp := result.Components[0]
		if comp.Name != name {
			t.Errorf("name = %q, want %q", comp.Name, name)
		}
		if shouldFail && errMsg != "" {
			if comp.Status != StatusDown {
				t.Errorf("status = %q, want %q for failing checker", comp.Status, StatusDown)
			}
		} else {
			if comp.Status != StatusUp {
				t.Errorf("status = %q, want %q for passing checker", comp.Status, StatusUp)
			}
		}
	})
}
