package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRedisRateLimiterAllow_RemainingCountAccurate(t *testing.T) {
	t.Parallel()

	counter := 0
	limit := 5
	client := newMockRedisClient(func(_ context.Context, cmd redis.Cmder) error {
		counter++
		c, ok := cmd.(*redis.Cmd)
		require.True(t, ok)

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
		require.NoError(t,
			err)
		require.True(t, result.
			Allowed,
		)

		expectedRemaining := limit - i
		require.Equal(t, expectedRemaining,

			result.
				Remaining)
	}

	// Next request should be rejected with remaining=0.
	result, err := limiter.Allow(ctx, "key", limit, time.Minute)
	require.NoError(t,
		err)
	require.False(t, result.
		Allowed,
	)
	require.Equal(t, 0,
		result.Remaining,
	)
}

func TestRedisRateLimiterAllow_FailOpenReturnsFullRemaining(t *testing.T) {
	t.Parallel()

	limiter := NewRedisRateLimiter(nil, true)
	result, err := limiter.Allow(t.Context(), "key", 100, time.Minute)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)
	require.Equal(t, 100,
		result.Remaining,
	)
}
