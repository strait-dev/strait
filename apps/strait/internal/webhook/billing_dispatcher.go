package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"strait/internal/billing"
	"strait/internal/domain"
)

// Compile-time check: *BillingDispatcher satisfies the dispatcher contract
// declared on the billing side.
var _ billing.BillingEventDispatcher = (*BillingDispatcher)(nil)

// billingProjectLister resolves the set of project IDs belonging to an org.
// Implemented by *billing.PgStore.
type billingProjectLister interface {
	ListProjectsByOrg(ctx context.Context, orgID string) ([]string, error)
}

// billingSubscriptionLister resolves the active webhook subscriptions for a
// project. Implemented by *store.Queries.
type billingSubscriptionLister interface {
	ListWebhookSubscriptions(ctx context.Context, projectID string) ([]domain.WebhookSubscription, error)
}

// BillingDispatcher implements billing.BillingEventDispatcher by fanning an
// org-scoped billing event out to every project-level webhook subscription
// that matches the event type. The org → project → subscription resolution
// keeps the billing package free of webhook/store dependencies and reuses
// the existing DeliveryWorker enqueue path so deliveries share the well-formed
// retry/payload shape with run and event-trigger webhooks.
type BillingDispatcher struct {
	notifier *DeliveryWorker
	projects billingProjectLister
	subs     billingSubscriptionLister
	logger   *slog.Logger
}

// NewBillingDispatcher constructs a dispatcher bound to a DeliveryWorker.
func NewBillingDispatcher(
	notifier *DeliveryWorker,
	projects billingProjectLister,
	subs billingSubscriptionLister,
	logger *slog.Logger,
) *BillingDispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &BillingDispatcher{
		notifier: notifier,
		projects: projects,
		subs:     subs,
		logger:   logger,
	}
}

// DispatchBillingEvent satisfies billing.BillingEventDispatcher. It looks up
// the org's projects, lists each project's subscriptions, and hands the
// matching set to the DeliveryWorker for enqueue. Per-project enumeration
// errors are logged and skipped rather than aborting the dispatch — a billing
// event that reaches some subscribers is strictly better than one that
// reaches none.
func (d *BillingDispatcher) DispatchBillingEvent(ctx context.Context, orgID, eventType string, payload []byte) error {
	if d.notifier == nil {
		return fmt.Errorf("billing dispatcher: nil delivery worker")
	}
	if orgID == "" {
		return fmt.Errorf("billing dispatcher: empty org id")
	}
	if eventType == "" {
		return fmt.Errorf("billing dispatcher: empty event type")
	}

	projectIDs, err := d.projects.ListProjectsByOrg(ctx, orgID)
	if err != nil {
		return fmt.Errorf("billing dispatcher: list projects for org %s: %w", orgID, err)
	}

	raw := json.RawMessage(payload)
	for _, projectID := range projectIDs {
		subs, err := d.subs.ListWebhookSubscriptions(ctx, projectID)
		if err != nil {
			d.logger.Warn("billing dispatcher: list subscriptions failed",
				"org_id", orgID,
				"project_id", projectID,
				"event_type", eventType,
				"error", err,
			)
			continue
		}
		d.notifier.EnqueueSubscriptionWebhooks(ctx, subs, eventType, raw)
	}
	return nil
}
