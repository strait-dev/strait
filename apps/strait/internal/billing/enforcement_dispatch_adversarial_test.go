package billing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 100 concurrent CheckSpendingLimit calls at 85% spend must dispatch the
// cap_warning event exactly once. The mock store's TryMarkBillingCapEvent
// is mutex-guarded to mirror PgStore's UPDATE ... WHERE col IS NULL atomicity.
func TestCheckSpendingLimit_ConcurrentWarningDispatchedOnce(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	sub := newPaidSubscription("org_race", string(domain.PlanPro), 1_000_000, "block")
	e, _, d := newFakeDispatcherEnforcer(t, sub, 850_000)

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		concWG.Go(func() {
			defer wg.Done()
			_ = e.CheckSpendingLimit(context.Background(), sub.OrgID)
		})
	}
	wg.Wait()

	got := dispatchedEventTypes(d)
	assert.EqualValues(t, 1,
		countEvent(got,
			domain.WebhookEventBillingCapWarning,
		))

}

// A clock-skew bounce (79% → 81% → 79%) must still produce exactly one
// cap_warning dispatch — the column write commits on the first 81% pass and
// blocks the second crossing.
func TestCheckSpendingLimit_ClockSkewBounceDispatchesOnce(t *testing.T) {
	t.Parallel()

	sub := newPaidSubscription("org_skew", string(domain.PlanPro), 1_000_000, "block")
	store := &mockBillingStore{
		subscriptions:    map[string]*OrgSubscription{sub.OrgID: sub},
		periodSpendByOrg: map[string]int64{sub.OrgID: 0},
	}
	d := &fakeDispatcher{}
	e := NewEnforcer(store, nil, nil, WithBillingDispatcher(d))

	ctx := context.Background()
	for _, spend := range []int64{790_000, 810_000, 790_000, 815_000} {
		store.periodSpendByOrg[sub.OrgID] = spend
		_ = e.CheckSpendingLimit(ctx, sub.OrgID)
	}
	got := dispatchedEventTypes(d)
	assert.EqualValues(t, 1,
		countEvent(got,
			domain.WebhookEventBillingCapWarning,
		))

}

// A webhook delivery failure must NOT block the enforcer's return path.
// CheckSpendingLimit at 100% returns a *LimitError; a flaky dispatcher
// returning an error from DispatchBillingEvent must not promote that to
// the enforcer's return value.
func TestCheckSpendingLimit_DispatchFailureDoesNotBlockReturn(t *testing.T) {
	t.Parallel()

	sub := newPaidSubscription("org_disperror", string(domain.PlanPro), 1_000_000, "block")
	store := &mockBillingStore{
		subscriptions:    map[string]*OrgSubscription{sub.OrgID: sub},
		periodSpendByOrg: map[string]int64{sub.OrgID: 1_500_000},
	}
	d := &fakeDispatcher{err: errors.New("delivery queue down")}
	e := NewEnforcer(store, nil, nil, WithBillingDispatcher(d))

	err := e.CheckSpendingLimit(context.Background(), sub.OrgID)
	var limitErr *LimitError
	require.True(t, errors.As(err,
		&limitErr,
	))

}

// Period-boundary race: at the millisecond a new billing period opens,
// the very first cap_warning dispatch in the new period must fire even if
// the previous period already fired one. The reset is simulated by
// dropping the org's cap-event marks (PgStore does this in UpsertOrgSubscription
// on current_period_start rollover).
func TestCheckSpendingLimit_PeriodBoundaryRace(t *testing.T) {
	t.Parallel()

	sub := newPaidSubscription("org_boundary", string(domain.PlanPro), 1_000_000, "block")
	e, store, d := newFakeDispatcherEnforcer(t, sub, 850_000)
	require.NoError(t,
		e.CheckSpendingLimit(context.
			Background(), sub.OrgID))

	// Roll over: drop dedup marks AND advance current_period_start.
	newStart := sub.CurrentPeriodStart.Add(30 * 24 * time.Hour)
	rolled := *sub
	rolled.CurrentPeriodStart = &newStart
	store.subscriptions[sub.OrgID] = &rolled
	store.mu.Lock()
	delete(store.capEventMarks, sub.OrgID)
	store.mu.Unlock()
	require.NoError(t,
		e.CheckSpendingLimit(context.
			Background(), sub.OrgID))

	got := dispatchedEventTypes(d)
	assert.EqualValues(t, 2,
		countEvent(got,
			domain.WebhookEventBillingCapWarning,
		))

}
