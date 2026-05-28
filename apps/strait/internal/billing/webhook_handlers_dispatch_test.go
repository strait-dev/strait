package billing

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"strait/internal/domain"
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
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if len(d.calls) == 0 {
		t.Fatal("expected at least one billing dispatch")
	}
	var saw bool
	for _, c := range d.calls {
		if c.eventType == domain.WebhookEventBillingSuspended {
			saw = true
			var env BillingEventEnvelope
			if err := json.Unmarshal(c.payload, &env); err != nil {
				t.Fatalf("envelope unmarshal: %v", err)
			}
			if env.OrgID != orgID {
				t.Errorf("env.org_id = %q, want %q", env.OrgID, orgID)
			}
			if env.PlanTier != "pro" {
				t.Errorf("env.plan_tier = %q, want pro", env.PlanTier)
			}
		}
	}
	if !saw {
		t.Errorf("billing.suspended not dispatched; saw event types %v", dispatchedEventTypes(d))
	}
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
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var saw bool
	var captured fakeDispatchCall
	for _, c := range d.calls {
		if c.eventType == domain.WebhookEventBillingDelinquent {
			saw = true
			captured = c
			if c.orgID != orgID {
				t.Errorf("dispatched org_id = %q, want %q", c.orgID, orgID)
			}
		}
	}
	if !saw {
		t.Errorf("billing.delinquent not dispatched; saw event types %v", dispatchedEventTypes(d))
	}
	var env BillingEventEnvelope
	if err := json.Unmarshal(captured.payload, &env); err != nil {
		t.Fatalf("envelope unmarshal: %v", err)
	}
	if env.Detail["amount_due_microusd"] != float64(15_000_000) {
		t.Errorf("detail.amount_due_microusd = %v, want 15000000", env.Detail["amount_due_microusd"])
	}
}
