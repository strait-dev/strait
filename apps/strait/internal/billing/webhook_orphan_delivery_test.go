package billing

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"strait/internal/telemetry"

	"github.com/stripe/stripe-go/v82"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// newOrphanTestEnforcer wires a real Enforcer with a manual-reader meter so
// tests can assert the WebhookOrphanDelivery counter actually increments.
func newOrphanTestEnforcer(t *testing.T) (*Enforcer, *metric.ManualReader) {
	t.Helper()
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := provider.Meter("billing-orphan-test")

	orphan, err := meter.Int64Counter("strait.billing.webhook_orphan_delivery_total")
	if err != nil {
		t.Fatalf("create orphan counter: %v", err)
	}
	m := &telemetry.Metrics{WebhookOrphanDelivery: orphan}

	enforcer := NewEnforcer(&mockBillingStore{}, nil, slog.Default(), WithMetrics(m))
	return enforcer, reader
}

// orphanCounterValue scrapes the manual reader for the orphan counter total
// across all attribute permutations. Returns the per-event-type breakdown so
// tests can assert which exact handler emitted.
func orphanCounterValue(t *testing.T, reader *metric.ManualReader) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	out := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "strait.billing.webhook_orphan_delivery_total" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("orphan metric is not int64 Sum: %T", m.Data)
			}
			for _, dp := range sum.DataPoints {
				ev := orphanAttr(dp.Attributes, "event_type")
				out[ev] += dp.Value
			}
		}
	}
	return out
}

func orphanAttr(set attribute.Set, key string) string {
	v, ok := set.Value(attribute.Key(key))
	if !ok {
		return ""
	}
	return v.AsString()
}

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
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestRecordOrphanInvoiceDelivery_NilEnforcerNoPanic guards the cheap path:
// a webhook handler constructed without an enforcer (community edition,
// tests, etc.) must not crash when an orphan event arrives.
func TestRecordOrphanInvoiceDelivery_NilEnforcerNoPanic(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&mockBillingStore{}, NewStripeMapping("starter-id", "", "pro-id", ""), nil)
	if h.enforcer != nil {
		t.Fatal("test setup: expected nil enforcer")
	}
	h.recordOrphanInvoiceDelivery(context.Background(), "invoice.finalized", &stripe.Invoice{
		ID:       "inv_no_enforcer",
		Customer: &stripe.Customer{ID: "cust_no_enforcer"},
	})
}

// TestHandleInvoiceFinalized_OrphanEmitsMetric proves the new telemetry hook
// is wired: a finalize event arriving for an unknown customer increments
// the WebhookOrphanDelivery counter under the canonical event_type label.
func TestHandleInvoiceFinalized_OrphanEmitsMetric(t *testing.T) {
	t.Parallel()

	enforcer, reader := newOrphanTestEnforcer(t)
	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	audit := &mockAuditStore{}
	h := newTestHandler(store, mapping, audit)
	h.enforcer = enforcer

	rr := fireWebhook(t, h, "invoice.finalized", orphanInvoicePayload(t, "inv_orphan_fin", "cust_orphan_x"))
	if rr.Code != http.StatusOK {
		t.Errorf("orphan delivery must still return 200 (Stripe retries 5xx), got %d", rr.Code)
	}
	if len(audit.events) != 0 {
		t.Errorf("orphan delivery must NOT emit an audit event, got %d", len(audit.events))
	}

	got := orphanCounterValue(t, reader)
	if got["invoice.finalized"] != 1 {
		t.Errorf("expected 1 orphan increment for invoice.finalized, got %v", got)
	}
}

// TestHandleInvoicePaid_OrphanEmitsMetric proves the same wiring for
// invoice.paid — the second of three invoice handlers we instrumented.
func TestHandleInvoicePaid_OrphanEmitsMetric(t *testing.T) {
	t.Parallel()

	enforcer, reader := newOrphanTestEnforcer(t)
	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	h := newTestHandler(store, mapping, nil)
	h.enforcer = enforcer

	rr := fireWebhook(t, h, "invoice.paid", orphanInvoicePayload(t, "inv_orphan_paid", "cust_orphan_y"))
	if rr.Code != http.StatusOK {
		t.Errorf("orphan delivery must still return 200, got %d", rr.Code)
	}
	got := orphanCounterValue(t, reader)
	if got["invoice.paid"] != 1 {
		t.Errorf("expected 1 orphan increment for invoice.paid, got %v", got)
	}
}

// TestHandleInvoicePaymentFailed_OrphanEmitsMetric proves the same wiring
// for invoice.payment_failed — the third of three invoice handlers.
func TestHandleInvoicePaymentFailed_OrphanEmitsMetric(t *testing.T) {
	t.Parallel()

	enforcer, reader := newOrphanTestEnforcer(t)
	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	h := newTestHandler(store, mapping, nil)
	h.enforcer = enforcer

	rr := fireWebhook(t, h, "invoice.payment_failed", orphanInvoicePayload(t, "inv_orphan_failed", "cust_orphan_z"))
	if rr.Code != http.StatusOK {
		t.Errorf("orphan delivery must still return 200, got %d", rr.Code)
	}
	got := orphanCounterValue(t, reader)
	if got["invoice.payment_failed"] != 1 {
		t.Errorf("expected 1 orphan increment for invoice.payment_failed, got %v", got)
	}
}

// TestHandleInvoiceFinalized_KnownCustomerNoOrphanMetric proves the metric
// is scoped: a finalize event with a resolvable org_id increments NOTHING
// on the orphan counter.
func TestHandleInvoiceFinalized_KnownCustomerNoOrphanMetric(t *testing.T) {
	t.Parallel()

	enforcer, reader := newOrphanTestEnforcer(t)
	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	audit := &mockAuditStore{}
	h := newTestHandler(store, mapping, audit)
	h.enforcer = enforcer

	data := mustJSON(t, testInvoiceDataFull{
		ID:         "inv_known_finalize",
		CustomerID: "cust_known_finalize",
		SubID:      "sub_known_finalize",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-aaaa00000001"},
		AmountDue:  4900,
	})
	rr := fireWebhook(t, h, "invoice.finalized", data)
	if rr.Code != http.StatusOK {
		t.Fatalf("known finalize: expected 200, got %d", rr.Code)
	}
	if len(audit.events) == 0 {
		t.Fatal("expected audit event for known customer")
	}
	got := orphanCounterValue(t, reader)
	if total := got["invoice.finalized"]; total != 0 {
		t.Errorf("known customer must not increment orphan counter, got %d", total)
	}
}
