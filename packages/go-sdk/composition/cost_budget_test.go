package composition

import (
	"errors"
	"testing"

	strait "github.com/strait-dev/go-sdk"
)

func TestNewCostTracker_DefaultThreshold(t *testing.T) {
	tracker := NewCostTracker(CostBudgetOptions{MaxCostMicrousd: 10000})
	if tracker.options.WarningThreshold != 0.8 {
		t.Errorf("expected default threshold 0.8, got %f", tracker.options.WarningThreshold)
	}
}

func TestNewCostTracker_CustomThreshold(t *testing.T) {
	tracker := NewCostTracker(CostBudgetOptions{MaxCostMicrousd: 10000, WarningThreshold: 0.5})
	if tracker.options.WarningThreshold != 0.5 {
		t.Errorf("expected threshold 0.5, got %f", tracker.options.WarningThreshold)
	}
}

func TestCostTracker_Current(t *testing.T) {
	tracker := NewCostTracker(CostBudgetOptions{MaxCostMicrousd: 10000})
	if tracker.Current() != 0 {
		t.Errorf("expected initial current 0, got %d", tracker.Current())
	}

	_ = tracker.Add(3000)
	if tracker.Current() != 3000 {
		t.Errorf("expected current 3000, got %d", tracker.Current())
	}
}

func TestCostTracker_Add_UnderBudget(t *testing.T) {
	tracker := NewCostTracker(CostBudgetOptions{MaxCostMicrousd: 10000})

	err := tracker.Add(5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tracker.Current() != 5000 {
		t.Errorf("expected current 5000, got %d", tracker.Current())
	}
}

func TestCostTracker_Add_ExceedsBudget(t *testing.T) {
	tracker := NewCostTracker(CostBudgetOptions{MaxCostMicrousd: 10000})

	_ = tracker.Add(5000)
	err := tracker.Add(6000)
	if err == nil {
		t.Fatal("expected error when exceeding budget")
	}

	var budgetErr *strait.CostBudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatal("expected CostBudgetExceededError")
	}
	if budgetErr.CurrentCostMicrousd != 11000 {
		t.Errorf("expected current 11000, got %d", budgetErr.CurrentCostMicrousd)
	}
	if budgetErr.MaxCostMicrousd != 10000 {
		t.Errorf("expected max 10000, got %d", budgetErr.MaxCostMicrousd)
	}
}

func TestCostTracker_Add_ExactBudget(t *testing.T) {
	tracker := NewCostTracker(CostBudgetOptions{MaxCostMicrousd: 10000})

	err := tracker.Add(10000)
	if err == nil {
		t.Fatal("expected error when exactly at budget")
	}
}

func TestCostTracker_Remaining(t *testing.T) {
	tracker := NewCostTracker(CostBudgetOptions{MaxCostMicrousd: 10000})

	if tracker.Remaining() != 10000 {
		t.Errorf("expected remaining 10000, got %d", tracker.Remaining())
	}

	_ = tracker.Add(3000)
	if tracker.Remaining() != 7000 {
		t.Errorf("expected remaining 7000, got %d", tracker.Remaining())
	}
}

func TestCostTracker_Remaining_WhenExceeded(t *testing.T) {
	tracker := NewCostTracker(CostBudgetOptions{MaxCostMicrousd: 5000})

	_ = tracker.Add(8000)
	if tracker.Remaining() != 0 {
		t.Errorf("expected remaining 0 when exceeded, got %d", tracker.Remaining())
	}
}

func TestCostTracker_IsExceeded(t *testing.T) {
	tracker := NewCostTracker(CostBudgetOptions{MaxCostMicrousd: 10000})

	if tracker.IsExceeded() {
		t.Error("expected not exceeded initially")
	}

	_ = tracker.Add(5000)
	if tracker.IsExceeded() {
		t.Error("expected not exceeded at half budget")
	}

	_ = tracker.Add(5000)
	if !tracker.IsExceeded() {
		t.Error("expected exceeded at full budget")
	}
}

func TestCostTracker_Warning_Fired(t *testing.T) {
	warningCalled := false
	var warningCurrent, warningMax int64

	tracker := NewCostTracker(CostBudgetOptions{
		MaxCostMicrousd: 10000,
		OnWarning: func(current, max int64) {
			warningCalled = true
			warningCurrent = current
			warningMax = max
		},
	})

	// Under threshold (80%)
	_ = tracker.Add(7000)
	if warningCalled {
		t.Error("expected warning not called under threshold")
	}

	// At threshold
	_ = tracker.Add(1000)
	if !warningCalled {
		t.Error("expected warning called at threshold")
	}
	if warningCurrent != 8000 {
		t.Errorf("expected warning current 8000, got %d", warningCurrent)
	}
	if warningMax != 10000 {
		t.Errorf("expected warning max 10000, got %d", warningMax)
	}
}

func TestCostTracker_Warning_OnlyOnce(t *testing.T) {
	callCount := 0
	tracker := NewCostTracker(CostBudgetOptions{
		MaxCostMicrousd: 10000,
		OnWarning: func(current, max int64) {
			callCount++
		},
	})

	_ = tracker.Add(8500)
	_ = tracker.Add(500)

	if callCount != 1 {
		t.Errorf("expected warning called once, got %d", callCount)
	}
}

func TestCostTracker_NoWarning_WhenNilCallback(t *testing.T) {
	tracker := NewCostTracker(CostBudgetOptions{MaxCostMicrousd: 10000})

	// Should not panic
	_ = tracker.Add(9000)
}

func TestWithCostBudget_Success(t *testing.T) {
	result, err := WithCostBudget(func(tracker *CostTracker) (string, error) {
		err := tracker.Add(5000)
		if err != nil {
			return "", err
		}
		return "done", nil
	}, CostBudgetOptions{MaxCostMicrousd: 10000})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "done" {
		t.Errorf("expected 'done', got %q", result)
	}
}

func TestWithCostBudget_Exceeded(t *testing.T) {
	_, err := WithCostBudget(func(tracker *CostTracker) (string, error) {
		return "", tracker.Add(15000)
	}, CostBudgetOptions{MaxCostMicrousd: 10000})

	if err == nil {
		t.Fatal("expected error")
	}
	var budgetErr *strait.CostBudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatal("expected CostBudgetExceededError")
	}
}

func TestCostTracker_MultipleAdds(t *testing.T) {
	tracker := NewCostTracker(CostBudgetOptions{MaxCostMicrousd: 100000})

	for range 10 {
		_ = tracker.Add(5000)
	}

	if tracker.Current() != 50000 {
		t.Errorf("expected current 50000, got %d", tracker.Current())
	}
	if tracker.Remaining() != 50000 {
		t.Errorf("expected remaining 50000, got %d", tracker.Remaining())
	}
}
