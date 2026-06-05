package billing

import (
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// On the entry transition (previousStep == DunningStepNone), the Dunner
// must NOT dispatch billing.delinquent. The Stripe webhook handler's
// handlePaymentFailed already announced the delinquent state when
// invoice.payment_failed fired; the 0→Entry transition is the same event
// arriving via a different path, so re-dispatching would deliver a
// duplicate to subscribers within the first day.
func TestDunner_EntryTransition_DoesNotDispatchDelinquent(t *testing.T) {
	t.Parallel()

	entered := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	clock := entered.Add(1 * time.Minute) // entry, well before day 3

	store := newFakeDunningStore()
	store.seed(&fakeDunningRow{
		OrgID:            "org_entry",
		PlanTier:         string(domain.PlanPro),
		PaymentStatus:    "grace",
		DunningStep:      DunningStepNone, // simulate a row in step 0 (pre-Entry)
		DunningEnteredAt: entered,
	})
	disp := &fakeDispatcher{}
	d := NewDunner(store,
		WithDunnerDispatcher(disp),
		WithDunnerClock(func() time.Time { return clock }),
		WithDunnerCooldown(1*time.Second),
	)
	require.NoError(t,
		d.Tick(context.
			Background()))

	got := store.get("org_entry")
	require.Equal(t,
		DunningStepEntry,
		got.DunningStep,
	)
	assert.EqualValues(t, 0,
		countEvent(dispatchedEventTypes(disp), domain.
			WebhookEventBillingDelinquent,
		))

}

// Escalation transitions (Entry→Day3, Day3→Day14, etc.) must still
// dispatch billing.delinquent — they're genuine state changes
// subscribers need to react to. Regression guard for the dedup fix.
func TestDunner_EscalationTransition_StillDispatchesDelinquent(t *testing.T) {
	t.Parallel()

	entered := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	clock := entered.Add(3*24*time.Hour + 1*time.Minute) // past day 3

	store := newFakeDunningStore()
	store.seed(&fakeDunningRow{
		OrgID:            "org_escalate",
		PlanTier:         string(domain.PlanPro),
		PaymentStatus:    "grace",
		DunningStep:      DunningStepEntry, // already past entry
		DunningEnteredAt: entered,
	})
	disp := &fakeDispatcher{}
	d := NewDunner(store,
		WithDunnerDispatcher(disp),
		WithDunnerClock(func() time.Time { return clock }),
		WithDunnerCooldown(1*time.Second),
	)
	require.NoError(t,
		d.Tick(context.
			Background()))

	got := store.get("org_escalate")
	require.Equal(t,
		DunningStepDay3,
		got.DunningStep,
	)
	assert.EqualValues(t, 1,
		countEvent(dispatchedEventTypes(disp), domain.
			WebhookEventBillingDelinquent,
		))

}

// Day 74 transition still emits both billing.delinquent (escalation) and
// billing.suspended (terminal). Regression guard.
func TestDunner_Day74Transition_DispatchesBoth(t *testing.T) {
	t.Parallel()

	entered := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	clock := entered.Add(75 * 24 * time.Hour)

	store := newFakeDunningStore()
	store.seed(&fakeDunningRow{
		OrgID:            "org_day74",
		PlanTier:         string(domain.PlanPro),
		PaymentStatus:    "grace",
		DunningStep:      DunningStepEntry,
		DunningEnteredAt: entered,
	})
	disp := &fakeDispatcher{}
	d := NewDunner(store,
		WithDunnerDispatcher(disp),
		WithDunnerClock(func() time.Time { return clock }),
		WithDunnerCooldown(1*time.Second),
	)
	require.NoError(t,
		d.Tick(context.
			Background()))

	events := dispatchedEventTypes(disp)
	assert.EqualValues(t, 1,
		countEvent(events, domain.
			WebhookEventBillingDelinquent,
		))
	assert.EqualValues(t, 1,
		countEvent(events, domain.
			WebhookEventBillingSuspended,
		))

}

// End-to-end pipeline guard: invoice.payment_failed runs handlePaymentFailed
// (which dispatches billing.delinquent once) then StartDunning (which seeds
// the row at step=1 in production). A subsequent Dunner.Tick at t=0 must
// not produce a second billing.delinquent — the only delinquent visible to
// subscribers should be the one from handlePaymentFailed.
func TestPipeline_PaymentFailedThenInitialDunnerTick_DelinquentExactlyOnce(t *testing.T) {
	t.Parallel()

	const orgID = "00000000-0000-0000-0000-400000000001"
	stripeSubID := "sub_pipeline_1"
	stripeCustID := "cust_pipeline_1"

	dunStore := newFakeDunningStore()
	// Seed the org row so StartDunning can find it. Step starts at None;
	// StartDunning will set it to Entry just like the pg_dunning impl.
	dunStore.seed(&fakeDunningRow{
		OrgID:    orgID,
		PlanTier: string(domain.PlanPro),
	})

	billingStore := &mockBillingStore{
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

	disp := &fakeDispatcher{}
	enf := NewEnforcer(billingStore, nil, nil, WithBillingDispatcher(disp))
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	h := NewWebhookHandler(billingStore, mapping, "", slog.Default(), enf, nil,
		WithDevBypassSignatureCheck(), WithDunningStore(dunStore))

	// Stripe marks the account delinquent before the dunning worker runs.
	data := mustJSON(t, testInvoiceDataFull{
		ID:         "in_pipeline_1",
		CustomerID: stripeCustID,
		SubID:      stripeSubID,
		Metadata:   map[string]string{"org_id": orgID},
		AmountDue:  1500,
	})
	rr := fireWebhook(t, h, "invoice.payment_failed", data)
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	// The dunning worker runs immediately afterwards at entered_at + 1s.
	clock := time.Now().Add(1 * time.Second)
	d := NewDunner(dunStore,
		WithDunnerDispatcher(disp),
		WithDunnerClock(func() time.Time { return clock }),
		WithDunnerCooldown(1*time.Millisecond),
	)
	require.NoError(t,
		d.Tick(context.
			Background()))
	require.EqualValues(t, 1, countEvent(dispatchedEventTypes(
		disp), domain.
		WebhookEventBillingDelinquent,
	))

}
