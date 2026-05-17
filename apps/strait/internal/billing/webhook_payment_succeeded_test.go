package billing

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"
)

// invoice.paid must dispatch billing.payment_succeeded when the org was
// actually in grace (or restricted) state — i.e. payment recovery.
func TestHandlePaymentSucceeded_DispatchesPaymentSucceeded_OnRecovery(t *testing.T) {
	t.Parallel()

	const orgID = "00000000-0000-0000-0000-300000000001"
	graceEnd := time.Now().Add(48 * time.Hour)
	stripeSubID := "sub_recover_1"
	stripeCustID := "cust_recover_1"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			orgID: {
				OrgID:                orgID,
				PlanTier:             "pro",
				Status:               "active",
				PaymentStatus:        "grace",
				GracePeriodEnd:       &graceEnd,
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
		ID:         "in_recover_1",
		CustomerID: stripeCustID,
		SubID:      stripeSubID,
		Metadata:   map[string]string{"org_id": orgID},
	})
	rr := fireWebhook(t, h, "invoice.paid", data)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var count int
	var captured fakeDispatchCall
	for _, c := range d.calls {
		if c.eventType == domain.WebhookEventBillingPaymentSucceeded {
			count++
			captured = c
		}
	}
	if count != 1 {
		t.Fatalf("billing.payment_succeeded dispatched %d times, want 1; saw event types %v", count, dispatchedEventTypes(d))
	}
	if captured.orgID != orgID {
		t.Errorf("dispatched org_id = %q, want %q", captured.orgID, orgID)
	}

	var env BillingEventEnvelope
	if err := json.Unmarshal(captured.payload, &env); err != nil {
		t.Fatalf("envelope unmarshal: %v", err)
	}
	if env.OrgID != orgID {
		t.Errorf("env.org_id = %q, want %q", env.OrgID, orgID)
	}
	if env.PlanTier != "pro" {
		t.Errorf("env.plan_tier = %q, want pro", env.PlanTier)
	}
	detail := env.Detail
	for _, key := range []string{"stripe_invoice_id", "stripe_subscription_id", "plan_tier", "paid_at"} {
		if _, ok := detail[key]; !ok {
			t.Errorf("detail missing %q; got %v", key, detail)
		}
	}
	if detail["stripe_invoice_id"] != "in_recover_1" {
		t.Errorf("detail.stripe_invoice_id = %v, want in_recover_1", detail["stripe_invoice_id"])
	}
}

// When the org is already ok (routine renewal payment), no dispatch fires —
// only state-change recoveries notify subscribers.
func TestHandlePaymentSucceeded_NoDispatch_WhenAlreadyOK(t *testing.T) {
	t.Parallel()

	const orgID = "00000000-0000-0000-0000-300000000002"
	stripeSubID := "sub_renewal_1"
	stripeCustID := "cust_renewal_1"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			orgID: {
				OrgID:                orgID,
				PlanTier:             "pro",
				Status:               "active",
				PaymentStatus:        "ok",
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
		ID:         "in_renewal_1",
		CustomerID: stripeCustID,
		SubID:      stripeSubID,
		Metadata:   map[string]string{"org_id": orgID},
	})
	rr := fireWebhook(t, h, "invoice.paid", data)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	for _, c := range d.calls {
		if c.eventType == domain.WebhookEventBillingPaymentSucceeded {
			t.Fatalf("did not expect billing.payment_succeeded on routine renewal; saw event types %v", dispatchedEventTypes(d))
		}
	}
}

// When the WebhookHandler is constructed without an enforcer (community
// edition), invoice.paid must still succeed without panicking, and no
// dispatch is emitted.
func TestHandlePaymentSucceeded_NoEnforcer_DoesNotPanic(t *testing.T) {
	t.Parallel()

	const orgID = "00000000-0000-0000-0000-300000000003"
	graceEnd := time.Now().Add(48 * time.Hour)
	stripeSubID := "sub_no_enf"
	stripeCustID := "cust_no_enf"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			orgID: {
				OrgID:                orgID,
				PlanTier:             "pro",
				Status:               "active",
				PaymentStatus:        "grace",
				GracePeriodEnd:       &graceEnd,
				StripeSubscriptionID: &stripeSubID,
				StripeCustomerID:     &stripeCustID,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	h := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	data := mustJSON(t, testInvoiceDataFull{
		ID:         "in_no_enf",
		CustomerID: stripeCustID,
		SubID:      stripeSubID,
		Metadata:   map[string]string{"org_id": orgID},
	})
	rr := fireWebhook(t, h, "invoice.paid", data)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}
