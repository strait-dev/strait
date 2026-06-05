package billing

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 10 MB synthetic detail blob: helper must reject before queuing the
// delivery, so a single misbehaving caller cannot wedge the dispatch
// queue with oversized payloads.
func TestDispatchBillingWebhook_OversizedPayloadRejected(t *testing.T) {
	t.Parallel()

	d := &fakeDispatcher{}
	huge := strings.Repeat("x", 10*1024*1024)
	err := DispatchBillingWebhook(context.Background(), d,
		"org_huge", domain.PlanScale, domain.WebhookEventBillingCapWarning,
		map[string]any{"blob": huge},
	)
	require.Error(t,
		err)
	assert.True(t, strings.Contains(err.
		Error(),
		"exceeds",
	))
	assert.Len(t, d.calls,
		0)

}

// Two callers producing the same logical event must produce distinct
// envelope event_ids, so per-event dedup downstream (subscriber-side
// idempotency keys) does not silently drop one of them.
func TestDispatchBillingWebhook_EventIDsAreUnique(t *testing.T) {
	t.Parallel()

	d := &fakeDispatcher{}
	for range 5 {
		err := DispatchBillingWebhook(context.Background(), d,
			"org", domain.PlanPro, domain.WebhookEventBillingCapReached,
			map[string]any{"spend_pct": 1.0},
		)
		require.NoError(t,
			err)

	}
	seen := make(map[string]bool, len(d.calls))
	for _, c := range d.calls {
		var env BillingEventEnvelope
		require.NoError(t,
			unmarshalEnvelope(c.payload,
				&env),
		)
		assert.False(t, seen[env.EventID])

		seen[env.EventID] = true
	}
	assert.Len(t, seen,
		5)

}

// Concurrent dispatches must not race on the helper's internal state.
// Helper has no per-call state, but this exercises the contract and is
// a cheap regression guard against future refactors that introduce
// shared mutable state.
func TestDispatchBillingWebhook_ConcurrencySafe(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	d := &fakeDispatcher{}
	var mu sync.Mutex
	wrapper := dispatcherFunc(func(ctx context.Context, orgID, eventType string, payload []byte) error {
		mu.Lock()
		defer mu.Unlock()
		return d.DispatchBillingEvent(ctx, orgID, eventType, payload)
	})

	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		{
			i := i
			concWG.Go(func() {
				defer wg.Done()
				_ = DispatchBillingWebhook(context.Background(), wrapper,
					"org", domain.PlanScale, domain.WebhookEventBillingCapWarning,
					map[string]any{"i": i},
				)
			})
		}
	}
	wg.Wait()
	assert.Len(t, d.calls,
		n)

}

// Wildcard event_types in webhook_subscriptions ("*") must match every
// known billing event. The match function lives in the webhook package;
// this test guards the contract by asserting the canonical event
// strings stay dotted (the matcher uses dotted-prefix semantics).
func TestBillingEventNames_DottedNamespace(t *testing.T) {
	t.Parallel()

	all := []string{
		domain.WebhookEventBillingCapWarning,
		domain.WebhookEventBillingCapReached,
		domain.WebhookEventBillingCapDisabled,
		domain.WebhookEventBillingOverageDisabled,
		domain.WebhookEventBillingSuspended,
		domain.WebhookEventBillingDelinquent,
		domain.WebhookEventScheduleSuspended,
		domain.WebhookEventWorkflowRegistrationRejected,
		domain.WebhookEventSLACreditIssued,
	}
	for _, ev := range all {
		assert.True(t, strings.Contains(ev,
			"."))

	}
}

type dispatcherFunc func(ctx context.Context, orgID, eventType string, payload []byte) error

func (f dispatcherFunc) DispatchBillingEvent(ctx context.Context, orgID, eventType string, payload []byte) error {
	return f(ctx, orgID, eventType, payload)
}

func unmarshalEnvelope(payload []byte, env *BillingEventEnvelope) error {
	return json.Unmarshal(payload, env)
}
