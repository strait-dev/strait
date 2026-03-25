package billing

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockBudgetAdversarialStore extends mockBillingStore with configurable
// budget and spend values for adversarial testing.
type mockBudgetAdversarialStore struct {
	mockBillingStore
	budget    int64
	action    string
	budgetErr error
	spend     int64
	spendErr  error
}

func (m *mockBudgetAdversarialStore) GetProjectBudget(_ context.Context, _ string) (int64, string, error) {
	return m.budget, m.action, m.budgetErr
}

func (m *mockBudgetAdversarialStore) GetProjectPeriodSpend(_ context.Context, _ string, _ time.Time) (int64, error) {
	return m.spend, m.spendErr
}

// TestBudget_ExactlyAtLimit verifies that spending exactly at the budget
// triggers rejection when action is "reject".
func TestBudget_ExactlyAtLimit(t *testing.T) {
	t.Parallel()

	store := &mockBudgetAdversarialStore{budget: 500000, action: "reject", spend: 500000}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckProjectBudgetLimit(context.Background(), "proj-at-limit")
	if err == nil {
		t.Fatal("expected error when spend equals budget with reject action")
	}

	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T: %v", err, err)
	}
	if le.Code != "project_budget_reached" {
		t.Errorf("expected code project_budget_reached, got %s", le.Code)
	}
}

// TestBudget_OneOverLimit verifies that spending one micro-USD over the budget
// triggers rejection.
func TestBudget_OneOverLimit(t *testing.T) {
	t.Parallel()

	store := &mockBudgetAdversarialStore{budget: 500000, action: "reject", spend: 500001}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckProjectBudgetLimit(context.Background(), "proj-over")
	if err == nil {
		t.Fatal("expected error when spend exceeds budget by 1 with reject action")
	}

	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T: %v", err, err)
	}
	if le.Code != "project_budget_reached" {
		t.Errorf("expected code project_budget_reached, got %s", le.Code)
	}
}

// TestBudget_ZeroBudget verifies that a zero budget with reject action blocks
// any spending.
func TestBudget_ZeroBudget(t *testing.T) {
	t.Parallel()

	store := &mockBudgetAdversarialStore{budget: 0, action: "reject", spend: 1}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckProjectBudgetLimit(context.Background(), "proj-zero-budget")
	if err == nil {
		t.Fatal("expected error for zero budget with any spend")
	}

	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T: %v", err, err)
	}

	// Also verify zero spend with zero budget still triggers (budget == 0 is special).
	store2 := &mockBudgetAdversarialStore{budget: 0, action: "reject", spend: 0}
	e2 := NewEnforcer(store2, nil, slog.Default())
	err = e2.CheckProjectBudgetLimit(context.Background(), "proj-zero-both")
	if err == nil {
		t.Fatal("expected error for zero budget even with zero spend (budget==0 triggers)")
	}
}

// TestBudget_NegativeBudget verifies that a negative budget means no limit is set.
func TestBudget_NegativeBudget(t *testing.T) {
	t.Parallel()

	store := &mockBudgetAdversarialStore{budget: -1, action: "reject", spend: 999999999}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckProjectBudgetLimit(context.Background(), "proj-neg-budget")
	if err != nil {
		t.Fatalf("expected nil error for negative budget (no limit), got %v", err)
	}
}

// TestBudget_MaxIntBudget verifies that a MaxInt64 budget does not overflow.
func TestBudget_MaxIntBudget(t *testing.T) {
	t.Parallel()

	store := &mockBudgetAdversarialStore{budget: math.MaxInt64, action: "reject", spend: math.MaxInt64 - 1}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckProjectBudgetLimit(context.Background(), "proj-maxint")
	if err != nil {
		t.Fatalf("expected nil error when spend < MaxInt64 budget, got %v", err)
	}

	// At exactly MaxInt64 spend should trigger.
	store2 := &mockBudgetAdversarialStore{budget: math.MaxInt64, action: "reject", spend: math.MaxInt64}
	e2 := NewEnforcer(store2, nil, slog.Default())
	err = e2.CheckProjectBudgetLimit(context.Background(), "proj-maxint-equal")
	if err == nil {
		t.Fatal("expected error when spend equals MaxInt64 budget")
	}
}

// TestBudget_FloatRounding verifies that budget enforcement works correctly
// with values that would cause float precision issues if converted.
func TestBudget_FloatRounding(t *testing.T) {
	t.Parallel()

	// 0.1 USD = 100000 micro-USD. Use values near boundaries where
	// float64 division might lose precision.
	cases := []struct {
		name   string
		budget int64
		spend  int64
		reject bool
	}{
		{"just-under", 100000, 99999, false},
		{"exactly-at", 100000, 100000, true},
		{"one-over", 100000, 100001, true},
		// 0.01 USD boundary.
		{"penny-under", 10000, 9999, false},
		{"penny-at", 10000, 10000, true},
		// Large value with precision risk.
		{"large-under", 1_000_000_001, 1_000_000_000, false},
		{"large-at", 1_000_000_001, 1_000_000_001, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &mockBudgetAdversarialStore{budget: tc.budget, action: "reject", spend: tc.spend}
			e := NewEnforcer(store, nil, slog.Default())

			err := e.CheckProjectBudgetLimit(context.Background(), "proj-float-"+tc.name)
			if tc.reject && err == nil {
				t.Fatalf("expected rejection for budget=%d spend=%d", tc.budget, tc.spend)
			}
			if !tc.reject && err != nil {
				t.Fatalf("expected pass for budget=%d spend=%d, got %v", tc.budget, tc.spend, err)
			}
		})
	}
}

// TestBudget_ConcurrentSpend verifies that 100 concurrent goroutines calling
// CheckProjectBudgetLimit do not cause data races or panics.
func TestBudget_ConcurrentSpend(t *testing.T) {
	t.Parallel()

	store := &mockBudgetAdversarialStore{budget: 100000, action: "reject", spend: 50000}
	e := NewEnforcer(store, nil, slog.Default())

	var wg sync.WaitGroup
	var errCount atomic.Int64

	for range 100 {
		wg.Go(func() {
			err := e.CheckProjectBudgetLimit(context.Background(), "proj-concurrent")
			if err != nil {
				errCount.Add(1)
			}
		})
	}

	wg.Wait()

	// All calls should pass since spend (50000) < budget (100000).
	if got := errCount.Load(); got != 0 {
		t.Fatalf("expected 0 errors for under-budget concurrent calls, got %d", got)
	}
}

// FuzzBudgetEnforcement fuzzes budget and spend combinations.
func FuzzBudgetEnforcement(f *testing.F) {
	f.Add(int64(100000), int64(50000))
	f.Add(int64(0), int64(0))
	f.Add(int64(-1), int64(999))
	f.Add(int64(math.MaxInt64), int64(math.MaxInt64))
	f.Add(int64(1), int64(0))

	f.Fuzz(func(t *testing.T, budget, spend int64) {
		store := &mockBudgetAdversarialStore{budget: budget, action: "reject", spend: spend}
		e := NewEnforcer(store, nil, slog.Default())

		// Should never panic regardless of input values.
		_ = e.CheckProjectBudgetLimit(context.Background(), "proj-fuzz")
	})
}
