package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisRateLimiterAllow_RemainingCountAccurate(t *testing.T) {
	t.Parallel()

	counter := 0
	limit := 5
	client := newMockRedisClient(func(_ context.Context, cmd redis.Cmder) error {
		counter++
		c, ok := cmd.(*redis.Cmd)
		if !ok {
			t.Fatal("unexpected command type")
		}
		remaining := max(limit-counter, 0)
		allowed := int64(1)
		if counter > limit {
			allowed = 0
		}
		c.SetVal([]any{allowed, int64(remaining)})
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	for i := 1; i <= limit; i++ {
		result, err := limiter.Allow(ctx, "key", limit, time.Minute)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		if !result.Allowed {
			t.Fatalf("request %d: expected allowed", i)
		}
		expectedRemaining := limit - i
		if result.Remaining != expectedRemaining {
			t.Fatalf("request %d: expected remaining=%d, got %d", i, expectedRemaining, result.Remaining)
		}
	}

	// Next request should be rejected with remaining=0.
	result, err := limiter.Allow(ctx, "key", limit, time.Minute)
	if err != nil {
		t.Fatalf("rejected request: unexpected error: %v", err)
	}
	if result.Allowed {
		t.Fatal("expected request to be rejected")
	}
	if result.Remaining != 0 {
		t.Fatalf("expected remaining=0 on rejection, got %d", result.Remaining)
	}
}

func TestRedisRateLimiterAllow_FailOpenReturnsFullRemaining(t *testing.T) {
	t.Parallel()

	limiter := NewRedisRateLimiter(nil, true)
	result, err := limiter.Allow(t.Context(), "key", 100, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed on fail-open")
	}
	if result.Remaining != 100 {
		t.Fatalf("expected remaining=100 on fail-open, got %d", result.Remaining)
	}
}
