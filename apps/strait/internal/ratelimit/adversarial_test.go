package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestRateLimit_ZeroWindow verifies behavior when window is zero.
func TestRateLimit_ZeroWindow(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(_ context.Context, cmd redis.Cmder) error {
		t.Fatal("redis should not be called for zero window")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	result, err := limiter.Allow(t.Context(), "key", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Zero window should be handled gracefully (fail-open).
	if !result.Allowed {
		t.Fatal("expected allowed for zero window (fail-open)")
	}
}

// TestRateLimit_NegativeWindow verifies behavior when window is negative.
func TestRateLimit_NegativeWindow(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(_ context.Context, cmd redis.Cmder) error {
		t.Fatal("redis should not be called for negative window")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	result, err := limiter.Allow(t.Context(), "key", 10, -1*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Negative window should be handled gracefully (fail-open).
	if !result.Allowed {
		t.Fatal("expected allowed for negative window (fail-open)")
	}
}

// TestRateLimit_MaxIntRequests verifies behavior with math.MaxInt as the limit.
func TestRateLimit_MaxIntRequests(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	counts := map[string]int{}
	client := newMockRedisClient(func(_ context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		if len(args) < 6 {
			return errors.New("unexpected eval args")
		}
		key, _ := args[3].(string)
		limitRaw := args[5]
		limit, err := strconv.Atoi(fmt.Sprint(limitRaw))
		if err != nil {
			return err
		}

		mu.Lock()
		counts[key]++
		current := counts[key]
		mu.Unlock()

		var result []any
		if current > limit {
			result = []any{int64(0), int64(0)}
		} else {
			result = []any{int64(1), int64(limit - current)}
		}
		if c, ok := cmd.(*redis.Cmd); ok {
			c.SetVal(result)
			return nil
		}
		return errors.New("unexpected command type")
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	result, err := limiter.Allow(t.Context(), "maxint-key", math.MaxInt, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed with MaxInt limit")
	}
}

// TestRateLimit_ConcurrentAccess verifies thread safety with 100 goroutines hitting the same key.
func TestRateLimit_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	counts := map[string]int{}
	client := newMockRedisClient(func(_ context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		if len(args) < 6 {
			return errors.New("unexpected eval args")
		}
		key, _ := args[3].(string)
		limitRaw := args[5]
		limit, err := strconv.Atoi(fmt.Sprint(limitRaw))
		if err != nil {
			return err
		}

		mu.Lock()
		counts[key]++
		current := counts[key]
		mu.Unlock()

		var result []any
		if current > limit {
			result = []any{int64(0), int64(0)}
		} else {
			result = []any{int64(1), int64(limit - current)}
		}
		if c, ok := cmd.(*redis.Cmd); ok {
			c.SetVal(result)
			return nil
		}
		return errors.New("unexpected command type")
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	const goroutines = 100
	const limit = 50

	var allowed atomic.Int64
	var denied atomic.Int64
	var wg sync.WaitGroup

	for range goroutines {
		wg.Go(func() {
			result, err := limiter.Allow(t.Context(), "concurrent-key", limit, time.Minute)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if result.Allowed {
				allowed.Add(1)
			} else {
				denied.Add(1)
			}
		})
	}

	wg.Wait()

	totalAllowed := allowed.Load()
	totalDenied := denied.Load()
	if totalAllowed+totalDenied != goroutines {
		t.Fatalf("total requests = %d, want %d", totalAllowed+totalDenied, goroutines)
	}
	if totalAllowed > limit {
		t.Fatalf("allowed = %d, exceeds limit %d", totalAllowed, limit)
	}
	if totalDenied < goroutines-int64(limit) {
		t.Fatalf("denied = %d, expected at least %d", totalDenied, goroutines-limit)
	}
}

// TestRateLimit_EdgeTimestamps verifies behavior with Unix epoch and far-future timestamps.
func TestRateLimit_EdgeTimestamps(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	counts := map[string]int{}
	client := newMockRedisClient(func(_ context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		if len(args) < 6 {
			return errors.New("unexpected eval args")
		}
		key, _ := args[3].(string)
		limitRaw := args[5]
		limit, err := strconv.Atoi(fmt.Sprint(limitRaw))
		if err != nil {
			return err
		}

		mu.Lock()
		counts[key]++
		current := counts[key]
		mu.Unlock()

		var result []any
		if current > limit {
			result = []any{int64(0), int64(0)}
		} else {
			result = []any{int64(1), int64(limit - current)}
		}
		if c, ok := cmd.(*redis.Cmd); ok {
			c.SetVal(result)
			return nil
		}
		return errors.New("unexpected command type")
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)

	// Very small positive window (1 nanosecond).
	result, err := limiter.Allow(t.Context(), "epoch-key", 10, time.Nanosecond)
	if err != nil {
		t.Fatalf("unexpected error with nanosecond window: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed with nanosecond window")
	}

	// Very large window (approaching max duration).
	result, err = limiter.Allow(t.Context(), "future-key", 10, 24*365*100*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error with huge window: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed with huge window")
	}
}

// FuzzRateLimitWindow fuzzes the rate limiter with various window, request, and key values.
func FuzzRateLimitWindow(f *testing.F) {
	f.Add("key", 10, int64(60000))
	f.Add("", 0, int64(0))
	f.Add("k", 1, int64(1))
	f.Add("rate:user:1", 100, int64(3600000))
	f.Add("negative", -1, int64(-1000))

	f.Fuzz(func(t *testing.T, key string, limit int, windowMs int64) {
		// Construct a disabled limiter to avoid needing real Redis.
		limiter := NewRedisRateLimiter(nil, true)

		// Must not panic regardless of input.
		window := time.Duration(windowMs) * time.Millisecond
		result, err := limiter.Allow(context.Background(), key, limit, window)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Nil client always fails open.
		if !result.Allowed {
			t.Fatal("nil client should always allow")
		}
	})
}
