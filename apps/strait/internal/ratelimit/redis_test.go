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
	"github.com/stretchr/testify/require"
)

func TestRedisRateLimiterAllow_NilClientFailOpen(t *testing.T) {
	t.Parallel()

	limiter := NewRedisRateLimiter(nil, true)
	result, err := limiter.Allow(t.Context(), "key", 1, time.Minute)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)

}

func TestRedisRateLimiterAllow_DisabledBypassesRedis(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called when limiter is disabled")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, false)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.Allow(ctx, "key", 1, time.Minute)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)

}

func TestRedisRateLimiterAllow_EffectivelyUnlimitedBypassesRedis(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called for effectively unlimited fail-open limits")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	result, err := limiter.Allow(t.Context(), "key", effectivelyUnlimitedRequests, time.Minute)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)
	require.Equal(t, effectivelyUnlimitedRequests,

		result.Remaining,
	)

}

func TestRedisRateLimiterAllow_EffectivelyUnlimitedDailyWindowStillUsesRedis(t *testing.T) {
	t.Parallel()

	calls := 0
	client := newMockRedisClient(func(_ context.Context, cmd redis.Cmder) error {
		calls++
		if c, ok := cmd.(*redis.Cmd); ok {
			c.SetVal([]any{int64(1), int64(effectivelyUnlimitedRequests - 1)})
			return nil
		}
		return errors.New("unexpected command type")
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	result, err := limiter.Allow(t.Context(), "key", effectivelyUnlimitedRequests, 24*time.Hour)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)
	require.Equal(t, 1,
		calls)

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
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)
	require.Equal(t, 1,
		result.Remaining,
	)

	result, err = limiter.Allow(ctx, "rate:user:1", 2, time.Minute)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)
	require.Equal(t, 0,
		result.Remaining,
	)

	result, err = limiter.Allow(ctx, "rate:user:1", 2, time.Minute)
	require.NoError(t,
		err)
	require.False(t, result.
		Allowed,
	)

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
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)

}

func TestRedisRateLimiterAllow_ZeroLimit_Allowed(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called for zero limit")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.Allow(ctx, "key", 0, time.Minute)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)

}

func TestRedisRateLimiterAllow_ZeroWindow_Allowed(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called for zero window")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.Allow(ctx, "key", 10, 0)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)

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
	require.True(t, r1.
		Allowed)

	// Key A: second request should be rejected.
	r2, _ := limiter.Allow(ctx, "rl:apikey:A", 1, time.Minute)
	require.False(t, r2.
		Allowed)

	// Key B: first request should be allowed (independent counter).
	r3, _ := limiter.Allow(ctx, "rl:apikey:B", 1, time.Minute)
	require.True(t, r3.
		Allowed)

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
	require.Error(t, err)

}

func TestRedisRateLimiterAllowStrict_ZeroLimit_Allowed(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called for zero limit in AllowStrict")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.AllowStrict(ctx, "key", 0, time.Minute)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)

}

func TestRedisRateLimiterAllowStrict_EffectivelyUnlimitedStillUsesRedis(t *testing.T) {
	t.Parallel()

	calls := 0
	client := newMockRedisClient(func(_ context.Context, cmd redis.Cmder) error {
		calls++
		if c, ok := cmd.(*redis.Cmd); ok {
			c.SetVal([]any{int64(1), int64(effectivelyUnlimitedRequests - 1)})
			return nil
		}
		return errors.New("unexpected command type")
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	result, err := limiter.AllowStrict(t.Context(), "key", effectivelyUnlimitedRequests, time.Minute)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)
	require.Equal(t, 1,
		calls)

}

func TestRedisRateLimiterAllowStrict_ZeroWindow_Allowed(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called for zero window in AllowStrict")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.AllowStrict(ctx, "key", 10, 0)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)

}

func TestRedisRateLimiterAllowStrict_NegativeLimit_Allowed(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called for negative limit in AllowStrict")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.AllowStrict(ctx, "key", -5, time.Minute)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)

}

func TestRedisRateLimiterAllowStrict_NegativeWindow_Allowed(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called for negative window in AllowStrict")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.AllowStrict(ctx, "key", 10, -time.Second)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)

}

func TestRedisRateLimiterAllowStrict_NilClient_FailOpen(t *testing.T) {
	t.Parallel()

	limiter := NewRedisRateLimiter(nil, true)
	result, err := limiter.AllowStrict(t.Context(), "key", 10, time.Minute)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)
	require.Equal(t, 10,
		result.Remaining,
	)

}

func TestRedisRateLimiterAllowStrict_Disabled_FailOpen(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called when disabled")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisRateLimiter(client, false)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := limiter.AllowStrict(ctx, "key", 10, time.Minute)
	require.NoError(t,
		err)
	require.True(t, result.
		Allowed,
	)

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
	require.Error(t, err)

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
	require.NoError(t,
		err)
	require.True(t, r1.
		Allowed)

	r2, err := limiter.AllowStrict(ctx, "key", 2, time.Minute)
	require.NoError(t,
		err)
	require.True(t, r2.
		Allowed)

	r3, err := limiter.AllowStrict(ctx, "key", 2, time.Minute)
	require.NoError(t,
		err)
	require.False(t, r3.
		Allowed)

}
