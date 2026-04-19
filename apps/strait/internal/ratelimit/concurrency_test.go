package ratelimit

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisConcurrencyLimiterAcquire_NilClientFailOpen(t *testing.T) {
	t.Parallel()

	limiter := NewRedisConcurrencyLimiter(nil, true)
	token, allowed, err := limiter.Acquire(t.Context(), "job", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed when redis client is nil")
	}
	if token != "" {
		t.Fatalf("expected empty token for fail-open path, got %q", token)
	}

	if err := limiter.Release(t.Context(), "job", "0:any-token"); err != nil {
		t.Fatalf("unexpected error releasing with nil client: %v", err)
	}
}

func TestRedisConcurrencyLimiterAcquire_DisabledBypassesRedis(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		t.Fatal("redis should not be called when limiter is disabled")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, false)
	token, allowed, err := limiter.Acquire(t.Context(), "job", 1, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed when limiter is disabled")
	}
	if token != "" {
		t.Fatalf("expected empty token, got %q", token)
	}
}

func TestRedisConcurrencyLimiterAcquireRelease_EnforcesSlots(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	slots := map[string]string{}
	client := newMockRedisClient(func(_ context.Context, cmd redis.Cmder) error {
		switch c := cmd.(type) {
		case *redis.StatusCmd:
			args := c.Args()
			if len(args) < 3 {
				return errors.New("unexpected set args")
			}
			key, _ := args[1].(string)
			value, _ := args[2].(string)
			mu.Lock()
			_, exists := slots[key]
			if !exists {
				slots[key] = value
			}
			mu.Unlock()
			if exists {
				return redis.Nil
			}
			c.SetVal("OK")
			return nil
		case *redis.Cmd:
			args := c.Args()
			if len(args) < 5 {
				return errors.New("unexpected eval args")
			}
			key, _ := args[3].(string)
			token, _ := args[4].(string)
			mu.Lock()
			stored := slots[key]
			if stored == token {
				delete(slots, key)
				mu.Unlock()
				c.SetVal(int64(1))
				return nil
			}
			mu.Unlock()
			c.SetVal(int64(0))
			return nil
		default:
			return errors.New("unexpected command type")
		}
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	ctx := t.Context()

	tok1, allowed, err := limiter.Acquire(ctx, "queue:alpha", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed || tok1 == "" {
		t.Fatal("expected first acquire to be allowed with token")
	}

	tok2, allowed, err := limiter.Acquire(ctx, "queue:alpha", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed || tok2 == "" {
		t.Fatal("expected second acquire to be allowed with token")
	}

	if _, allowed, err = limiter.Acquire(ctx, "queue:alpha", 2, time.Minute); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if allowed {
		t.Fatal("expected third acquire to be rejected")
	}

	if err := limiter.Release(ctx, "queue:alpha", tok1); err != nil {
		t.Fatalf("unexpected release error: %v", err)
	}

	if _, allowed, err = limiter.Acquire(ctx, "queue:alpha", 2, time.Minute); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if !allowed {
		t.Fatal("expected acquire to succeed after release")
	}
}

func TestRedisConcurrencyLimiterAcquire_RedisErrorFailsOpen(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		return errors.New("redis unavailable")
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	token, allowed, err := limiter.Acquire(t.Context(), "job", 1, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected fail-open behavior")
	}
	if token != "" {
		t.Fatalf("expected empty token in fail-open path, got %q", token)
	}
}

func TestRedisConcurrencyLimiterRelease_InvalidToken(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	if err := limiter.Release(t.Context(), "job", "invalid"); err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestParseRedisConcurrencyToken(t *testing.T) {
	t.Parallel()

	slot, id, err := parseRedisConcurrencyToken("3:abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slot != 3 || id != "abc" {
		t.Fatalf("unexpected token parse result slot=%d id=%q", slot, id)
	}

	if _, _, err := parseRedisConcurrencyToken("x:abc"); err == nil {
		t.Fatal("expected parse error for non-numeric slot")
	}
}

func TestRedisConcurrencyLimiterAcquire_ZeroConcurrency_Error(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		t.Fatal("redis should not be called for zero concurrency")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	_, _, err := limiter.Acquire(t.Context(), "key", 0, time.Minute)
	if err == nil {
		t.Fatal("expected error for zero maxConcurrent")
	}
}

func TestRedisConcurrencyLimiterAcquire_NegativeConcurrency_Error(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		t.Fatal("redis should not be called for negative concurrency")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	_, _, err := limiter.Acquire(t.Context(), "key", -1, time.Minute)
	if err == nil {
		t.Fatal("expected error for negative maxConcurrent")
	}
}

func TestRedisConcurrencyLimiterAcquire_ZeroTTL_Error(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		t.Fatal("redis should not be called for zero TTL")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	_, _, err := limiter.Acquire(t.Context(), "key", 1, 0)
	if err == nil {
		t.Fatal("expected error for zero TTL")
	}
}

func TestRedisConcurrencyLimiterAcquire_NegativeTTL_Error(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		t.Fatal("redis should not be called for negative TTL")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	_, _, err := limiter.Acquire(t.Context(), "key", 1, -time.Second)
	if err == nil {
		t.Fatal("expected error for negative TTL")
	}
}

func TestRedisConcurrencyLimiterRelease_EmptyToken(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	if err := limiter.Release(t.Context(), "job", "0:"); err == nil {
		t.Fatal("expected error for token with empty ID part")
	}
	if err := limiter.Release(t.Context(), "job", ":abc"); err == nil {
		t.Fatal("expected error for token with empty slot part")
	}
	if err := limiter.Release(t.Context(), "job", ""); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestRedisConcurrencyLimiterRelease_RedisErrorFailsOpen(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		return errors.New("redis unavailable")
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	err := limiter.Release(t.Context(), "job", "0:some-id")
	if err != nil {
		t.Fatalf("expected nil (fail-open) on Redis error, got %v", err)
	}
}

func TestRedisConcurrencySlotKey(t *testing.T) {
	t.Parallel()

	key := redisConcurrencySlotKey("group", 7)
	parts := strings.Split(key, ":")
	if len(parts) != 3 {
		t.Fatalf("unexpected key format: %q", key)
	}
	if parts[0] != "concurrency" || parts[1] != "group" {
		t.Fatalf("unexpected key prefix: %q", key)
	}
	if _, err := strconv.Atoi(parts[2]); err != nil {
		t.Fatalf("unexpected slot segment %q: %v", parts[2], err)
	}
}
