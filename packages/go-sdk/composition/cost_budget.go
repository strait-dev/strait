package composition

import (
	"fmt"

	strait "github.com/strait-dev/go-sdk"
)

// CostBudgetOptions configures cost budget tracking.
type CostBudgetOptions struct {
	MaxCostMicrousd  int64
	OnWarning        func(current, max int64)
	WarningThreshold float64 // default 0.8
}

// CostTracker tracks accumulated costs against a budget.
type CostTracker struct {
	currentCost  int64
	options      CostBudgetOptions
	warningFired bool
}

// NewCostTracker creates a new cost tracker.
func NewCostTracker(opts CostBudgetOptions) *CostTracker {
	if opts.WarningThreshold == 0 {
		opts.WarningThreshold = 0.8
	}
	return &CostTracker{options: opts}
}

// Add adds cost and checks budget. Returns error if exceeded.
func (t *CostTracker) Add(costMicrousd int64) error {
	t.currentCost += costMicrousd

	threshold := int64(float64(t.options.MaxCostMicrousd) * t.options.WarningThreshold)
	if !t.warningFired && t.options.OnWarning != nil && t.currentCost >= threshold {
		t.warningFired = true
		t.options.OnWarning(t.currentCost, t.options.MaxCostMicrousd)
	}

	if t.currentCost >= t.options.MaxCostMicrousd {
		return &strait.CostBudgetExceededError{
			Message:             fmt.Sprintf("Cost budget exceeded: %d >= %d microusd", t.currentCost, t.options.MaxCostMicrousd),
			CurrentCostMicrousd: t.currentCost,
			MaxCostMicrousd:     t.options.MaxCostMicrousd,
		}
	}
	return nil
}

// Current returns the accumulated cost.
func (t *CostTracker) Current() int64 { return t.currentCost }

// Remaining returns the remaining budget.
func (t *CostTracker) Remaining() int64 {
	r := t.options.MaxCostMicrousd - t.currentCost
	if r < 0 {
		return 0
	}
	return r
}

// IsExceeded returns true if the budget is exceeded.
func (t *CostTracker) IsExceeded() bool { return t.currentCost >= t.options.MaxCostMicrousd }

// WithCostBudget creates a tracker and passes it to fn.
func WithCostBudget[T any](fn func(tracker *CostTracker) (T, error), opts CostBudgetOptions) (T, error) {
	tracker := NewCostTracker(opts)
	return fn(tracker)
}
