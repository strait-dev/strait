package billing

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two Tick goroutines running against the same batch must advance each row
// exactly once. The fake store mirrors PgStore's FOR UPDATE SKIP LOCKED via
// a per-row claim guard so this test exercises the policy/dispatch path
// under contention.
func TestDunner_ConcurrentTicksAdvanceEachRowOnce(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	entered := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	clock := entered.Add(4 * 24 * time.Hour)
	store := newFakeDunningStore()
	const n = 50
	for i := range n {
		store.seed(&fakeDunningRow{
			OrgID:            fmtOrgID(i),
			PlanTier:         string(domain.PlanPro),
			DunningStep:      1,
			DunningEnteredAt: entered,
		})
	}
	disp := &fakeDispatcher{}
	d := NewDunner(store,
		WithDunnerDispatcher(disp),
		WithDunnerClock(func() time.Time { return clock }),
		WithDunnerCooldown(1*time.Second),
	)

	var wg sync.WaitGroup
	wg.Add(2)
	for range 2 {
		concWG.Go(func() {
			defer wg.Done()
			_ = d.Tick(context.Background())
		})
	}
	wg.Wait()

	for i := range n {
		row := store.get(fmtOrgID(i))
		assert.Equal(t, DunningStepDay3,

			row.DunningStep,
		)
	}
	assert.Equal(t, n,
		countEvent(dispatchedEventTypes(
			disp), domain.
			WebhookEventBillingDelinquent,
		))
}

// A hand-edited row with dunning_entered_at in the future (clock skew or
// operator tampering) must be a safe no-op rather than regressing the step.
func TestDunner_FutureEnteredAtIsSafeNoOp(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	store := newFakeDunningStore()
	store.seed(&fakeDunningRow{
		OrgID:            "org_future",
		PlanTier:         string(domain.PlanPro),
		DunningStep:      3, // previously at day 7
		DunningEnteredAt: clock.Add(48 * time.Hour),
	})
	disp := &fakeDispatcher{}
	d := NewDunner(store,
		WithDunnerDispatcher(disp),
		WithDunnerClock(func() time.Time { return clock }),
		WithDunnerCooldown(1*time.Second),
	)
	require.NoError(t,
		d.Tick(context.Background()))

	row := store.get("org_future")
	assert.Equal(t, 3,
		row.DunningStep,
	)
	assert.Equal(t, 0,
		countEvent(dispatchedEventTypes(
			disp), domain.
			WebhookEventBillingDelinquent,
		))
}

// Stripe webhook replays: handlePaymentFailed delivered twice must only set
// dunning_entered_at on the first delivery. The second call is a no-op so
// the dunning timer is not reset.
func TestDunner_StartDunningReplayPreservesEnteredAt(t *testing.T) {
	t.Parallel()
	store := newFakeDunningStore()
	store.seed(&fakeDunningRow{
		OrgID:    "org_replay",
		PlanTier: string(domain.PlanPro),
	})
	first := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t,
		store.
			StartDunning(context.
				Background(),

				"org_replay", first))
	require.NoError(t,
		store.
			StartDunning(context.
				Background(),

				"org_replay", first.Add(2*
					time.
						Hour)))

	row := store.get("org_replay")
	assert.True(t, row.
		DunningEnteredAt.
		Equal(first))
}

// A failing dispatcher at step 6 must NOT prevent the payment_status flip to
// 'suspended'. Webhook delivery is fire-and-forget; the local state machine
// is the authority for the suspension transition.
func TestDunner_Step6FlipsPaymentStatusWhenDispatcherDown(t *testing.T) {
	t.Parallel()
	entered := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	clock := entered.Add(75 * 24 * time.Hour)
	store := newFakeDunningStore()
	store.seed(&fakeDunningRow{
		OrgID:            "org_dispatch_down",
		PlanTier:         string(domain.PlanPro),
		DunningStep:      1,
		DunningEnteredAt: entered,
	})
	d := NewDunner(store,
		WithDunnerDispatcher(&failingDispatcher{}),
		WithDunnerClock(func() time.Time { return clock }),
		WithDunnerCooldown(1*time.Second),
	)
	require.NoError(t,
		d.Tick(context.Background()))

	row := store.get("org_dispatch_down")
	assert.Equal(t, "suspended",

		row.PaymentStatus,
	)
	assert.Equal(t, DunningStepDay74,

		row.DunningStep,
	)
}

// failingDispatcher counts attempts so the test asserts the dispatcher *was*
// tried (proving fire-and-forget) but did not block the transition.
type failingDispatcher struct{ attempts atomic.Int32 }

func (f *failingDispatcher) DispatchBillingEvent(_ context.Context, _ string, _ string, _ []byte) error {
	f.attempts.Add(1)
	return errDispatcherDown
}

var errDispatcherDown = newSentinelError("dispatcher unavailable")

func newSentinelError(msg string) error { return &sentinelError{msg: msg} }

type sentinelError struct{ msg string }

func (e *sentinelError) Error() string { return e.msg }

// fmtOrgID returns a deterministic dummy org_id for fan-out tests.
func fmtOrgID(i int) string {
	const hex = "0123456789abcdef"
	out := []byte("00000000-0000-0000-0000-0000000000__")
	out[len(out)-2] = hex[(i>>4)&0xF]
	out[len(out)-1] = hex[i&0xF]
	return string(out)
}
