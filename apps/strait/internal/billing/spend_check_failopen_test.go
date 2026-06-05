package billing

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

// TestSpendCheck_ContendedLock_FailsOpen verifies that a sibling goroutine
// holding the legacy "strait:spend_check:<org>" lock does NOT block the
// primary CheckSpendingLimit caller. The lock is gone entirely, so the
// wall-clock cost of the check is unaffected by contention.
func TestSpendCheck_ContendedLock_FailsOpen(t *testing.T) {
	t.Parallel()

	enforcer, _ := setupSpendingEnforcer(t, "org-cont", "pro", 100_000_000, 9_990_000)

	// Pre-acquire the legacy lock from a sibling goroutine; the primary
	// caller must not wait for it to expire.
	ctx := context.Background()
	_, _ = enforcer.rdb.SetNX(ctx, "strait:spend_check:org-cont", "sibling", 5*time.Second).Result()

	start := time.Now()
	require.NoError(t,
		enforcer.CheckSpendingLimit(ctx,

			"org-cont"))

	elapsed := time.Since(start)
	require.LessOrEqual(t, elapsed,
		100*time.
			Millisecond,
	)

}

// TestSpendCheck_NoBlockingSleep guards against re-introducing the sleep
// loop. The legacy code blocked the caller up to 600ms (3 × 200ms) under
// contention; the fail-open path must complete well under that.
func TestSpendCheck_NoBlockingSleep(t *testing.T) {
	t.Parallel()

	enforcer, _ := setupSpendingEnforcer(t, "org-fast", "pro", 100_000_000, 9_990_000)
	ctx := context.Background()
	// Hold the lock continuously so any sleep-retry path would observe it.
	_, _ = enforcer.rdb.SetNX(ctx, "strait:spend_check:org-fast", "x", 30*time.Second).Result()

	const trials = 5
	for range trials {
		start := time.Now()
		require.NoError(t,
			enforcer.CheckSpendingLimit(ctx,

				"org-fast"))
		require.LessOrEqual(t, time.Since(start), 150*
			time.
				Millisecond)

	}
}

// TestSpendCheck_ConcurrentOrgUpdates fans out 50 concurrent spend checks
// against the same org and asserts every call completes within a short
// budget and produces a correct verdict. The legacy implementation would
// have serialized these via a 200ms sleep loop, blowing the P95 well past
// 50ms.
func TestSpendCheck_ConcurrentOrgUpdates(t *testing.T) {
	t.Parallel()

	// Under the limit: every call should pass.
	enforcer, _ := setupSpendingEnforcer(t, "org-conc", "pro", 100_000_000, 9_990_000)
	ctx := context.Background()

	const goroutines = 50
	var (
		wg       conc.WaitGroup
		failures atomic.Int64
		maxLat   atomic.Int64
	)

	for range goroutines {
		wg.Go(func() {
			start := time.Now()
			err := enforcer.CheckSpendingLimit(ctx, "org-conc")
			elapsed := time.Since(start).Nanoseconds()
			if err != nil {
				failures.Add(1)
			}
			for {
				prev := maxLat.Load()
				if elapsed <= prev || maxLat.CompareAndSwap(prev, elapsed) {
					break
				}
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 0, failures.Load())
	require.LessOrEqual(t, time.Duration(maxLat.
		Load()),

		200*time.Millisecond)

}

// TestSpendCheck_OverLimit_StillBlocks ensures the cleanup did not weaken
// enforcement: an over-limit org must still receive a *LimitError even when
// the legacy lock is held by a sibling.
func TestSpendCheck_OverLimit_StillBlocks(t *testing.T) {
	t.Parallel()

	enforcer, _ := setupSpendingEnforcer(t, "org-over", "pro", 50_000_000, 100_000_000)
	ctx := context.Background()
	_, _ = enforcer.rdb.SetNX(ctx, "strait:spend_check:org-over", "sibling", 5*time.Second).Result()

	err := enforcer.CheckSpendingLimit(ctx, "org-over")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(err,
		&le))
	require.Equal(t,
		"spending_limit_reached",

		le.Code)

}
