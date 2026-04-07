package billing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stripe/stripe-go/v82"
)

// Test helpers for invoice and dispute payloads.

// testInvoiceDataFull extends testInvoiceData with amount and date fields.
type testInvoiceDataFull struct {
	ID                 string            `json:"-"`
	CustomerID         string            `json:"-"`
	SubID              string            `json:"-"`
	Metadata           map[string]string `json:"-"`
	AmountDue          int64             `json:"-"`
	DueDate            int64             `json:"-"`
	NextPaymentAttempt int64             `json:"-"`
}

func (d testInvoiceDataFull) MarshalJSON() ([]byte, error) {
	type subDetail struct {
		Subscription *stripe.Subscription `json:"subscription"`
		Metadata     map[string]string    `json:"metadata,omitempty"`
	}
	type parent struct {
		SubscriptionDetails *subDetail `json:"subscription_details"`
	}
	type invoiceJSON struct {
		ID                 string           `json:"id"`
		Customer           *stripe.Customer `json:"customer,omitempty"`
		Parent             *parent          `json:"parent,omitempty"`
		AmountDue          int64            `json:"amount_due,omitempty"`
		DueDate            int64            `json:"due_date,omitempty"`
		NextPaymentAttempt int64            `json:"next_payment_attempt,omitempty"`
	}

	inv := invoiceJSON{
		ID:                 d.ID,
		AmountDue:          d.AmountDue,
		DueDate:            d.DueDate,
		NextPaymentAttempt: d.NextPaymentAttempt,
	}
	if d.CustomerID != "" {
		inv.Customer = &stripe.Customer{ID: d.CustomerID}
	}
	if d.SubID != "" || d.Metadata != nil {
		sub := &stripe.Subscription{ID: d.SubID, Metadata: d.Metadata}
		inv.Parent = &parent{
			SubscriptionDetails: &subDetail{
				Subscription: sub,
				Metadata:     d.Metadata,
			},
		}
	}
	return json.Marshal(inv)
}

// testDisputeData builds JSON compatible with stripe.Dispute.
type testDisputeData struct {
	ID         string `json:"-"`
	Amount     int64  `json:"-"`
	Reason     string `json:"-"`
	CustomerID string `json:"-"`
	ChargeID   string `json:"-"`
	// OrgID is placed in charge.customer.metadata.
	OrgID string `json:"-"`
}

func (d testDisputeData) MarshalJSON() ([]byte, error) {
	type customerJSON struct {
		ID       string            `json:"id"`
		Metadata map[string]string `json:"metadata,omitempty"`
	}
	type chargeJSON struct {
		ID       string        `json:"id"`
		Customer *customerJSON `json:"customer,omitempty"`
	}
	type disputeJSON struct {
		ID     string      `json:"id"`
		Amount int64       `json:"amount"`
		Reason string      `json:"reason,omitempty"`
		Charge *chargeJSON `json:"charge,omitempty"`
	}

	dj := disputeJSON{
		ID:     d.ID,
		Amount: d.Amount,
		Reason: d.Reason,
	}
	if d.ChargeID != "" || d.CustomerID != "" || d.OrgID != "" {
		cj := &customerJSON{ID: d.CustomerID}
		if d.OrgID != "" {
			cj.Metadata = map[string]string{"org_id": d.OrgID}
		}
		dj.Charge = &chargeJSON{
			ID:       d.ChargeID,
			Customer: cj,
		}
	}
	return json.Marshal(dj)
}

// fireWebhook is a helper that sends a webhook event through the handler and returns the response code.
func fireWebhook(t *testing.T, handler http.Handler, eventType string, data json.RawMessage) *httptest.ResponseRecorder {
	t.Helper()
	payload := StripeWebhookPayload{
		Type: eventType,
		Data: data,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// fireWebhookWithID sends a webhook event with an explicit event ID for idempotency testing.
func fireWebhookWithID(t *testing.T, handler http.Handler, eventID, eventType string, data json.RawMessage) *httptest.ResponseRecorder {
	t.Helper()
	payload := StripeWebhookPayload{
		ID:   eventID,
		Type: eventType,
		Data: data,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// newTestHandler creates a WebhookHandler with dev bypass and optional audit store.
func newTestHandler(store *mockBillingStore, mapping *StripeMapping, audit AuditStore, opts ...WebhookOption) *WebhookHandler {
	allOpts := []WebhookOption{WithDevBypassSignatureCheck()}
	allOpts = append(allOpts, opts...)
	return NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, allOpts...)
}

// Tests for handleSubscriptionPaused.

func TestWebhookHandler_SubscriptionPaused(t *testing.T) {
	t.Parallel()

	t.Run("happy_path", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-100000000001": {
					OrgID:    "00000000-0000-0000-0000-100000000001",
					PlanTier: "pro",
					Status:   "active",
				},
			},
		}
		audit := &mockAuditStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, audit)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_pause_1",
			ProductID:  "pro-id",
			CustomerID: "cust_pause_1",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-100000000001"},
		})

		rr := fireWebhook(t, handler, "customer.subscription.paused", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if store.lastPlanUpdate == nil {
			t.Fatal("expected plan update to be called")
		}
		if store.lastPlanUpdate.status != "paused" {
			t.Errorf("status = %q, want paused", store.lastPlanUpdate.status)
		}
		if len(audit.events) == 0 {
			t.Error("expected audit event to be recorded")
		}
	})

	t.Run("missing_org_id_returns_ok", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_pause_no_org",
			ProductID:  "pro-id",
			CustomerID: "cust_pause_no_org",
			Metadata:   map[string]string{},
		})

		rr := fireWebhook(t, handler, "customer.subscription.paused", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 when orgID empty, got %d", rr.Code)
		}
		if store.lastPlanUpdate != nil {
			t.Error("expected no plan update when orgID is empty")
		}
	})

	t.Run("subscription_not_found_returns_ok", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_pause_missing",
			ProductID:  "pro-id",
			CustomerID: "cust_pause_missing",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-100000000002"},
		})

		rr := fireWebhook(t, handler, "customer.subscription.paused", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 when subscription not found, got %d", rr.Code)
		}
	})

	t.Run("idempotent_duplicate_event", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-100000000003": {
					OrgID:    "00000000-0000-0000-0000-100000000003",
					PlanTier: "pro",
					Status:   "active",
				},
			},
		}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_pause_idem",
			ProductID:  "pro-id",
			CustomerID: "cust_pause_idem",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-100000000003"},
		})

		// First call succeeds.
		rr1 := fireWebhookWithID(t, handler, "evt_pause_dup_1", "customer.subscription.paused", data)
		if rr1.Code != http.StatusOK {
			t.Errorf("first call: expected 200, got %d", rr1.Code)
		}

		// Second call with same event ID is deduplicated by replay cache.
		rr2 := fireWebhookWithID(t, handler, "evt_pause_dup_1", "customer.subscription.paused", data)
		if rr2.Code != http.StatusOK {
			t.Errorf("duplicate call: expected 200, got %d", rr2.Code)
		}
	})
}

// Tests for handleSubscriptionResumed.

func TestWebhookHandler_SubscriptionResumed(t *testing.T) {
	t.Parallel()

	t.Run("happy_path", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-200000000001": {
					OrgID:    "00000000-0000-0000-0000-200000000001",
					PlanTier: "pro",
					Status:   "paused",
				},
			},
		}
		audit := &mockAuditStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, audit)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_resume_1",
			ProductID:  "pro-id",
			CustomerID: "cust_resume_1",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-200000000001"},
		})

		rr := fireWebhook(t, handler, "customer.subscription.resumed", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if store.lastFullUpdate == nil {
			t.Fatal("expected full update to be called")
		}
		if store.lastFullUpdate.tier != "pro" {
			t.Errorf("tier = %q, want pro", store.lastFullUpdate.tier)
		}
		if store.lastFullUpdate.status != "active" {
			t.Errorf("status = %q, want active", store.lastFullUpdate.status)
		}
		if len(audit.events) == 0 {
			t.Error("expected audit event to be recorded")
		}
	})

	t.Run("missing_org_id_returns_ok", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_resume_no_org",
			ProductID:  "pro-id",
			CustomerID: "cust_resume_no_org",
			Metadata:   map[string]string{},
		})

		rr := fireWebhook(t, handler, "customer.subscription.resumed", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 when orgID empty, got %d", rr.Code)
		}
	})

	t.Run("subscription_not_found_returns_ok", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_resume_missing",
			ProductID:  "pro-id",
			CustomerID: "cust_resume_missing",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-200000000002"},
		})

		rr := fireWebhook(t, handler, "customer.subscription.resumed", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 when subscription not found, got %d", rr.Code)
		}
	})

	t.Run("unknown_price_returns_ok", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-200000000003": {
					OrgID:    "00000000-0000-0000-0000-200000000003",
					PlanTier: "pro",
					Status:   "paused",
				},
			},
		}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_resume_unknown",
			ProductID:  "unknown-price-id",
			CustomerID: "cust_resume_unknown",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-200000000003"},
		})

		rr := fireWebhook(t, handler, "customer.subscription.resumed", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 when price unknown, got %d", rr.Code)
		}
		if store.lastFullUpdate != nil {
			t.Error("expected no full update when price is unknown")
		}
	})

	t.Run("idempotent_duplicate_event", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-200000000004": {
					OrgID:    "00000000-0000-0000-0000-200000000004",
					PlanTier: "pro",
					Status:   "paused",
				},
			},
		}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_resume_idem",
			ProductID:  "pro-id",
			CustomerID: "cust_resume_idem",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-200000000004"},
		})

		rr1 := fireWebhookWithID(t, handler, "evt_resume_dup_1", "customer.subscription.resumed", data)
		if rr1.Code != http.StatusOK {
			t.Errorf("first call: expected 200, got %d", rr1.Code)
		}

		rr2 := fireWebhookWithID(t, handler, "evt_resume_dup_1", "customer.subscription.resumed", data)
		if rr2.Code != http.StatusOK {
			t.Errorf("duplicate call: expected 200, got %d", rr2.Code)
		}
	})
}

// Tests for handleTrialWillEnd.

func TestWebhookHandler_TrialWillEnd(t *testing.T) {
	t.Parallel()

	t.Run("happy_path", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-300000000001": {
					OrgID:    "00000000-0000-0000-0000-300000000001",
					PlanTier: "pro",
					Status:   "trialing",
				},
			},
		}
		audit := &mockAuditStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, audit)

		// Build raw JSON with trial_end field.
		trialEnd := time.Now().Add(3 * 24 * time.Hour)
		subData := testSubscriptionData{
			ID:         "sub_trial_1",
			ProductID:  "pro-id",
			CustomerID: "cust_trial_1",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-300000000001"},
		}
		subStripe := subData.ToStripe()
		subStripe.TrialEnd = trialEnd.Unix()
		data, err := json.Marshal(subStripe)
		if err != nil {
			t.Fatal(err)
		}

		rr := fireWebhook(t, handler, "customer.subscription.trial_will_end", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if len(audit.events) == 0 {
			t.Error("expected audit event to be recorded")
		}
		found := false
		for _, ev := range audit.events {
			if ev.Action == "subscription.trial_will_end" {
				found = true
			}
		}
		if !found {
			t.Error("expected trial_will_end audit event")
		}
	})

	t.Run("missing_org_id_returns_ok", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_trial_no_org",
			ProductID:  "pro-id",
			CustomerID: "cust_trial_no_org",
			Metadata:   map[string]string{},
		})

		rr := fireWebhook(t, handler, "customer.subscription.trial_will_end", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 when orgID empty, got %d", rr.Code)
		}
	})

	t.Run("no_trial_end_timestamp", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-300000000002": {
					OrgID:    "00000000-0000-0000-0000-300000000002",
					PlanTier: "pro",
					Status:   "trialing",
				},
			},
		}
		audit := &mockAuditStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, audit)

		// TrialEnd defaults to 0, so timeFromUnix returns nil.
		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_trial_no_end",
			ProductID:  "pro-id",
			CustomerID: "cust_trial_no_end",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-300000000002"},
		})

		rr := fireWebhook(t, handler, "customer.subscription.trial_will_end", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if len(audit.events) == 0 {
			t.Error("expected audit event even without trial_end timestamp")
		}
	})

	t.Run("idempotent_duplicate_event", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-300000000003": {
					OrgID:    "00000000-0000-0000-0000-300000000003",
					PlanTier: "pro",
					Status:   "trialing",
				},
			},
		}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_trial_idem",
			ProductID:  "pro-id",
			CustomerID: "cust_trial_idem",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-300000000003"},
		})

		rr1 := fireWebhookWithID(t, handler, "evt_trial_dup_1", "customer.subscription.trial_will_end", data)
		if rr1.Code != http.StatusOK {
			t.Errorf("first call: expected 200, got %d", rr1.Code)
		}

		rr2 := fireWebhookWithID(t, handler, "evt_trial_dup_1", "customer.subscription.trial_will_end", data)
		if rr2.Code != http.StatusOK {
			t.Errorf("duplicate call: expected 200, got %d", rr2.Code)
		}
	})
}

// Tests for handleChargeDisputeCreated.

func TestWebhookHandler_ChargeDisputeCreated(t *testing.T) {
	t.Parallel()

	t.Run("happy_path", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-400000000001": {
					OrgID:    "00000000-0000-0000-0000-400000000001",
					PlanTier: "pro",
					Status:   "active",
				},
			},
		}
		audit := &mockAuditStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, audit)

		data := mustJSON(t, testDisputeData{
			ID:         "dp_test_1",
			Amount:     5000,
			Reason:     "fraudulent",
			ChargeID:   "ch_test_1",
			CustomerID: "cust_dispute_1",
			OrgID:      "00000000-0000-0000-0000-400000000001",
		})

		rr := fireWebhook(t, handler, "charge.dispute.created", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if len(audit.events) == 0 {
			t.Fatal("expected audit event to be recorded")
		}
		if audit.events[0].Action != "charge.dispute.created" {
			t.Errorf("audit action = %q, want charge.dispute.created", audit.events[0].Action)
		}
	})

	t.Run("missing_org_id_returns_ok", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		audit := &mockAuditStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, audit)

		// Dispute with no customer metadata.
		data := mustJSON(t, testDisputeData{
			ID:         "dp_no_org",
			Amount:     1000,
			Reason:     "general",
			ChargeID:   "ch_no_org",
			CustomerID: "cust_no_org",
			OrgID:      "", // empty org_id
		})

		rr := fireWebhook(t, handler, "charge.dispute.created", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 when org_id is missing, got %d", rr.Code)
		}
		if len(audit.events) != 0 {
			t.Error("expected no audit events when org_id is empty")
		}
	})

	t.Run("invalid_org_id_returns_ok", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		audit := &mockAuditStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, audit)

		// Dispute with invalid UUID in metadata.
		disputeJSON := []byte(`{
			"id": "dp_invalid_org",
			"amount": 1000,
			"reason": "general",
			"charge": {
				"id": "ch_invalid",
				"customer": {
					"id": "cust_invalid",
					"metadata": {"org_id": "not-a-uuid"}
				}
			}
		}`)

		rr := fireWebhook(t, handler, "charge.dispute.created", disputeJSON)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 when org_id is invalid, got %d", rr.Code)
		}
		if len(audit.events) != 0 {
			t.Error("expected no audit events when org_id is invalid UUID")
		}
	})

	t.Run("no_charge_object_returns_ok", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		// Dispute with no charge object at all.
		disputeJSON := []byte(`{"id": "dp_no_charge", "amount": 500}`)

		rr := fireWebhook(t, handler, "charge.dispute.created", disputeJSON)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("idempotent_duplicate_event", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-400000000002": {
					OrgID:    "00000000-0000-0000-0000-400000000002",
					PlanTier: "pro",
					Status:   "active",
				},
			},
		}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testDisputeData{
			ID:         "dp_dup_1",
			Amount:     2000,
			ChargeID:   "ch_dup_1",
			CustomerID: "cust_dup_1",
			OrgID:      "00000000-0000-0000-0000-400000000002",
		})

		rr1 := fireWebhookWithID(t, handler, "evt_dispute_dup_1", "charge.dispute.created", data)
		if rr1.Code != http.StatusOK {
			t.Errorf("first call: expected 200, got %d", rr1.Code)
		}

		rr2 := fireWebhookWithID(t, handler, "evt_dispute_dup_1", "charge.dispute.created", data)
		if rr2.Code != http.StatusOK {
			t.Errorf("duplicate call: expected 200, got %d", rr2.Code)
		}
	})
}

// Tests for handleInvoiceUpcoming.

func TestWebhookHandler_InvoiceUpcoming(t *testing.T) {
	t.Parallel()

	t.Run("happy_path", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-500000000001": {
					OrgID:    "00000000-0000-0000-0000-500000000001",
					PlanTier: "pro",
					Status:   "active",
				},
			},
		}
		audit := &mockAuditStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, audit)

		data := mustJSON(t, testInvoiceDataFull{
			ID:         "inv_upcoming_1",
			CustomerID: "cust_upcoming_1",
			SubID:      "sub_upcoming_1",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-500000000001"},
			AmountDue:  2999,
			DueDate:    time.Now().Add(72 * time.Hour).Unix(),
		})

		rr := fireWebhook(t, handler, "invoice.upcoming", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if len(audit.events) == 0 {
			t.Fatal("expected audit event to be recorded")
		}
		if audit.events[0].Action != "invoice.upcoming" {
			t.Errorf("audit action = %q, want invoice.upcoming", audit.events[0].Action)
		}
	})

	t.Run("missing_org_id_returns_ok", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		audit := &mockAuditStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, audit)

		data := mustJSON(t, testInvoiceDataFull{
			ID:         "inv_upcoming_no_org",
			CustomerID: "cust_upcoming_no_org",
		})

		rr := fireWebhook(t, handler, "invoice.upcoming", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 when org_id missing, got %d", rr.Code)
		}
		if len(audit.events) != 0 {
			t.Error("expected no audit events when org_id is empty")
		}
	})

	t.Run("next_payment_attempt_used_as_fallback_date", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		audit := &mockAuditStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, audit)

		data := mustJSON(t, testInvoiceDataFull{
			ID:                 "inv_upcoming_npa",
			CustomerID:         "cust_upcoming_npa",
			SubID:              "sub_upcoming_npa",
			Metadata:           map[string]string{"org_id": "00000000-0000-0000-0000-500000000002"},
			AmountDue:          4999,
			NextPaymentAttempt: time.Now().Add(48 * time.Hour).Unix(),
		})

		rr := fireWebhook(t, handler, "invoice.upcoming", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if len(audit.events) == 0 {
			t.Error("expected audit event to be recorded")
		}
	})

	t.Run("idempotent_duplicate_event", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testInvoiceDataFull{
			ID:         "inv_upcoming_idem",
			CustomerID: "cust_upcoming_idem",
			SubID:      "sub_upcoming_idem",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-500000000003"},
			AmountDue:  1999,
		})

		rr1 := fireWebhookWithID(t, handler, "evt_inv_up_dup_1", "invoice.upcoming", data)
		if rr1.Code != http.StatusOK {
			t.Errorf("first call: expected 200, got %d", rr1.Code)
		}

		rr2 := fireWebhookWithID(t, handler, "evt_inv_up_dup_1", "invoice.upcoming", data)
		if rr2.Code != http.StatusOK {
			t.Errorf("duplicate call: expected 200, got %d", rr2.Code)
		}
	})
}

// Tests for handleInvoiceUncollectible.

func TestWebhookHandler_InvoiceUncollectible(t *testing.T) {
	t.Parallel()

	t.Run("happy_path_sets_restricted", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-600000000001": {
					OrgID:         "00000000-0000-0000-0000-600000000001",
					PlanTier:      "pro",
					Status:        "active",
					PaymentStatus: "grace",
				},
			},
		}
		audit := &mockAuditStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, audit)

		data := mustJSON(t, testInvoiceDataFull{
			ID:         "inv_uncollectible_1",
			CustomerID: "cust_uncollectible_1",
			SubID:      "sub_uncollectible_1",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-600000000001"},
		})

		rr := fireWebhook(t, handler, "invoice.marked_uncollectible", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if store.lastPaymentStatusUpdate == nil {
			t.Fatal("expected payment status update")
		}
		if store.lastPaymentStatusUpdate.status != "restricted" {
			t.Errorf("payment_status = %q, want restricted", store.lastPaymentStatusUpdate.status)
		}
		if store.lastPaymentStatusUpdate.graceEnd != nil {
			t.Error("expected nil grace end for restricted status")
		}
		if len(audit.events) == 0 {
			t.Error("expected audit event to be recorded")
		}
	})

	t.Run("free_plan_skips_restriction", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-600000000002": {
					OrgID:    "00000000-0000-0000-0000-600000000002",
					PlanTier: "free",
					Status:   "active",
				},
			},
		}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testInvoiceDataFull{
			ID:         "inv_uncollectible_free",
			CustomerID: "cust_uncollectible_free",
			SubID:      "sub_uncollectible_free",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-600000000002"},
		})

		rr := fireWebhook(t, handler, "invoice.marked_uncollectible", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if store.lastPaymentStatusUpdate != nil {
			t.Error("expected no payment status update for free plan")
		}
	})

	t.Run("missing_org_id_returns_ok", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testInvoiceDataFull{
			ID:         "inv_uncollectible_no_org",
			CustomerID: "cust_uncollectible_no_org",
		})

		rr := fireWebhook(t, handler, "invoice.marked_uncollectible", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("subscription_not_found_returns_ok", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testInvoiceDataFull{
			ID:         "inv_uncollectible_nosub",
			CustomerID: "cust_uncollectible_nosub",
			SubID:      "sub_uncollectible_nosub",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-600000000003"},
		})

		rr := fireWebhook(t, handler, "invoice.marked_uncollectible", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 when subscription not found, got %d", rr.Code)
		}
	})

	t.Run("idempotent_duplicate_event", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-600000000004": {
					OrgID:    "00000000-0000-0000-0000-600000000004",
					PlanTier: "pro",
					Status:   "active",
				},
			},
		}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testInvoiceDataFull{
			ID:         "inv_uncollectible_idem",
			CustomerID: "cust_uncollectible_idem",
			SubID:      "sub_uncollectible_idem",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-600000000004"},
		})

		rr1 := fireWebhookWithID(t, handler, "evt_uncol_dup_1", "invoice.marked_uncollectible", data)
		if rr1.Code != http.StatusOK {
			t.Errorf("first call: expected 200, got %d", rr1.Code)
		}

		rr2 := fireWebhookWithID(t, handler, "evt_uncol_dup_1", "invoice.marked_uncollectible", data)
		if rr2.Code != http.StatusOK {
			t.Errorf("duplicate call: expected 200, got %d", rr2.Code)
		}
	})
}

// Tests for handleInvoiceFinalizationFailed.

func TestWebhookHandler_InvoiceFinalizationFailed(t *testing.T) {
	t.Parallel()

	t.Run("happy_path", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		audit := &mockAuditStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, audit)

		data := mustJSON(t, testInvoiceDataFull{
			ID:         "inv_finalize_fail_1",
			CustomerID: "cust_finalize_fail_1",
			SubID:      "sub_finalize_fail_1",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-700000000001"},
		})

		rr := fireWebhook(t, handler, "invoice.finalization_failed", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if len(audit.events) == 0 {
			t.Fatal("expected audit event to be recorded")
		}
		if audit.events[0].Action != "invoice.finalization_failed" {
			t.Errorf("audit action = %q, want invoice.finalization_failed", audit.events[0].Action)
		}
	})

	t.Run("missing_org_id_still_logs", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		audit := &mockAuditStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, audit)

		data := mustJSON(t, testInvoiceDataFull{
			ID:         "inv_finalize_no_org",
			CustomerID: "cust_finalize_no_org",
		})

		rr := fireWebhook(t, handler, "invoice.finalization_failed", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		// The handler still calls logAuditEvent even with empty orgID.
		if len(audit.events) == 0 {
			t.Error("expected audit event even without org_id")
		}
	})

	t.Run("idempotent_duplicate_event", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testInvoiceDataFull{
			ID:         "inv_finalize_idem",
			CustomerID: "cust_finalize_idem",
			SubID:      "sub_finalize_idem",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-700000000002"},
		})

		rr1 := fireWebhookWithID(t, handler, "evt_fin_dup_1", "invoice.finalization_failed", data)
		if rr1.Code != http.StatusOK {
			t.Errorf("first call: expected 200, got %d", rr1.Code)
		}

		rr2 := fireWebhookWithID(t, handler, "evt_fin_dup_1", "invoice.finalization_failed", data)
		if rr2.Code != http.StatusOK {
			t.Errorf("duplicate call: expected 200, got %d", rr2.Code)
		}
	})
}

// Tests for handleAddonSubscriptionCreated.

func TestWebhookHandler_AddonSubscriptionCreated(t *testing.T) {
	t.Parallel()

	t.Run("happy_path", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-800000000001": {
					OrgID:    "00000000-0000-0000-0000-800000000001",
					PlanTier: "pro",
					Status:   "active",
				},
			},
		}
		mapping := NewStripeMappingFromOptions(
			WithStarterPrices("starter-id", ""),
			WithProPrices("pro-id", ""),
			WithAddonPrice("addon-concurrent-runs-id", AddonConcurrentRuns),
		)
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_addon_create_1",
			ProductID:  "addon-concurrent-runs-id",
			CustomerID: "cust_addon_1",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-800000000001"},
		})

		rr := fireWebhook(t, handler, "customer.subscription.created", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("missing_org_id_returns_ok", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMappingFromOptions(
			WithStarterPrices("starter-id", ""),
			WithProPrices("pro-id", ""),
			WithAddonPrice("addon-concurrent-runs-id", AddonConcurrentRuns),
		)
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_addon_no_org",
			ProductID:  "addon-concurrent-runs-id",
			CustomerID: "cust_addon_no_org",
			Metadata:   map[string]string{},
		})

		rr := fireWebhook(t, handler, "customer.subscription.created", data)

		// Missing org_id for addon returns 200 (noop) per the handler logic.
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("idempotent_duplicate_event", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-800000000002": {
					OrgID:    "00000000-0000-0000-0000-800000000002",
					PlanTier: "pro",
					Status:   "active",
				},
			},
		}
		mapping := NewStripeMappingFromOptions(
			WithStarterPrices("starter-id", ""),
			WithProPrices("pro-id", ""),
			WithAddonPrice("addon-members-id", AddonMembers),
		)
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_addon_idem",
			ProductID:  "addon-members-id",
			CustomerID: "cust_addon_idem",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-800000000002"},
		})

		rr1 := fireWebhookWithID(t, handler, "evt_addon_dup_1", "customer.subscription.created", data)
		if rr1.Code != http.StatusOK {
			t.Errorf("first call: expected 200, got %d", rr1.Code)
		}

		rr2 := fireWebhookWithID(t, handler, "evt_addon_dup_1", "customer.subscription.created", data)
		if rr2.Code != http.StatusOK {
			t.Errorf("duplicate call: expected 200, got %d", rr2.Code)
		}
	})
}

// Tests for handleAddonSubscriptionCanceled.

func TestWebhookHandler_AddonSubscriptionCanceled(t *testing.T) {
	t.Parallel()

	t.Run("happy_path", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-900000000001": {
					OrgID:    "00000000-0000-0000-0000-900000000001",
					PlanTier: "pro",
					Status:   "active",
				},
			},
		}
		mapping := NewStripeMappingFromOptions(
			WithStarterPrices("starter-id", ""),
			WithProPrices("pro-id", ""),
			WithAddonPrice("addon-concurrent-runs-id", AddonConcurrentRuns),
		)
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_addon_cancel_1",
			ProductID:  "addon-concurrent-runs-id",
			CustomerID: "cust_addon_cancel_1",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-900000000001"},
		})

		rr := fireWebhook(t, handler, "customer.subscription.deleted", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("missing_org_id_returns_ok", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMappingFromOptions(
			WithStarterPrices("starter-id", ""),
			WithProPrices("pro-id", ""),
			WithAddonPrice("addon-concurrent-runs-id", AddonConcurrentRuns),
		)
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_addon_cancel_no_org",
			ProductID:  "addon-concurrent-runs-id",
			CustomerID: "cust_addon_cancel_no_org",
			Metadata:   map[string]string{},
		})

		rr := fireWebhook(t, handler, "customer.subscription.deleted", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("idempotent_duplicate_event", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-900000000002": {
					OrgID:    "00000000-0000-0000-0000-900000000002",
					PlanTier: "pro",
					Status:   "active",
				},
			},
		}
		mapping := NewStripeMappingFromOptions(
			WithStarterPrices("starter-id", ""),
			WithProPrices("pro-id", ""),
			WithAddonPrice("addon-members-id", AddonMembers),
		)
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_addon_cancel_idem",
			ProductID:  "addon-members-id",
			CustomerID: "cust_addon_cancel_idem",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-900000000002"},
		})

		rr1 := fireWebhookWithID(t, handler, "evt_addon_cancel_dup_1", "customer.subscription.deleted", data)
		if rr1.Code != http.StatusOK {
			t.Errorf("first call: expected 200, got %d", rr1.Code)
		}

		rr2 := fireWebhookWithID(t, handler, "evt_addon_cancel_dup_1", "customer.subscription.deleted", data)
		if rr2.Code != http.StatusOK {
			t.Errorf("duplicate call: expected 200, got %d", rr2.Code)
		}
	})
}

// Tests for maybeSendHTTPJobsDowngradeWarning.

func TestWebhookHandler_MaybeSendHTTPJobsDowngradeWarning(t *testing.T) {
	t.Parallel()

	t.Run("downgrade_from_pro_to_free_with_http_jobs", func(t *testing.T) {
		t.Parallel()

		// This test verifies the full downgrade flow triggers the warning path.
		// The actual email is async and not directly observable, but we verify the
		// handler reaches the right code path by checking the downgrade is deferred.
		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-a00000000001": {
					OrgID:    "00000000-0000-0000-0000-a00000000001",
					PlanTier: "pro",
					Status:   "active",
				},
			},
		}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		// Create a billing email sender (nil client means sends are noops in tests).
		handler := newTestHandler(store, mapping, nil)

		// Downgrade from pro to starter (which triggers maybeSendHTTPJobsDowngradeWarning
		// in handleSubscriptionUpdated).
		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_downgrade_http",
			ProductID:  "starter-id",
			CustomerID: "cust_downgrade_http",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-a00000000001"},
		})

		rr := fireWebhook(t, handler, "customer.subscription.updated", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		// Verify the downgrade was deferred (pending tier set).
		if store.lastPendingTier != "starter" {
			t.Errorf("expected pending tier starter, got %q", store.lastPendingTier)
		}
	})

	t.Run("downgrade_to_plan_with_http_mode_no_warning", func(t *testing.T) {
		t.Parallel()

		// Pro to pro (same tier) should not trigger warning. We verify no pending tier is set.
		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-a00000000002": {
					OrgID:    "00000000-0000-0000-0000-a00000000002",
					PlanTier: "pro",
					Status:   "active",
				},
			},
		}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		// Same tier update (pro to pro) -- no downgrade, no warning.
		data := mustJSON(t, testSubscriptionData{
			ID:         "sub_same_tier",
			ProductID:  "pro-id",
			CustomerID: "cust_same_tier",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-a00000000002"},
		})

		rr := fireWebhook(t, handler, "customer.subscription.updated", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if store.lastPendingTier != "" {
			t.Errorf("expected no pending tier for same-tier update, got %q", store.lastPendingTier)
		}
	})

	t.Run("downgrade_with_period_end_date", func(t *testing.T) {
		t.Parallel()

		periodEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-a00000000003": {
					OrgID:    "00000000-0000-0000-0000-a00000000003",
					PlanTier: "pro",
					Status:   "active",
				},
			},
		}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := mustJSON(t, testSubscriptionData{
			ID:               "sub_downgrade_period",
			ProductID:        "starter-id",
			CustomerID:       "cust_downgrade_period",
			CurrentPeriodEnd: &periodEnd,
			Metadata:         map[string]string{"org_id": "00000000-0000-0000-0000-a00000000003"},
		})

		rr := fireWebhook(t, handler, "customer.subscription.updated", data)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if store.lastPendingDowngrade == nil {
			t.Fatal("expected pending downgrade to be set")
		}
		if store.lastPendingDowngrade.periodEnd == nil || !store.lastPendingDowngrade.periodEnd.Equal(periodEnd) {
			t.Error("expected period end to be passed to pending downgrade")
		}
	})
}

// Tests for malformed payloads.

// fireWebhookRawBody sends a raw body directly to the handler, bypassing JSON marshaling.
func fireWebhookRawBody(t *testing.T, handler http.Handler, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// malformedEventBody builds a valid Stripe event envelope with invalid data.object content.
// This tests the inner handler's JSON parsing without breaking the outer event parsing.
func malformedEventBody(eventType string) []byte {
	// The "not json" value is a valid JSON string, but the handlers try to unmarshal
	// data.object.raw into a struct, which will fail because it's a string, not an object.
	return fmt.Appendf(nil, `{"type":"%s","data":{"object":"not_a_valid_object"}}`, eventType)
}

func TestWebhookHandler_MalformedPayloads(t *testing.T) {
	t.Parallel()

	mapping := NewStripeMapping("starter-id", "", "pro-id", "")

	malformedCases := []struct {
		name      string
		eventType string
	}{
		{"paused_invalid_data", "customer.subscription.paused"},
		{"resumed_invalid_data", "customer.subscription.resumed"},
		{"trial_will_end_invalid_data", "customer.subscription.trial_will_end"},
		{"dispute_invalid_data", "charge.dispute.created"},
		{"invoice_upcoming_invalid_data", "invoice.upcoming"},
		{"invoice_uncollectible_invalid_data", "invoice.marked_uncollectible"},
		{"invoice_finalization_failed_invalid_data", "invoice.finalization_failed"},
	}

	for _, tc := range malformedCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &mockBillingStore{}
			handler := newTestHandler(store, mapping, nil)

			body := malformedEventBody(tc.eventType)
			rr := fireWebhookRawBody(t, handler, body)

			if rr.Code == http.StatusOK {
				t.Errorf("expected non-200 for malformed data in %s, got %d", tc.eventType, rr.Code)
			}
		})
	}
}

// Tests for resumed with validation errors.

func TestWebhookHandler_SubscriptionResumed_ValidationErrors(t *testing.T) {
	t.Parallel()

	t.Run("empty_subscription_id", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		// Build a subscription with empty ID.
		data := []byte(`{
			"id": "",
			"status": "active",
			"customer": {"id": "cust_test"},
			"items": {"data": [{"price": {"id": "pro-id"}}]},
			"metadata": {"org_id": "00000000-0000-0000-0000-200000000099"}
		}`)

		rr := fireWebhook(t, handler, "customer.subscription.resumed", data)

		if rr.Code == http.StatusOK {
			t.Error("expected non-200 for empty subscription ID")
		}
	})

	t.Run("empty_customer_id", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := []byte(`{
			"id": "sub_no_cust",
			"status": "active",
			"customer": {"id": ""},
			"items": {"data": [{"price": {"id": "pro-id"}}]},
			"metadata": {"org_id": "00000000-0000-0000-0000-200000000098"}
		}`)

		rr := fireWebhook(t, handler, "customer.subscription.resumed", data)

		if rr.Code == http.StatusOK {
			t.Error("expected non-200 for empty customer ID")
		}
	})

	t.Run("empty_price_id", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := newTestHandler(store, mapping, nil)

		data := []byte(`{
			"id": "sub_no_price",
			"status": "active",
			"customer": {"id": "cust_test"},
			"items": {"data": [{"price": {"id": ""}}]},
			"metadata": {"org_id": "00000000-0000-0000-0000-200000000097"}
		}`)

		rr := fireWebhook(t, handler, "customer.subscription.resumed", data)

		if rr.Code == http.StatusOK {
			t.Error("expected non-200 for empty price ID")
		}
	})
}

// Suppress unused import warning for domain.
var _ = domain.PlanFree
