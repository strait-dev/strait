//go:build integration

package queue_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/queue"
)

func TestBackpressure_FirstConsumeSucceeds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mustClean(t, ctx)

	bp := queue.NewBackpressure(testDB.Pool, queue.BackpressureConfig{
		DefaultMaxTokens:    5,
		DefaultRefillPerSec: 1,
	}, true)
	if err := bp.TryConsume(ctx, "proj-bp-first"); err != nil {
		t.Errorf("first consume: %v", err)
	}
}

func TestBackpressure_ExhaustionThrottles(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mustClean(t, ctx)

	bp := queue.NewBackpressure(testDB.Pool, queue.BackpressureConfig{
		DefaultMaxTokens:    3,
		DefaultRefillPerSec: 1,
	}, true)
	project := "proj-bp-exhaust"

	// Consume 3 tokens — all should succeed.
	for i := range 3 {
		if err := bp.TryConsume(ctx, project); err != nil {
			t.Fatalf("consume %d: %v", i, err)
		}
	}
	// 4th should throttle.
	err := bp.TryConsume(ctx, project)
	thr, ok := queue.AsThrottled(err)
	if !ok {
		t.Fatalf("expected throttle, got %v", err)
	}
	if thr.RetryAfter <= 0 {
		t.Errorf("retry after = %v, want > 0", thr.RetryAfter)
	}
}

func TestBackpressure_ProjectIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mustClean(t, ctx)

	bp := queue.NewBackpressure(testDB.Pool, queue.BackpressureConfig{
		DefaultMaxTokens:    1,
		DefaultRefillPerSec: 1,
	}, true)

	// Project A drains its bucket.
	if err := bp.TryConsume(ctx, "proj-bp-iso-a"); err != nil {
		t.Fatalf("A first: %v", err)
	}
	if _, ok := queue.AsThrottled(bp.TryConsume(ctx, "proj-bp-iso-a")); !ok {
		t.Error("A should be throttled")
	}
	// Project B unaffected.
	if err := bp.TryConsume(ctx, "proj-bp-iso-b"); err != nil {
		t.Errorf("B should pass, got %v", err)
	}
}

func TestBackpressure_RefillAfterWait(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mustClean(t, ctx)

	bp := queue.NewBackpressure(testDB.Pool, queue.BackpressureConfig{
		DefaultMaxTokens:    1,
		DefaultRefillPerSec: 10, // 10/sec → 100ms per token
	}, true)
	project := "proj-bp-refill"

	if err := bp.TryConsume(ctx, project); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, ok := queue.AsThrottled(bp.TryConsume(ctx, project)); !ok {
		t.Fatal("expected throttle after drain")
	}

	// Wait 250ms — at 10/sec that's 2 tokens worth of refill.
	time.Sleep(250 * time.Millisecond)
	if err := bp.TryConsume(ctx, project); err != nil {
		t.Errorf("after refill: %v", err)
	}
}

func TestBackpressure_DisabledIsNoOp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mustClean(t, ctx)

	bp := queue.NewBackpressure(testDB.Pool, queue.BackpressureConfig{
		DefaultMaxTokens:    1,
		DefaultRefillPerSec: 1,
	}, false)
	// Hammer 1000 times; none should throttle.
	for i := range 1000 {
		if err := bp.TryConsume(ctx, "proj-bp-off"); err != nil {
			t.Fatalf("disabled throttled at %d: %v", i, err)
		}
	}
}

func TestBackpressure_ConcurrentConsumers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)

	bp := queue.NewBackpressure(testDB.Pool, queue.BackpressureConfig{
		DefaultMaxTokens:    10,
		DefaultRefillPerSec: 0, // no refill so the cap is deterministic within the run
	}, true)
	project := "proj-bp-concurrent"

	var allowed, throttled atomic.Int64
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			for range 5 {
				err := bp.TryConsume(ctx, project)
				if err == nil {
					allowed.Add(1)
				} else if _, ok := queue.AsThrottled(err); ok {
					throttled.Add(1)
				}
			}
		})
	}
	wg.Wait()

	// With max=10 and zero refill, we should see exactly 10 allowed
	// plus some slack for the first-insert path which also decrements.
	total := allowed.Load() + throttled.Load()
	if total != 100 {
		t.Errorf("total = %d, want 100", total)
	}
	// Allowed should be at least 1 and at most ~max_tokens + small slack.
	if allowed.Load() < 1 || allowed.Load() > 12 {
		t.Errorf("allowed = %d, want 1..12", allowed.Load())
	}
}
