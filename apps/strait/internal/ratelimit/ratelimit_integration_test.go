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
	if err := testRedis.FlushAll(context.Background()); err != nil {
		t.Fatalf("flush redis: %v", err)
	}
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
		if err != nil {
			t.Fatalf("Allow() call %d error = %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("Allow() call %d denied, want allowed", i)
		}
		wantRemaining := limit - 1 - i
		if res.Remaining != wantRemaining {
			t.Errorf("Allow() call %d remaining = %d, want %d", i, res.Remaining, wantRemaining)
		}
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
	for i := range limit {
		res, err := rl.Allow(ctx, key, limit, window)
		if err != nil {
			t.Fatalf("Allow() call %d error = %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("Allow() call %d denied prematurely", i)
		}
	}

	// Next request should be denied.
	res, err := rl.Allow(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("Allow() over-limit error = %v", err)
	}
	if res.Allowed {
		t.Error("Allow() over-limit allowed, want denied")
	}
	if res.Remaining != 0 {
		t.Errorf("Allow() over-limit remaining = %d, want 0", res.Remaining)
	}
}

func TestAllow_WindowExpiry(t *testing.T) {
	flushRedis(t)
	rl := newRateLimiter(t)
	ctx := context.Background()

	key := "test:window-expiry"
	limit := 2
	window := 500 * time.Millisecond

	// Exhaust the limit.
	for i := range limit {
		res, err := rl.Allow(ctx, key, limit, window)
		if err != nil {
			t.Fatalf("Allow() call %d error = %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("Allow() call %d denied prematurely", i)
		}
	}

	// Denied immediately.
	res, err := rl.Allow(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("Allow() over-limit error = %v", err)
	}
	if res.Allowed {
		t.Error("Allow() should deny after limit exhausted")
	}

	// Wait for the window to expire.
	time.Sleep(600 * time.Millisecond)

	// Should be allowed again.
	res, err = rl.Allow(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("Allow() after expiry error = %v", err)
	}
	if !res.Allowed {
		t.Error("Allow() denied after window expiry, want allowed")
	}
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
			if err != nil {
				t.Errorf("Allow() error = %v", err)
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

	if allowed != limit {
		t.Errorf("concurrent allowed = %d, want %d", allowed, limit)
	}
	if denied != goroutines-limit {
		t.Errorf("concurrent denied = %d, want %d", denied, goroutines-limit)
	}
}

func TestAllow_MultipleKeys(t *testing.T) {
	flushRedis(t)
	rl := newRateLimiter(t)
	ctx := context.Background()

	window := 10 * time.Second

	// Two independent keys should have independent limits.
	for i := range 3 {
		res, err := rl.Allow(ctx, "test:key-a", 3, window)
		if err != nil {
			t.Fatalf("key-a call %d error = %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("key-a call %d denied, want allowed", i)
		}
	}

	// key-a is now exhausted.
	res, err := rl.Allow(ctx, "test:key-a", 3, window)
	if err != nil {
		t.Fatalf("key-a over-limit error = %v", err)
	}
	if res.Allowed {
		t.Error("key-a should be denied after exhaustion")
	}

	// key-b should still be fully available.
	res, err = rl.Allow(ctx, "test:key-b", 3, window)
	if err != nil {
		t.Fatalf("key-b error = %v", err)
	}
	if !res.Allowed {
		t.Error("key-b denied, want allowed (independent from key-a)")
	}
	if res.Remaining != 2 {
		t.Errorf("key-b remaining = %d, want 2", res.Remaining)
	}
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
	for i := range 2 {
		res, err := rl1.Allow(ctx, key, limit, window)
		if err != nil {
			t.Fatalf("rl1 call %d error = %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("rl1 call %d denied prematurely", i)
		}
	}

	// Client 2 uses the remaining 2 slots.
	for i := range 2 {
		res, err := rl2.Allow(ctx, key, limit, window)
		if err != nil {
			t.Fatalf("rl2 call %d error = %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("rl2 call %d denied prematurely", i)
		}
	}

	// Both clients should now be denied.
	res, err := rl1.Allow(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("rl1 over-limit error = %v", err)
	}
	if res.Allowed {
		t.Error("rl1 allowed after limit exhausted by shared counter")
	}

	res, err = rl2.Allow(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("rl2 over-limit error = %v", err)
	}
	if res.Allowed {
		t.Error("rl2 allowed after limit exhausted by shared counter")
	}
}

func TestAllow_DisabledLimiter(t *testing.T) {
	client := redis.NewClient(testRedis.Options())
	t.Cleanup(func() { _ = client.Close() })

	rl := ratelimit.NewRedisRateLimiter(client, false)
	ctx := context.Background()

	res, err := rl.Allow(ctx, "test:disabled", 1, time.Second)
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if !res.Allowed {
		t.Error("disabled limiter denied a request, want allowed")
	}
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
	if err != nil {
		t.Fatalf("Acquire() #1 error = %v", err)
	}
	if !ok {
		t.Fatal("Acquire() #1 denied, want allowed")
	}
	if token1 == "" {
		t.Fatal("Acquire() #1 returned empty token")
	}

	// Acquire second slot.
	token2, ok, err := cl.Acquire(ctx, key, maxConcurrent, ttl)
	if err != nil {
		t.Fatalf("Acquire() #2 error = %v", err)
	}
	if !ok {
		t.Fatal("Acquire() #2 denied, want allowed")
	}
	if token2 == "" {
		t.Fatal("Acquire() #2 returned empty token")
	}

	// Third acquire should be denied (all slots taken).
	_, ok, err = cl.Acquire(ctx, key, maxConcurrent, ttl)
	if err != nil {
		t.Fatalf("Acquire() #3 error = %v", err)
	}
	if ok {
		t.Error("Acquire() #3 allowed, want denied (all slots taken)")
	}

	// Release first slot.
	if err := cl.Release(ctx, key, token1); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	// Now acquire should succeed again.
	token3, ok, err := cl.Acquire(ctx, key, maxConcurrent, ttl)
	if err != nil {
		t.Fatalf("Acquire() #4 error = %v", err)
	}
	if !ok {
		t.Error("Acquire() #4 denied after release, want allowed")
	}
	if token3 == "" {
		t.Error("Acquire() #4 returned empty token")
	}
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
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if !ok {
		t.Fatal("Acquire() denied, want allowed")
	}

	// Slot is taken.
	_, ok, err = cl.Acquire(ctx, key, maxConcurrent, ttl)
	if err != nil {
		t.Fatalf("Acquire() while taken error = %v", err)
	}
	if ok {
		t.Error("Acquire() allowed while slot taken, want denied")
	}

	// Wait for TTL to expire.
	time.Sleep(700 * time.Millisecond)

	// Slot should be available again after TTL expiry.
	_, ok, err = cl.Acquire(ctx, key, maxConcurrent, ttl)
	if err != nil {
		t.Fatalf("Acquire() after TTL error = %v", err)
	}
	if !ok {
		t.Error("Acquire() denied after TTL expiry, want allowed")
	}
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
			if err != nil {
				t.Errorf("Acquire() error = %v", err)
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

	if acquired != maxConcurrent {
		t.Errorf("concurrent acquired = %d, want %d", acquired, maxConcurrent)
	}

	// Release all and verify we can acquire again.
	for _, token := range tokens {
		if err := cl.Release(ctx, key, token); err != nil {
			t.Errorf("Release(%q) error = %v", token, err)
		}
	}

	token, ok, err := cl.Acquire(ctx, key, maxConcurrent, ttl)
	if err != nil {
		t.Fatalf("Acquire() after release-all error = %v", err)
	}
	if !ok {
		t.Error("Acquire() denied after releasing all slots")
	}
	if token == "" {
		t.Error("Acquire() returned empty token after release-all")
	}
}

func TestConcurrency_IndependentKeys(t *testing.T) {
	flushRedis(t)
	cl := newConcurrencyLimiter(t)
	ctx := context.Background()

	maxConcurrent := 1
	ttl := 10 * time.Second

	// Acquire slot on key-a.
	_, ok, err := cl.Acquire(ctx, "test:conc-key-a", maxConcurrent, ttl)
	if err != nil {
		t.Fatalf("Acquire(key-a) error = %v", err)
	}
	if !ok {
		t.Fatal("Acquire(key-a) denied")
	}

	// Key-b should be independent and available.
	_, ok, err = cl.Acquire(ctx, "test:conc-key-b", maxConcurrent, ttl)
	if err != nil {
		t.Fatalf("Acquire(key-b) error = %v", err)
	}
	if !ok {
		t.Error("Acquire(key-b) denied, want allowed (independent key)")
	}
}

func TestConcurrency_DisabledLimiter(t *testing.T) {
	client := redis.NewClient(testRedis.Options())
	t.Cleanup(func() { _ = client.Close() })

	cl := ratelimit.NewRedisConcurrencyLimiter(client, false)
	ctx := context.Background()

	_, ok, err := cl.Acquire(ctx, "test:disabled-conc", 1, time.Second)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if !ok {
		t.Error("disabled concurrency limiter denied, want allowed")
	}
}
