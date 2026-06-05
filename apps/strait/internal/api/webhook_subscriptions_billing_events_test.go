package api

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
)

// Lock the registered outbound webhook event-type set. Adding a new
// type without registering it here will silently drop subscriptions
// to it; removing one is a wire-incompatible change.
func TestValidWebhookEventTypes_BillingEventsRegistered(t *testing.T) {
	t.Parallel()

	required := []string{
		domain.WebhookEventBillingCapWarning,
		domain.WebhookEventBillingCapReached,
		domain.WebhookEventBillingCapDisabled,
		domain.WebhookEventBillingOverageDisabled,
		domain.WebhookEventBillingSuspended,
		domain.WebhookEventBillingDelinquent,
		domain.WebhookEventBillingPaymentSucceeded,
		domain.WebhookEventScheduleSuspended,
		domain.WebhookEventWorkflowRegistrationRejected,
		domain.WebhookEventSLACreditIssued,
	}
	for _, ev := range required {
		assert.True(t,

			validWebhookEventTypes[ev])
	}
}
