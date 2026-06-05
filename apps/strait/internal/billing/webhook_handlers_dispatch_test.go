package billing

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Stripe customer.subscription.paused must dispatch billing.suspended via the
// enforcer's BillingEventDispatcher. The handler already updates status and
// logs an audit row; this guard ensures the outbound webhook follows.
func TestHandleSubscriptionPaused_DispatchesSuspended(t *testing.T) {
	t.Parallel()

	const orgID = "00000000-0000-0000-0000-200000000001"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			orgID: {OrgID: orgID, PlanTier: "pro", Status: "active"},
		},
	}
	d := &fakeDispatcher{}
	enf := NewEnforcer(store, nil, nil, WithBillingDispatcher(d))
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	h := NewWebhookHandler(store, mapping, "", slog.Default(), enf, nil, WithDevBypassSignatureCheck())

	data := mustJSON(t, testSubscriptionData{
		ID:         "sub_paused_dispatch",
		ProductID:  "pro-id",
		CustomerID: "cust_paused_dispatch",
		Metadata:   map[string]string{"org_id": orgID},
	})
	rr := fireWebhook(t, h, "customer.subscription.paused", data)
	require.Equal(t, http.StatusOK,
		rr.
			Code)
	require.NotEmpty(t, d.calls)

	var saw bool
	for _, c := range d.calls {
		if c.eventType == domain.WebhookEventBillingSuspended {
			saw = true
			var env BillingEventEnvelope
			require.NoError(t, json.
				Unmarshal(c.
					payload, &env,
				))
			assert.Equal(t, orgID,
				env.OrgID)
			assert.Equal(t, "pro",
				env.PlanTier,
			)
		}
	}
	assert.True(t, saw)
}

// invoice.payment_failed must dispatch billing.delinquent.
func TestHandlePaymentFailed_DispatchesDelinquent(t *testing.T) {
	t.Parallel()

	const orgID = "00000000-0000-0000-0000-200000000002"
	stripeSubID := "sub_pf_dispatch"
	stripeCustID := "cust_pf_dispatch"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			orgID: {
				OrgID:                orgID,
				PlanTier:             "pro",
				Status:               "active",
				StripeSubscriptionID: &stripeSubID,
				StripeCustomerID:     &stripeCustID,
			},
		},
	}
	d := &fakeDispatcher{}
	enf := NewEnforcer(store, nil, nil, WithBillingDispatcher(d))
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	h := NewWebhookHandler(store, mapping, "", slog.Default(), enf, nil, WithDevBypassSignatureCheck())

	data := mustJSON(t, testInvoiceDataFull{
		ID:         "in_pf_dispatch",
		CustomerID: stripeCustID,
		SubID:      stripeSubID,
		Metadata:   map[string]string{"org_id": orgID},
		AmountDue:  1500,
	})
	rr := fireWebhook(t, h, "invoice.payment_failed", data)
	require.Equal(t, http.StatusOK,
		rr.
			Code)

	var saw bool
	var captured fakeDispatchCall
	for _, c := range d.calls {
		if c.eventType == domain.WebhookEventBillingDelinquent {
			saw = true
			captured = c
			assert.Equal(t, orgID,
				c.orgID)
		}
	}
	assert.True(t, saw)

	var env BillingEventEnvelope
	require.NoError(t, json.
		Unmarshal(captured.
			payload,
			&env))
	assert.InDelta(t, float64(15_000_000),
		env.Detail["amount_due_microusd"], 1e-9)
}
