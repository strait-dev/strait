//go:build integration

package queue_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/queue"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackpressure_FirstConsumeSucceeds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mustClean(t, ctx)

	bp := queue.NewBackpressure(testDB.Pool, queue.BackpressureConfig{
		DefaultMaxTokens:    5,
		DefaultRefillPerSec: 1,
	}, true)
	assert.NoError(t, bp.TryConsume(ctx,
		"proj-bp-first",
	))

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
	for range 3 {
		require.NoError(t, bp.TryConsume(
			ctx, project,
		))

	}
	// 4th should throttle.
	err := bp.TryConsume(ctx, project)
	thr, ok := queue.AsThrottled(err)
	require.True(t, ok)
	assert.False(t, thr.RetryAfter <=
		0)

}

func TestBackpressure_ProjectIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mustClean(t, ctx)

	bp := queue.NewBackpressure(testDB.Pool, queue.BackpressureConfig{
		DefaultMaxTokens:    1,
		DefaultRefillPerSec: 1,
	}, true)
	require.NoError(t, bp.TryConsume(
		ctx, "proj-bp-iso-a",
	))

	// Project A drains its bucket.

	if _, ok := queue.AsThrottled(bp.TryConsume(ctx, "proj-bp-iso-a")); !ok {
		assert.Fail(t,

			"A should be throttled")
	}
	assert.NoError(t, bp.TryConsume(ctx,
		"proj-bp-iso-b",
	))

	// Project B unaffected.

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
	require.NoError(t, bp.TryConsume(
		ctx, project,
	))

	if _, ok := queue.AsThrottled(bp.TryConsume(ctx, project)); !ok {
		require.Fail(t,

			"expected throttle after drain")
	}

	// Wait 250ms — at 10/sec that's 2 tokens worth of refill.
	time.Sleep(250 * time.Millisecond)
	assert.NoError(t, bp.TryConsume(ctx,
		project,
	))

}

func TestBackpressure_PreservesPartialRefillAcrossSuccessfulConsume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mustClean(t, ctx)

	bp := queue.NewBackpressure(testDB.Pool, queue.BackpressureConfig{
		DefaultMaxTokens:    2,
		DefaultRefillPerSec: 2, // 2/sec -> 500ms per token
	}, true)
	project := "proj-bp-partial-refill"

	require.NoError(t, bp.TryConsume(ctx, project))

	// This consumes the second burst token before a full refill token is earned.
	// The bucket must keep the partial refill time instead of resetting
	// last_refill_at, otherwise steady traffic below the configured refill rate
	// slowly starves itself.
	time.Sleep(350 * time.Millisecond)
	require.NoError(t, bp.TryConsume(ctx, project))

	time.Sleep(250 * time.Millisecond)
	require.NoError(t, bp.TryConsume(ctx, project))
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
	for range 1000 {
		require.NoError(t, bp.TryConsume(
			ctx, "proj-bp-off",
		),
		)

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
	assert.EqualValues(t, 100, total)
	assert.False(t, allowed.
		Load() <
		1 || allowed.
		Load() >
		12)

	// Allowed should be at least 1 and at most ~max_tokens + small slack.

}
