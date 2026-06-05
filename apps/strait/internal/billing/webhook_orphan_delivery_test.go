package billing

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"
)

// orphanInvoicePayload synthesizes the minimum viable invoice JSON for an
// orphan delivery: a customer ID we don't know about and no metadata.
func orphanInvoicePayload(t *testing.T, invoiceID, customerID string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"id":         invoiceID,
		"customer":   map[string]any{"id": customerID},
		"amount_due": 4900,
		"currency":   "USD",
		"parent": map[string]any{
			"subscription_details": map[string]any{
				"subscription": map[string]any{"id": "sub_orphan"},
			},
		},
	})
	require.NoError(t,
		err)

	return b
}

// newOrphanTestEnforcer wires a real Enforcer so the orphan-delivery hook
// runs through its full code path. The metric increment itself is exercised
// at runtime via the package-level recordBillingWebhookOrphanDelivery helper
// metrics_build_tags_test.go for that side effect.
func newOrphanTestEnforcer(t *testing.T) *Enforcer {
	t.Helper()
	return NewEnforcer(&mockBillingStore{}, nil, slog.Default())
}

// TestRecordOrphanInvoiceDelivery_NilEnforcerNoPanic guards the cheap path:
// a webhook handler constructed without an enforcer (community edition,
// tests, etc.) must not crash when an orphan event arrives.
func TestRecordOrphanInvoiceDelivery_NilEnforcerNoPanic(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&mockBillingStore{}, NewStripeMapping("starter-id", "", "pro-id", ""), nil)
	require.Nil(t, h.enforcer)

	h.recordOrphanInvoiceDelivery(context.Background(), "invoice.finalized", &stripe.Invoice{
		ID:       "inv_no_enforcer",
		Customer: &stripe.Customer{ID: "cust_no_enforcer"},
	})
}

// TestHandleInvoiceFinalized_OrphanReturns200 proves orphan finalize events
// still return 200 (so Stripe doesn't retry) and never emit an audit event.
// The metric increment runs through recordBillingWebhookOrphanDelivery,
// covered by the build-tag no-panic test.
func TestHandleInvoiceFinalized_OrphanReturns200(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	audit := &mockAuditStore{}
	h := newTestHandler(store, mapping, audit)
	h.enforcer = newOrphanTestEnforcer(t)

	rr := fireWebhook(t, h, "invoice.finalized", orphanInvoicePayload(t, "inv_orphan_fin", "cust_orphan_x"))
	assert.Equal(t, http.
		StatusOK, rr.
		Code)
	assert.Empty(t, audit.
		events)
}

// TestHandleInvoicePaid_OrphanReturns200 proves the same shape for
// invoice.paid — the second of three invoice handlers we instrumented.
func TestHandleInvoicePaid_OrphanReturns200(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	h := newTestHandler(store, mapping, nil)
	h.enforcer = newOrphanTestEnforcer(t)

	rr := fireWebhook(t, h, "invoice.paid", orphanInvoicePayload(t, "inv_orphan_paid", "cust_orphan_y"))
	assert.Equal(t, http.
		StatusOK, rr.
		Code)
}

// TestHandleInvoicePaymentFailed_OrphanReturns200 proves the same shape for
// invoice.payment_failed — the third of three invoice handlers.
func TestHandleInvoicePaymentFailed_OrphanReturns200(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	h := newTestHandler(store, mapping, nil)
	h.enforcer = newOrphanTestEnforcer(t)

	rr := fireWebhook(t, h, "invoice.payment_failed", orphanInvoicePayload(t, "inv_orphan_failed", "cust_orphan_z"))
	assert.Equal(t, http.
		StatusOK, rr.
		Code)
}

// TestHandleInvoiceFinalized_KnownCustomerEmitsAudit proves the orphan path
// is scoped: a finalize event with a resolvable org_id DOES emit an audit
// event (the non-orphan branch).
func TestHandleInvoiceFinalized_KnownCustomerEmitsAudit(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	audit := &mockAuditStore{}
	h := newTestHandler(store, mapping, audit)
	h.enforcer = newOrphanTestEnforcer(t)

	data := mustJSON(t, testInvoiceDataFull{
		ID:         "inv_known_finalize",
		CustomerID: "cust_known_finalize",
		SubID:      "sub_known_finalize",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-aaaa00000001"},
		AmountDue:  4900,
	})
	rr := fireWebhook(t, h, "invoice.finalized", data)
	require.Equal(t,
		http.StatusOK, rr.
			Code)
	require.NotEmpty(
		t, audit.events)
}
