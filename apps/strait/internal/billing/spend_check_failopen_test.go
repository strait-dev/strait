package billing

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
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
	if err := enforcer.CheckSpendingLimit(ctx, "org-cont"); err != nil {
		t.Fatalf("unexpected error under lock contention: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Fatalf("CheckSpendingLimit took %v under contention, expected < 100ms (no sleep-retry)", elapsed)
	}
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
		if err := enforcer.CheckSpendingLimit(ctx, "org-fast"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if d := time.Since(start); d > 150*time.Millisecond {
			t.Fatalf("CheckSpendingLimit took %v, want < 150ms (no sleep-retry)", d)
		}
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

	if failures.Load() != 0 {
		t.Fatalf("got %d failures under concurrent check (expected 0)", failures.Load())
	}
	if max := time.Duration(maxLat.Load()); max > 200*time.Millisecond {
		t.Fatalf("max latency = %v, want < 200ms (no sleep-retry serialization)", max)
	}
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
	if err == nil {
		t.Fatal("expected over-limit to block even under contention")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "spending_limit_reached" {
		t.Fatalf("Code = %q, want spending_limit_reached", le.Code)
	}
}
