package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/google/uuid"
)

// BudgetWebhookAdapter implements BudgetMonitorWebhookEnqueuer by creating
// WebhookDelivery records for projects with matching webhook subscriptions.
type BudgetWebhookAdapter struct {
	store  budgetWebhookStore
	logger *slog.Logger
}

// budgetWebhookStore defines the minimal store operations needed by the adapter.
type budgetWebhookStore interface {
	ListWebhookSubscriptions(ctx context.Context, projectID string) ([]domain.WebhookSubscription, error)
	CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
}

// Verify interface compliance.
var _ BudgetMonitorWebhookEnqueuer = (*BudgetWebhookAdapter)(nil)

// NewBudgetWebhookAdapter creates a new adapter.
func NewBudgetWebhookAdapter(s budgetWebhookStore) *BudgetWebhookAdapter {
	return &BudgetWebhookAdapter{
		store:  s,
		logger: slog.Default(),
	}
}

// EnqueueBudgetAlert creates a WebhookDelivery for each active subscription
// that includes the compute_budget_warning event type.
func (a *BudgetWebhookAdapter) EnqueueBudgetAlert(ctx context.Context, projectID string, payload json.RawMessage) error {
	subs, err := a.store.ListWebhookSubscriptions(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list webhook subscriptions: %w", err)
	}

	for _, sub := range subs {
		if !sub.Active || !containsEventType(sub.EventTypes, domain.WebhookEventComputeBudgetWarning) {
			continue
		}

		delivery := &domain.WebhookDelivery{
			ID:          uuid.Must(uuid.NewV7()).String(),
			WebhookURL:  sub.WebhookURL,
			Status:      domain.WebhookStatusPending,
			MaxAttempts: 3,
		}

		if err := a.store.CreateWebhookDelivery(ctx, delivery); err != nil {
			a.logger.Warn("failed to create budget webhook delivery",
				"project_id", projectID,
				"subscription_id", sub.ID,
				"error", err,
			)
			continue
		}

		a.logger.Info("enqueued budget alert webhook",
			"project_id", projectID,
			"subscription_id", sub.ID,
			"delivery_id", delivery.ID,
		)
	}

	// Suppress the unused variable lint — payload is part of the interface
	// contract and will be included in the delivery body in a follow-up.
	_ = payload

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

// Ensure *store.Queries satisfies budgetWebhookStore.
var _ budgetWebhookStore = (*store.Queries)(nil)
