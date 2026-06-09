package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func BenchmarkRedisRateLimiterAllow(b *testing.B) {
	ctx := context.Background()
	client := newMockRedisClient(func(_ context.Context, cmd redis.Cmder) error {
		if c, ok := cmd.(*redis.Cmd); ok {
			args := cmd.Args()
			if len(args) < 6 {
				return errors.New("unexpected eval args")
			}

			limit, err := strconv.Atoi(fmt.Sprint(args[5]))
			if err != nil {
				return err
			}
			if limit <= 0 {
				return errors.New("invalid limit")
			}

			c.SetVal([]any{int64(1), int64(limit - 1)})
			return nil
		}

		return errors.New("unexpected command type")
	})
	b.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		result, err := limiter.Allow(ctx, "rate:user:bench", 1000, time.Minute)
		if err != nil {
			b.Fatalf("Allow() error = %v", err)
		}
		if !result.Allowed {
			b.Fatal("Allow() = false, want true")
		}
	}
}

func BenchmarkRedisRateLimiterAllowEffectivelyUnlimited(b *testing.B) {
	ctx := context.Background()
	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		b.Fatal("redis should not be called for effectively unlimited limits")
		return nil
	})
	b.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		result, err := limiter.Allow(ctx, "rate:user:bench", effectivelyUnlimitedRequests, time.Minute)
		if err != nil {
			b.Fatalf("Allow() error = %v", err)
		}
		if !result.Allowed {
			b.Fatal("Allow() = false, want true")
		}
	}
}

func BenchmarkRedisConcurrencySlotKey(b *testing.B) {
	for b.Loop() {
		if got := redisConcurrencySlotKey("queue:alpha:worker", 128); got == "" {
			b.Fatal("redisConcurrencySlotKey() returned empty string")
		}
	}
}

func BenchmarkParseRedisConcurrencyToken(b *testing.B) {
	for b.Loop() {
		slot, id, err := parseRedisConcurrencyToken("128:01234567-89ab-cdef-0123-456789abcdef")
		if err != nil {
			b.Fatalf("parseRedisConcurrencyToken() error = %v", err)
		}
		if slot != 128 || id == "" {
			b.Fatalf("parseRedisConcurrencyToken() = %d, %q", slot, id)
		}
	}
}

func BenchmarkRedisConcurrencyLimiterAcquire(b *testing.B) {
	ctx := context.Background()
	client := newMockRedisClient(func(_ context.Context, cmd redis.Cmder) error {
		status, ok := cmd.(*redis.StatusCmd)
		if !ok {
			return errors.New("unexpected command type")
		}
		status.SetVal("OK")
		return nil
	})
	b.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		token, allowed, err := limiter.Acquire(ctx, "queue:alpha:worker", 256, time.Minute)
		if err != nil {
			b.Fatalf("Acquire() error = %v", err)
		}
		if !allowed || token == "" {
			b.Fatalf("Acquire() = %q, %v", token, allowed)
		}
	}
}
