package billing

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"
)

// webhookEventCase describes a single dispatched Stripe event the handler
// must cover. Each case provides a fresh store + payload builder so cases
// can run in parallel without sharing state.
type webhookEventCase struct {
	name           string
	eventType      string
	newStoreOrgID  string
	buildStore     func(orgID string) *mockBillingStore
	buildPayload   func(orgID string) json.RawMessage
	sideEffectName string
	sideEffectHits func(s *mockBillingStore, a *mockAuditStore) int
}

func subscriptionPayload(orgID, subID, productID, customerID string) func(*testing.T) json.RawMessage {
	return func(t *testing.T) json.RawMessage {
		t.Helper()

		return mustJSON(t, testSubscriptionData{
			ID:         subID,
			ProductID:  productID,
			CustomerID: customerID,
			Metadata:   map[string]string{"org_id": orgID},
		})
	}
}

func invoicePayload(orgID, invID, customerID, subID string) func(*testing.T) json.RawMessage {
	return func(t *testing.T) json.RawMessage {
		t.Helper()

		return mustJSON(t, testInvoiceData{
			ID:         invID,
			CustomerID: customerID,
			SubID:      subID,
			Metadata:   map[string]string{"org_id": orgID},
		})
	}
}

func upcomingInvoicePayload(orgID, invID, customerID, subID string, amount int64) func(*testing.T) json.RawMessage {
	return func(t *testing.T) json.RawMessage {
		t.Helper()

		return mustJSON(t, testInvoiceDataFull{
			ID:                 invID,
			CustomerID:         customerID,
			SubID:              subID,
			Metadata:           map[string]string{"org_id": orgID},
			AmountDue:          amount,
			NextPaymentAttempt: 1_900_000_000,
		})
	}
}

func disputePayload(orgID, disputeID, chargeID, customerID string, amount int64) func(*testing.T) json.RawMessage {
	return func(t *testing.T) json.RawMessage {
		t.Helper()

		return mustJSON(t, testDisputeData{
			ID:         disputeID,
			Amount:     amount,
			Reason:     "fraudulent",
			ChargeID:   chargeID,
			CustomerID: customerID,
			OrgID:      orgID,
		})
	}
}

// activeSubStore returns a mock store with a single org on the pro tier with
// an active subscription bound to the given Stripe IDs.
func activeSubStore(orgID string) *mockBillingStore {
	subID := "sub_" + orgID
	custID := "cust_" + orgID
	return &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			orgID: {
				OrgID:                orgID,
				PlanTier:             "pro",
				Status:               "active",
				StripeSubscriptionID: &subID,
				StripeCustomerID:     &custID,
			},
		},
	}
}

// allDispatchedEventCases returns one case per branch of dispatchStripeEvent.
// The org IDs are kept distinct so the parallel test variants don't collide
// in mock store maps.
func allDispatchedEventCases(t *testing.T) []webhookEventCase {
	t.Helper()
	const productID = "pro-id"

	mkSub := func(orgID, suffix string) func(string) json.RawMessage {
		subID := "sub_" + orgID + "_" + suffix
		custID := "cust_" + orgID + "_" + suffix
		f := subscriptionPayload(orgID, subID, productID, custID)
		return func(string) json.RawMessage { return f(t) }
	}
	mkInv := func(orgID, suffix string) func(string) json.RawMessage {
		invID := "in_" + orgID + "_" + suffix
		custID := "cust_" + orgID
		subID := "sub_" + orgID
		f := invoicePayload(orgID, invID, custID, subID)
		return func(string) json.RawMessage { return f(t) }
	}
	mkInvUpcoming := func(orgID, suffix string) func(string) json.RawMessage {
		invID := "in_" + orgID + "_" + suffix
		custID := "cust_" + orgID
		subID := "sub_" + orgID
		f := upcomingInvoicePayload(orgID, invID, custID, subID, 9900)
		return func(string) json.RawMessage { return f(t) }
	}
	mkDispute := func(orgID, suffix string) func(string) json.RawMessage {
		disputeID := "dp_" + orgID + "_" + suffix
		chargeID := "ch_" + orgID + "_" + suffix
		custID := "cust_" + orgID
		f := disputePayload(orgID, disputeID, chargeID, custID, 9900)
		return func(string) json.RawMessage { return f(t) }
	}

	auditHits := func(_ *mockBillingStore, a *mockAuditStore) int {
		if a == nil {
			return 0
		}
		return len(a.events)
	}
	upsertHits := func(s *mockBillingStore, _ *mockAuditStore) int { return s.upsertCount }
	statusUpdateHits := func(s *mockBillingStore, _ *mockAuditStore) int {
		if s.lastStatusUpdate == nil {
			return 0
		}
		return 1
	}
	fullUpdateHits := func(s *mockBillingStore, _ *mockAuditStore) int {
		if s.lastFullUpdate == nil {
			return 0
		}
		return 1
	}
	paymentStatusHits := func(s *mockBillingStore, _ *mockAuditStore) int {
		if s.lastPaymentStatusUpdate == nil {
			return 0
		}
		return 1
	}

	return []webhookEventCase{
		{
			name:           "subscription_created",
			eventType:      string(stripe.EventTypeCustomerSubscriptionCreated),
			newStoreOrgID:  "00000000-0000-0000-0000-aaaa00000001",
			buildStore:     func(string) *mockBillingStore { return &mockBillingStore{} },
			buildPayload:   mkSub("00000000-0000-0000-0000-aaaa00000001", "create"),
			sideEffectName: "upsert",
			sideEffectHits: upsertHits,
		},
		{
			name:           "subscription_updated",
			eventType:      string(stripe.EventTypeCustomerSubscriptionUpdated),
			newStoreOrgID:  "00000000-0000-0000-0000-aaaa00000002",
			buildStore:     activeSubStore,
			buildPayload:   mkSub("00000000-0000-0000-0000-aaaa00000002", "update"),
			sideEffectName: "full_update",
			sideEffectHits: fullUpdateHits,
		},
		{
			name:           "subscription_deleted",
			eventType:      string(stripe.EventTypeCustomerSubscriptionDeleted),
			newStoreOrgID:  "00000000-0000-0000-0000-aaaa00000003",
			buildStore:     activeSubStore,
			buildPayload:   mkSub("00000000-0000-0000-0000-aaaa00000003", "delete"),
			sideEffectName: "audit",
			sideEffectHits: auditHits,
		},
		{
			name:           "subscription_paused",
			eventType:      string(stripe.EventTypeCustomerSubscriptionPaused),
			newStoreOrgID:  "00000000-0000-0000-0000-aaaa00000004",
			buildStore:     activeSubStore,
			buildPayload:   mkSub("00000000-0000-0000-0000-aaaa00000004", "pause"),
			sideEffectName: "status_update",
			sideEffectHits: statusUpdateHits,
		},
		{
			name:          "subscription_resumed",
			eventType:     string(stripe.EventTypeCustomerSubscriptionResumed),
			newStoreOrgID: "00000000-0000-0000-0000-aaaa00000005",
			buildStore: func(orgID string) *mockBillingStore {
				s := activeSubStore(orgID)
				s.subscriptions[orgID].Status = "paused"
				return s
			},
			buildPayload:   mkSub("00000000-0000-0000-0000-aaaa00000005", "resume"),
			sideEffectName: "full_update",
			sideEffectHits: fullUpdateHits,
		},
		{
			name:           "subscription_trial_will_end",
			eventType:      string(stripe.EventTypeCustomerSubscriptionTrialWillEnd),
			newStoreOrgID:  "00000000-0000-0000-0000-aaaa00000006",
			buildStore:     activeSubStore,
			buildPayload:   mkSub("00000000-0000-0000-0000-aaaa00000006", "trial"),
			sideEffectName: "audit",
			sideEffectHits: auditHits,
		},
		{
			name:          "invoice_paid",
			eventType:     string(stripe.EventTypeInvoicePaid),
			newStoreOrgID: "00000000-0000-0000-0000-aaaa00000007",
			buildStore: func(orgID string) *mockBillingStore {
				s := activeSubStore(orgID)
				s.subscriptions[orgID].PaymentStatus = "grace"
				return s
			},
			buildPayload:   mkInv("00000000-0000-0000-0000-aaaa00000007", "paid"),
			sideEffectName: "payment_status",
			sideEffectHits: paymentStatusHits,
		},
		{
			name:           "invoice_payment_failed",
			eventType:      string(stripe.EventTypeInvoicePaymentFailed),
			newStoreOrgID:  "00000000-0000-0000-0000-aaaa00000008",
			buildStore:     activeSubStore,
			buildPayload:   mkInv("00000000-0000-0000-0000-aaaa00000008", "failed"),
			sideEffectName: "payment_status",
			sideEffectHits: paymentStatusHits,
		},
		{
			name:           "invoice_upcoming",
			eventType:      string(stripe.EventTypeInvoiceUpcoming),
			newStoreOrgID:  "00000000-0000-0000-0000-aaaa00000009",
			buildStore:     activeSubStore,
			buildPayload:   mkInvUpcoming("00000000-0000-0000-0000-aaaa00000009", "upcoming"),
			sideEffectName: "audit",
			sideEffectHits: auditHits,
		},
		{
			name:           "invoice_marked_uncollectible",
			eventType:      string(stripe.EventTypeInvoiceMarkedUncollectible),
			newStoreOrgID:  "00000000-0000-0000-0000-aaaa0000000a",
			buildStore:     activeSubStore,
			buildPayload:   mkInv("00000000-0000-0000-0000-aaaa0000000a", "uncoll"),
			sideEffectName: "payment_status",
			sideEffectHits: paymentStatusHits,
		},
		{
			name:           "invoice_finalization_failed",
			eventType:      string(stripe.EventTypeInvoiceFinalizationFailed),
			newStoreOrgID:  "00000000-0000-0000-0000-aaaa0000000b",
			buildStore:     activeSubStore,
			buildPayload:   mkInv("00000000-0000-0000-0000-aaaa0000000b", "finfail"),
			sideEffectName: "audit",
			sideEffectHits: auditHits,
		},
		{
			name:           "invoice_finalized",
			eventType:      string(stripe.EventTypeInvoiceFinalized),
			newStoreOrgID:  "00000000-0000-0000-0000-aaaa0000000d",
			buildStore:     activeSubStore,
			buildPayload:   mkInv("00000000-0000-0000-0000-aaaa0000000d", "final"),
			sideEffectName: "audit",
			sideEffectHits: auditHits,
		},
		{
			name:           "charge_dispute_created",
			eventType:      string(stripe.EventTypeChargeDisputeCreated),
			newStoreOrgID:  "00000000-0000-0000-0000-aaaa0000000c",
			buildStore:     activeSubStore,
			buildPayload:   mkDispute("00000000-0000-0000-0000-aaaa0000000c", "dispute"),
			sideEffectName: "audit",
			sideEffectHits: auditHits,
		},
	}
}

// TestWebhookDispatchCoverage_AllEvents verifies that every branch of
// dispatchStripeEvent is reachable end-to-end through ServeHTTP, returns 200,
// and produces the side effect characteristic of its handler. Adding a new
// dispatched event without updating allDispatchedEventCases will surface as
// a missing case in this table.
func TestWebhookDispatchCoverage_AllEvents(t *testing.T) {
	t.Parallel()

	cases := allDispatchedEventCases(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := tc.buildStore(tc.newStoreOrgID)
			audit := &mockAuditStore{}
			mapping := NewStripeMapping("starter-id", "", "pro-id", "")
			handler := newTestHandler(store, mapping, audit)

			payload := tc.buildPayload(tc.newStoreOrgID)
			eventID := "evt_cov_" + tc.name
			rr := fireWebhookWithID(t, handler, eventID, tc.eventType, payload)
			require.Equal(t, http.StatusOK,

				rr.Code)
			require.GreaterOrEqual(
				t, tc.sideEffectHits(store, audit), 1)
		})
	}
}

// TestWebhookDispatchCoverage_UnknownEvent confirms that an event type the
// handler does not dispatch is short-circuited as 200 OK without producing
// any side effects (Stripe treats this as "ack and skip").
func TestWebhookDispatchCoverage_UnknownEvent(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := newTestHandler(store, mapping, audit)

	payload := mustJSON(t, map[string]any{"id": "obj_unknown"})
	rr := fireWebhookWithID(t, handler, "evt_unknown_1", "customer.tax_id.created", payload)
	require.Equal(t, http.StatusOK,

		rr.Code)
	require.Equal(t, 0, store.
		upsertCount,
	)
	require.Nil(t, store.
		lastPlanUpdate,
	)
	require.Nil(t, store.
		lastPaymentStatusUpdate,
	)
	require.Empty(t, audit.events)
}

// TestWebhookIdempotency_ReplayThreeTimes verifies that for every dispatched
// event type, redelivering the same event ID three times produces at most one
// round of side effects. The replay cache and the DB-backed claim path
// together must hold even under tight retry loops.
func TestWebhookIdempotency_ReplayThreeTimes(t *testing.T) {
	t.Parallel()

	cases := allDispatchedEventCases(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := tc.buildStore(tc.newStoreOrgID)
			audit := &mockAuditStore{}
			mapping := NewStripeMapping("starter-id", "", "pro-id", "")
			handler := newTestHandler(store, mapping, audit)

			payload := tc.buildPayload(tc.newStoreOrgID)
			eventID := "evt_replay_" + tc.name

			for i := 1; i <= 3; i++ {
				rr := fireWebhookWithID(t, handler, eventID, tc.eventType, payload)
				require.Equal(t, http.StatusOK,

					rr.Code)
			}

			hits := tc.sideEffectHits(store, audit)
			require.Equal(t, 1, hits)
			require.LessOrEqual(t,
				store.upsertCount,

				1)
		})
	}
}

// TestWebhookIdempotency_DistinctEventIDsAllProcess confirms that distinct
// event IDs of the same type each dispatch independently — the idempotency
// gate keys on event.id, not on event.type.
func TestWebhookIdempotency_DistinctEventIDsAllProcess(t *testing.T) {
	t.Parallel()

	const orgID = "00000000-0000-0000-0000-bbbb00000001"
	store := &mockBillingStore{}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := newTestHandler(store, mapping, audit)

	for i := range 3 {
		eventID := fmt.Sprintf("evt_distinct_%d", i)
		payload := mustJSON(t, testSubscriptionData{
			ID:         fmt.Sprintf("sub_distinct_%d", i),
			ProductID:  "pro-id",
			CustomerID: fmt.Sprintf("cust_distinct_%d", i),
			Metadata:   map[string]string{"org_id": orgID},
		})
		rr := fireWebhookWithID(t, handler, eventID, "customer.subscription.created", payload)
		require.Equal(t, http.StatusOK,

			rr.Code)
	}
	require.Equal(t, 3, store.
		upsertCount,
	)
}
