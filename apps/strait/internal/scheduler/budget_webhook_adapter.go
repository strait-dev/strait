package scheduler

import (
	"context"
	"encoding/json"
)

// BudgetWebhookAdapter implements BudgetMonitorWebhookEnqueuer.
// The compute_budget_warning path was removed when run_compute_usage was dropped;
// this type is retained to satisfy existing wiring in cmd/strait/server.go.
type BudgetWebhookAdapter struct{}

// Verify interface compliance.
var _ BudgetMonitorWebhookEnqueuer = (*BudgetWebhookAdapter)(nil)

// NewBudgetWebhookAdapter creates a new adapter.
func NewBudgetWebhookAdapter() *BudgetWebhookAdapter {
	return &BudgetWebhookAdapter{}
}

// EnqueueBudgetAlert is a no-op since compute_budget_warning was removed in
// migration 000227.
func (a *BudgetWebhookAdapter) EnqueueBudgetAlert(_ context.Context, _ string, _ json.RawMessage) error {
	return nil
}

// containsEventType checks if a slice of event types contains the target.
func containsEventType(types []string, target string) bool {
	for _, t := range types {
		if t == target || t == "*" {
			return true
		}
	}
	return false
}
