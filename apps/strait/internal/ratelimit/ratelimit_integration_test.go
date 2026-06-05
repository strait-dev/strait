//go:build integration

package ratelimit_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/ratelimit"
	"strait/internal/testutil"
)

var testRedis *testutil.TestRedis

func TestMain(m *testing.M) {
	ctx := context.Background()
	var err error
	testRedis, err = testutil.SetupSharedTestRedis(ctx, "ratelimit")
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup redis: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	testRedis.Cleanup(ctx)
	os.Exit(code)
}

func newRateLimiter(t *testing.T) *ratelimit.RedisRateLimiter {
	t.Helper()
	client := redis.NewClient(testRedis.Options())
	t.Cleanup(func() { _ = client.Close() })
	return ratelimit.NewRedisRateLimiter(client, true)
}

func newConcurrencyLimiter(t *testing.T) *ratelimit.RedisConcurrencyLimiter {
	t.Helper()
	client := redis.NewClient(testRedis.Options())
	t.Cleanup(func() { _ = client.Close() })
	return ratelimit.NewRedisConcurrencyLimiter(client, true)
}

func flushRedis(t *testing.T) {
	t.Helper()
	require.NoError(t, testRedis.
		FlushAll(context.Background()))

}

func TestAllow_WithinLimit(t *testing.T) {
	flushRedis(t)
	rl := newRateLimiter(t)
	ctx := context.Background()

	key := "test:within-limit"
	limit := 5
	window := 10 * time.Second

	for i := range limit {
		res, err := rl.Allow(ctx, key, limit, window)
		require.NoError(t, err)
		require.True(t, res.Allowed)

		wantRemaining := limit - 1 - i
		assert.Equal(t, wantRemaining,

			res.Remaining,
		)

	}
}

func TestAllow_ExceedsLimit(t *testing.T) {
	flushRedis(t)
	rl := newRateLimiter(t)
	ctx := context.Background()

	key := "test:exceeds-limit"
	limit := 3
	window := 10 * time.Second

	// Exhaust the limit.
	for range limit {
		res, err := rl.Allow(ctx, key, limit, window)
		require.NoError(t, err)
		require.True(t, res.Allowed)

	}

	// Next request should be denied.
	res, err := rl.Allow(ctx, key, limit, window)
	require.NoError(t, err)
	assert.False(t, res.Allowed)
	assert.Equal(t, 0, res.Remaining)

}

func TestAllow_WindowExpiry(t *testing.T) {
	flushRedis(t)
	rl := newRateLimiter(t)
	ctx := context.Background()

	key := "test:window-expiry"
	limit := 2
	window := 500 * time.Millisecond

	// Exhaust the limit.
	for range limit {
		res, err := rl.Allow(ctx, key, limit, window)
		require.NoError(t, err)
		require.True(t, res.Allowed)

	}

	// Denied immediately.
	res, err := rl.Allow(ctx, key, limit, window)
	require.NoError(t, err)
	assert.False(t, res.Allowed)

	// Wait for the window to expire.
	time.Sleep(600 * time.Millisecond)

	// Should be allowed again.
	res, err = rl.Allow(ctx, key, limit, window)
	require.NoError(t, err)
	assert.True(t, res.Allowed)

}

func TestAllow_ConcurrentAccess(t *testing.T) {
	flushRedis(t)
	rl := newRateLimiter(t)
	ctx := context.Background()

	key := "test:concurrent"
	limit := 10
	window := 10 * time.Second
	goroutines := 20

	var mu sync.Mutex
	var allowed, denied int

	var wg conc.WaitGroup
	for range goroutines {
		wg.Go(func() {
			res, err := rl.AllowStrict(ctx, key, limit, window)
			if !assert.NoError(t, err) {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if res.Allowed {
				allowed++
			} else {
				denied++
			}
		})
	}
	wg.Wait()
	assert.Equal(t, limit, allowed)
	assert.Equal(t, goroutines-
		limit, denied,
	)

}

func TestAllow_MultipleKeys(t *testing.T) {
	flushRedis(t)
	rl := newRateLimiter(t)
	ctx := context.Background()

	window := 10 * time.Second

	// Two independent keys should have independent limits.
	for range 3 {
		res, err := rl.Allow(ctx, "test:key-a", 3, window)
		require.NoError(t, err)
		require.True(t, res.Allowed)

	}

	// key-a is now exhausted.
	res, err := rl.Allow(ctx, "test:key-a", 3, window)
	require.NoError(t, err)
	assert.False(t, res.Allowed)

	// key-b should still be fully available.
	res, err = rl.Allow(ctx, "test:key-b", 3, window)
	require.NoError(t, err)
	assert.True(t, res.Allowed)
	assert.Equal(t, 2, res.Remaining)

}

func TestAllow_MultipleClients(t *testing.T) {
	flushRedis(t)
	ctx := context.Background()

	rl1 := newRateLimiter(t)
	rl2 := newRateLimiter(t)

	key := "test:shared-key"
	limit := 4
	window := 10 * time.Second

	// Client 1 uses 2 of the 4 slots.
	for range 2 {
		res, err := rl1.Allow(ctx, key, limit, window)
		require.NoError(t, err)
		require.True(t, res.Allowed)

	}

	// Client 2 uses the remaining 2 slots.
	for range 2 {
		res, err := rl2.Allow(ctx, key, limit, window)
		require.NoError(t, err)
		require.True(t, res.Allowed)

	}

	// Both clients should now be denied.
	res, err := rl1.Allow(ctx, key, limit, window)
	require.NoError(t, err)
	assert.False(t, res.Allowed)

	res, err = rl2.Allow(ctx, key, limit, window)
	require.NoError(t, err)
	assert.False(t, res.Allowed)

}

func TestAllow_DisabledLimiter(t *testing.T) {
	client := redis.NewClient(testRedis.Options())
	t.Cleanup(func() { _ = client.Close() })

	rl := ratelimit.NewRedisRateLimiter(client, false)
	ctx := context.Background()

	res, err := rl.Allow(ctx, "test:disabled", 1, time.Second)
	require.NoError(t, err)
	assert.True(t, res.Allowed)

}

func TestConcurrency_AcquireRelease(t *testing.T) {
	flushRedis(t)
	cl := newConcurrencyLimiter(t)
	ctx := context.Background()

	key := "test:concurrency-basic"
	maxConcurrent := 2
	ttl := 10 * time.Second

	// Acquire first slot.
	token1, ok, err := cl.Acquire(ctx, key, maxConcurrent, ttl)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotEqual(t, "", token1)

	// Acquire second slot.
	token2, ok, err := cl.Acquire(ctx, key, maxConcurrent, ttl)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotEqual(t, "", token2)

	// Third acquire should be denied (all slots taken).
	_, ok, err = cl.Acquire(ctx, key, maxConcurrent, ttl)
	require.NoError(t, err)
	assert.False(t, ok)
	require.NoError(t, cl.Release(ctx, key,
		token1))

	// Release first slot.

	// Now acquire should succeed again.
	token3, ok, err := cl.Acquire(ctx, key, maxConcurrent, ttl)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.NotEqual(t, "", token3)

}

func TestConcurrency_TokenExpiration(t *testing.T) {
	flushRedis(t)
	cl := newConcurrencyLimiter(t)
	ctx := context.Background()

	key := "test:concurrency-ttl"
	maxConcurrent := 1
	ttl := 500 * time.Millisecond

	// Acquire the only slot.
	_, ok, err := cl.Acquire(ctx, key, maxConcurrent, ttl)
	require.NoError(t, err)
	require.True(t, ok)

	// Slot is taken.
	_, ok, err = cl.Acquire(ctx, key, maxConcurrent, ttl)
	require.NoError(t, err)
	assert.False(t, ok)

	// Wait for TTL to expire.
	time.Sleep(700 * time.Millisecond)

	// Slot should be available again after TTL expiry.
	_, ok, err = cl.Acquire(ctx, key, maxConcurrent, ttl)
	require.NoError(t, err)
	assert.True(t, ok)

}

func TestConcurrency_ConcurrentAcquire(t *testing.T) {
	flushRedis(t)
	cl := newConcurrencyLimiter(t)
	ctx := context.Background()

	key := "test:concurrency-race"
	maxConcurrent := 5
	ttl := 10 * time.Second
	goroutines := 15

	var mu sync.Mutex
	var acquired int
	tokens := make([]string, 0, maxConcurrent)

	var wg conc.WaitGroup
	for range goroutines {
		wg.Go(func() {
			token, ok, err := cl.Acquire(ctx, key, maxConcurrent, ttl)
			if !assert.NoError(t, err) {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if ok {
				acquired++
				tokens = append(tokens, token)
			}
		})
	}
	wg.Wait()
	assert.Equal(t, maxConcurrent,

		acquired,
	)

	// Release all and verify we can acquire again.
	for _, token := range tokens {
		assert.NoError(t, cl.Release(ctx, key,
			token))

	}

	token, ok, err := cl.Acquire(ctx, key, maxConcurrent, ttl)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.NotEqual(t, "", token)

}

func TestConcurrency_IndependentKeys(t *testing.T) {
	flushRedis(t)
	cl := newConcurrencyLimiter(t)
	ctx := context.Background()

	maxConcurrent := 1
	ttl := 10 * time.Second

	// Acquire slot on key-a.
	_, ok, err := cl.Acquire(ctx, "test:conc-key-a", maxConcurrent, ttl)
	require.NoError(t, err)
	require.True(t, ok)

	// Key-b should be independent and available.
	_, ok, err = cl.Acquire(ctx, "test:conc-key-b", maxConcurrent, ttl)
	require.NoError(t, err)
	assert.True(t, ok)

}

func TestConcurrency_DisabledLimiter(t *testing.T) {
	client := redis.NewClient(testRedis.Options())
	t.Cleanup(func() { _ = client.Close() })

	cl := ratelimit.NewRedisConcurrencyLimiter(client, false)
	ctx := context.Background()

	_, ok, err := cl.Acquire(ctx, "test:disabled-conc", 1, time.Second)
	require.NoError(t, err)
	assert.True(t, ok)

}
