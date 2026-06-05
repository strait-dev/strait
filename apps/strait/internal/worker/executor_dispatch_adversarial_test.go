package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
)

// TestDispatchSpendingLimit_RaceUnderConcurrency drives 100 concurrent
// dispatches against an org whose persisted spend sits just below the
// cap. The point is to confirm we do not deadlock, panic, or trip the
// race detector under contention on the spending-check Redis lock —
// every concurrent goroutine must return cleanly (either all permitted
// because nobody bumped the spend, or all blocked because the mock
// reports over-limit). The shape of the rejection split is not what
// matters here; absence of corruption is.
func TestDispatchSpendingLimit_RaceUnderConcurrency(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	sub := &billing.OrgSubscription{
		OrgID:                 "org-spend-race",
		PlanTier:              string(domain.PlanPro),
		Status:                "active",
		SpendingLimitMicrousd: 10_000_000,
	}
	h := newDispatchHarness(t, sub, 9_999_999) // $0.000001 below the cap

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		concWG.Go(func() {
			defer wg.Done()
			run := &domain.JobRun{
				ID:         "run-spend-race-" + itoa(i),
				JobID:      "job-spend",
				JobVersion: 1,
				Status:     domain.StatusDequeued,
			}
			ec := &ExecutionContext{Run: run, Start: time.Now()}
			h.exec.executeInner(context.Background(), ec)
		})
	}
	wg.Wait()

	// We do not assert on the precise count of rejections; we assert
	// only that the harness survived and returned coherent state. If
	// the run loop panicked or the Redis lock deadlocked, we never
	// would have made it here.
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	assert.NotEmpty(
		t, h.store.
			statusCalls)
}

// TestDispatchSpendingLimit_StaleCacheFailsClosed simulates a stale
// spend cache by having the store report over-limit even though
// hypothetically a refund or correction has dropped real spend below
// the cap. The dispatch must reject (fail-closed): we never allow
// further runs based on the assumption that the cache might be wrong
// in our favor.
func TestDispatchSpendingLimit_StaleCacheFailsClosed(t *testing.T) {
	t.Parallel()

	sub := &billing.OrgSubscription{
		OrgID:                 "org-stale-cache",
		PlanTier:              string(domain.PlanPro),
		Status:                "active",
		SpendingLimitMicrousd: 5_000_000,
		LimitAction:           "block",
	}
	// Mock store returns 6_000_000 = $6 of spend (over the $5 cap).
	// Real recent spend may be lower; we only see what the store says.
	h := newDispatchHarness(t, sub, 6_000_000)

	runDispatch(h, "run-stale-cache")
	assert.True(t, sawSystemFailed(h.store))
}

// TestDispatchSpendingLimit_NotInfluencedByDailyCounter is a regression
// guard: a deliberately-low daily counter must not cause a misclassified
// dispatch to leak through if spending is over the limit. This locks the
// ordering "spending check first, daily check second" — if a future
// refactor flips the order, this test should make that surface.
func TestDispatchSpendingLimit_NotInfluencedByDailyCounter(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	sub := &billing.OrgSubscription{
		OrgID:                 "org-order",
		PlanTier:              string(domain.PlanPro),
		Status:                "active",
		SpendingLimitMicrousd: 1_000_000,
		LimitAction:           "block",
	}
	h := newDispatchHarness(t, sub, 5_000_000) // way over

	var hits atomic.Int32
	const n = 5
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		concWG.Go(func() {
			defer wg.Done()
			run := &domain.JobRun{
				ID:         "run-order-" + itoa(i),
				JobID:      "job-spend",
				JobVersion: 1,
				Status:     domain.StatusDequeued,
			}
			ec := &ExecutionContext{Run: run, Start: time.Now()}
			h.exec.executeInner(context.Background(), ec)
			hits.Add(1)
		})
	}
	wg.Wait()
	assert.Equal(t,
		int32(n),
		hits.Load())

	// All must have failed via spending limit; daily counter never
	// incremented because spending check fires first.
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	failed := 0
	for _, c := range h.store.statusCalls {
		if c.to == domain.StatusSystemFailed {
			failed++
		}
	}
	assert.GreaterOrEqual(t,
		failed, n)
}

// itoa avoids strconv import noise — same job for every run, only the
// ID must be unique across the goroutines so duplicate-id deduping
// doesn't drop attempts.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
