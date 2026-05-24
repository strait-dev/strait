package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/google/uuid"
)

// SLOWebhookNotifier sends webhook notifications for SLO budget warnings.
type SLOWebhookNotifier interface {
	NotifySLOBudgetWarning(ctx context.Context, projectID string, payload json.RawMessage) error
}

// SLOWebhookAdapter implements SLOWebhookNotifier by creating WebhookDelivery
// records for projects with matching webhook subscriptions.
type SLOWebhookAdapter struct {
	store  sloWebhookStore
	logger *slog.Logger
}

// sloWebhookStore defines the minimal store operations needed by the adapter.
type sloWebhookStore interface {
	ListWebhookSubscriptions(ctx context.Context, projectID string) ([]domain.WebhookSubscription, error)
	CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
}

// Verify interface compliance.
var _ SLOWebhookNotifier = (*SLOWebhookAdapter)(nil)

// Ensure *store.Queries satisfies sloWebhookStore.
var _ sloWebhookStore = (*store.Queries)(nil)

// NewSLOWebhookAdapter creates a new adapter.
func NewSLOWebhookAdapter(s sloWebhookStore) *SLOWebhookAdapter {
	return &SLOWebhookAdapter{
		store:  s,
		logger: slog.Default(),
	}
}

// NotifySLOBudgetWarning creates a WebhookDelivery for each active subscription
// that includes the slo.budget_warning event type.
func (a *SLOWebhookAdapter) NotifySLOBudgetWarning(ctx context.Context, projectID string, payload json.RawMessage) error {
	subs, err := a.store.ListWebhookSubscriptions(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list webhook subscriptions: %w", err)
	}

	for _, sub := range subs {
		if !sub.Active || !containsEventType(sub.EventTypes, domain.WebhookEventSLOBudgetWarning) {
			continue
		}

		now := time.Now().UTC()
		delivery := &domain.WebhookDelivery{
			ID:             uuid.Must(uuid.NewV7()).String(),
			SubscriptionID: sub.ID,
			ProjectID:      projectID,
			WebhookURL:     sub.WebhookURL,
			Payload:        payload,
			RetryPolicy:    domain.WebhookRetryPolicyExponential,
			Status:         domain.WebhookStatusPending,
			Attempts:       0,
			MaxAttempts:    3,
			NextRetryAt:    &now,
		}

		if err := a.store.CreateWebhookDelivery(ctx, delivery); err != nil {
			a.logger.Warn("failed to create slo budget webhook delivery",
				"project_id", projectID,
				"subscription_id", sub.ID,
				"error", err,
			)
			continue
		}

		a.logger.Info("enqueued slo budget warning webhook",
			"project_id", projectID,
			"subscription_id", sub.ID,
			"delivery_id", delivery.ID,
		)
	}

	return nil
}
