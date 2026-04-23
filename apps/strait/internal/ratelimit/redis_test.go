package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisRateLimiterAllow_NilClientFailOpen(t *testing.T) {
	t.Parallel()

	limiter := NewRedisRateLimiter(nil, true)
	result, err := limiter.Allow(t.Context(), "key", 1, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed when redis client is nil")
	}
}

func TestRedisRateLimiterAllow_DisabledBypassesRedis(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		t.Fatal("redis should not be called when limiter is disabled")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, false)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.Allow(ctx, "key", 1, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed when limiter is disabled")
	}
}

func TestRedisRateLimiterAllow_EnforcesLimit(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	result, err := limiter.Allow(ctx, "rate:user:1", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected first request to be allowed")
	}
	if result.Remaining != 1 {
		t.Fatalf("expected remaining=1, got %d", result.Remaining)
	}

	result, err = limiter.Allow(ctx, "rate:user:1", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected second request to be allowed")
	}
	if result.Remaining != 0 {
		t.Fatalf("expected remaining=0, got %d", result.Remaining)
	}

	result, err = limiter.Allow(ctx, "rate:user:1", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Allowed {
		t.Fatal("expected third request to be rejected")
	}
}

func TestRedisRateLimiterAllow_RedisErrorFailsOpen(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		return errors.New("redis unavailable")
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.Allow(ctx, "key", 1, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected fail-open behavior on redis error")
	}
}

func TestRedisRateLimiterAllow_ZeroLimit_Allowed(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		t.Fatal("redis should not be called for zero limit")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.Allow(ctx, "key", 0, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed when limit is zero")
	}
}

func TestRedisRateLimiterAllow_ZeroWindow_Allowed(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		t.Fatal("redis should not be called for zero window")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.Allow(ctx, "key", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed when window is zero")
	}
}

func TestRedisRateLimiterAllow_DifferentKeys_Independent(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// Key A: allow 1 request.
	r1, _ := limiter.Allow(ctx, "rl:apikey:A", 1, time.Minute)
	if !r1.Allowed {
		t.Fatal("key A first request should be allowed")
	}

	// Key A: second request should be rejected.
	r2, _ := limiter.Allow(ctx, "rl:apikey:A", 1, time.Minute)
	if r2.Allowed {
		t.Fatal("key A second request should be rejected")
	}

	// Key B: first request should be allowed (independent counter).
	r3, _ := limiter.Allow(ctx, "rl:apikey:B", 1, time.Minute)
	if !r3.Allowed {
		t.Fatal("key B first request should be allowed (independent from key A)")
	}
}

func TestRedisRateLimiterAllowStrict_RedisErrorFailsClosed(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		return errors.New("redis unavailable")
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err := limiter.AllowStrict(ctx, "key", 1, time.Minute)
	if err == nil {
		t.Fatal("expected error on Redis failure, got nil (fail-closed)")
	}
}

func TestRedisRateLimiterAllowStrict_ZeroLimit_Allowed(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		t.Fatal("redis should not be called for zero limit in AllowStrict")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.AllowStrict(ctx, "key", 0, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed when limit is zero")
	}
}

func TestRedisRateLimiterAllowStrict_ZeroWindow_Allowed(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		t.Fatal("redis should not be called for zero window in AllowStrict")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.AllowStrict(ctx, "key", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed when window is zero")
	}
}

func TestRedisRateLimiterAllowStrict_NegativeLimit_Allowed(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		t.Fatal("redis should not be called for negative limit in AllowStrict")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.AllowStrict(ctx, "key", -5, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed when limit is negative")
	}
}

func TestRedisRateLimiterAllowStrict_NegativeWindow_Allowed(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		t.Fatal("redis should not be called for negative window in AllowStrict")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.AllowStrict(ctx, "key", 10, -time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed when window is negative")
	}
}

func TestRedisRateLimiterAllowStrict_NilClient_FailOpen(t *testing.T) {
	t.Parallel()

	limiter := NewRedisRateLimiter(nil, true)
	result, err := limiter.AllowStrict(t.Context(), "key", 10, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed when client is nil")
	}
	if result.Remaining != 10 {
		t.Fatalf("expected remaining=10, got %d", result.Remaining)
	}
}

func TestRedisRateLimiterAllowStrict_Disabled_FailOpen(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		t.Fatal("redis should not be called when disabled")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, false)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.AllowStrict(ctx, "key", 10, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed when disabled")
	}
}

func TestRedisRateLimiterAllowStrict_ShortResult_Error(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(_ context.Context, cmd redis.Cmder) error {
		if c, ok := cmd.(*redis.Cmd); ok {
			c.SetVal([]any{int64(1)})
			return nil
		}
		return errors.New("unexpected command type")
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err := limiter.AllowStrict(ctx, "key", 10, time.Minute)
	if err == nil {
		t.Fatal("expected error for short script response")
	}
}

func TestRedisRateLimiterAllowStrict_EnforcesLimit(t *testing.T) {
	t.Parallel()

	var counter int
	client := newMockRedisClient(func(_ context.Context, cmd redis.Cmder) error {
		counter++
		limit := 2
		var result []any
		if counter > limit {
			result = []any{int64(0), int64(0)}
		} else {
			result = []any{int64(1), int64(limit - counter)}
		}
		if c, ok := cmd.(*redis.Cmd); ok {
			c.SetVal(result)
			return nil
		}
		return errors.New("unexpected command type")
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	r1, err := limiter.AllowStrict(ctx, "key", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r1.Allowed {
		t.Fatal("first request should be allowed")
	}

	r2, err := limiter.AllowStrict(ctx, "key", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r2.Allowed {
		t.Fatal("second request should be allowed")
	}

	r3, err := limiter.AllowStrict(ctx, "key", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r3.Allowed {
		t.Fatal("third request should be rejected")
	}
}
