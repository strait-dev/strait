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
