package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"
)

// QuotaWebhookNotifier sends webhook notifications for quota-related events.
type QuotaWebhookNotifier interface {
	// NotifyQuotaExceeded enqueues webhook deliveries for the quota.exceeded event.
	NotifyQuotaExceeded(ctx context.Context, orgID string, payload json.RawMessage) error
	// NotifyCronPausedQuota enqueues webhook deliveries for the cron.paused_quota event.
	NotifyCronPausedQuota(ctx context.Context, orgID string, payload json.RawMessage) error
	// NotifyCronResumed enqueues webhook deliveries for the cron.resumed event.
	NotifyCronResumed(ctx context.Context, orgID string, payload json.RawMessage) error
}

// quotaOrgStore resolves the set of project IDs for an org. Implemented by
// billing.PgStore which returns project IDs as plain strings.
type quotaOrgStore interface {
	ListProjectsByOrg(ctx context.Context, orgID string) ([]string, error)
}

// quotaDeliveryStore creates webhook deliveries and resolves subscriptions.
// Implemented by store.Queries.
type quotaDeliveryStore interface {
	ListWebhookSubscriptions(ctx context.Context, projectID string) ([]domain.WebhookSubscription, error)
	CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
}

// QuotaWebhookAdapter implements QuotaWebhookNotifier by creating
// WebhookDelivery records for all projects belonging to an org that have
// active subscriptions matching the emitted event type.
type QuotaWebhookAdapter struct {
	orgStore      quotaOrgStore
	deliveryStore quotaDeliveryStore
	logger        *slog.Logger
}

// Verify interface compliance.
var _ QuotaWebhookNotifier = (*QuotaWebhookAdapter)(nil)

// Ensure *billing.PgStore satisfies quotaOrgStore.
var _ quotaOrgStore = (*billing.PgStore)(nil)

// Ensure *store.Queries satisfies quotaDeliveryStore.
var _ quotaDeliveryStore = (*store.Queries)(nil)

// NewQuotaWebhookAdapter creates a new adapter.
func NewQuotaWebhookAdapter(orgStore quotaOrgStore, deliveryStore quotaDeliveryStore) *QuotaWebhookAdapter {
	return &QuotaWebhookAdapter{
		orgStore:      orgStore,
		deliveryStore: deliveryStore,
		logger:        slog.Default(),
	}
}

// NotifyQuotaExceeded enqueues webhook deliveries for quota.exceeded events
// across all projects in the org that subscribe to that event type.
func (a *QuotaWebhookAdapter) NotifyQuotaExceeded(ctx context.Context, orgID string, payload json.RawMessage) error {
	return a.notifyOrgEvent(ctx, orgID, domain.WebhookEventQuotaExceeded, payload)
}

// NotifyCronPausedQuota enqueues webhook deliveries for cron.paused_quota events.
func (a *QuotaWebhookAdapter) NotifyCronPausedQuota(ctx context.Context, orgID string, payload json.RawMessage) error {
	return a.notifyOrgEvent(ctx, orgID, domain.WebhookEventCronPausedQuota, payload)
}

// NotifyCronResumed enqueues webhook deliveries for cron.resumed events.
func (a *QuotaWebhookAdapter) NotifyCronResumed(ctx context.Context, orgID string, payload json.RawMessage) error {
	return a.notifyOrgEvent(ctx, orgID, domain.WebhookEventCronResumed, payload)
}

// notifyOrgEvent fans out a webhook delivery to all projects in the org that
// have an active subscription matching eventType.
func (a *QuotaWebhookAdapter) notifyOrgEvent(ctx context.Context, orgID, eventType string, payload json.RawMessage) error {
	projectIDs, err := a.orgStore.ListProjectsByOrg(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list projects for org %s: %w", orgID, err)
	}

	for _, projectID := range projectIDs {
		subs, err := a.deliveryStore.ListWebhookSubscriptions(ctx, projectID)
		if err != nil {
			a.logger.Warn("quota webhook adapter: failed to list subscriptions",
				"project_id", projectID, "error", err)
			continue
		}

		for _, sub := range subs {
			if !sub.Active || !containsEventType(sub.EventTypes, eventType) {
				continue
			}

			now := time.Now().UTC()
			delivery := &domain.WebhookDelivery{
				SubscriptionID: sub.ID,
				ProjectID:      sub.ProjectID,
				WebhookURL:     sub.WebhookURL,
				RetryPolicy:    domain.WebhookRetryPolicyExponential,
				Status:         domain.WebhookStatusPending,
				Attempts:       0,
				MaxAttempts:    3,
				NextRetryAt:    &now,
				Payload:        payload,
			}

			if err := a.deliveryStore.CreateWebhookDelivery(ctx, delivery); err != nil {
				a.logger.Warn("quota webhook adapter: failed to create delivery",
					"project_id", projectID,
					"subscription_id", sub.ID,
					"event_type", eventType,
					"error", err,
				)
				continue
			}

			a.logger.Info("quota webhook adapter: enqueued delivery",
				"project_id", projectID,
				"subscription_id", sub.ID,
				"event_type", eventType,
				"delivery_id", delivery.ID,
			)
		}
	}

	return nil
}
