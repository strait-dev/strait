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
	"github.com/stretchr/testify/require"
)

func TestRedisConcurrencyLimiterAcquire_NilClientFailOpen(t *testing.T) {
	t.Parallel()

	limiter := NewRedisConcurrencyLimiter(nil, true)
	token, allowed, err := limiter.Acquire(t.Context(), "job", 2, time.Minute)
	require.NoError(t,
		err)
	require.True(t, allowed)
	require.Equal(t, "",
		token)
	require.NoError(t,
		limiter.Release(t.Context(), "job",
			"0:any-token",
		))

}

func TestRedisConcurrencyLimiterAcquire_DisabledBypassesRedis(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called when limiter is disabled")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, false)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	token, allowed, err := limiter.Acquire(ctx, "job", 1, time.Minute)
	require.NoError(t,
		err)
	require.True(t, allowed)
	require.Equal(t, "",
		token)

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
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	tok1, allowed, err := limiter.Acquire(ctx, "queue:alpha", 2, time.Minute)
	require.NoError(t,
		err)
	require.False(t, !allowed ||
		tok1 == "")

	tok2, allowed, err := limiter.Acquire(ctx, "queue:alpha", 2, time.Minute)
	require.NoError(t,
		err)
	require.False(t, !allowed ||
		tok2 == "")

	_, allowed, err = limiter.Acquire(ctx, "queue:alpha", 2, time.Minute)
	require.NoError(t, err)
	require.False(t, allowed)
	require.NoError(t,
		limiter.Release(ctx,
			"queue:alpha",
			tok1,
		))

	_, allowed, err = limiter.Acquire(ctx, "queue:alpha", 2, time.Minute)
	require.NoError(t, err)
	require.True(t, allowed)
}

func TestRedisConcurrencyLimiterAcquire_RedisErrorFailsOpen(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		return errors.New("redis unavailable")
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	token, allowed, err := limiter.Acquire(ctx, "job", 1, time.Minute)
	require.NoError(t,
		err)
	require.True(t, allowed)
	require.Equal(t, "",
		token)

}

func TestRedisConcurrencyLimiterRelease_InvalidToken(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.Error(t, limiter.
		Release(ctx, "job",
			"invalid",
		))

}

func TestParseRedisConcurrencyToken(t *testing.T) {
	t.Parallel()

	slot, id, err := parseRedisConcurrencyToken("3:abc")
	require.NoError(t,
		err)
	require.False(t, slot !=
		3 ||
		id != "abc",
	)

	if _, _, err := parseRedisConcurrencyToken("x:abc"); err == nil {
		require.Fail(t, "expected parse error for non-numeric slot")
	}
}

func TestRedisConcurrencyLimiterAcquire_ZeroConcurrency_Error(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called for zero concurrency")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, _, err := limiter.Acquire(ctx, "key", 0, time.Minute)
	require.Error(t, err)

}

func TestRedisConcurrencyLimiterAcquire_NegativeConcurrency_Error(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called for negative concurrency")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, _, err := limiter.Acquire(ctx, "key", -1, time.Minute)
	require.Error(t, err)

}

func TestRedisConcurrencyLimiterAcquire_ZeroTTL_Error(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called for zero TTL")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, _, err := limiter.Acquire(ctx, "key", 1, 0)
	require.Error(t, err)

}

func TestRedisConcurrencyLimiterAcquire_NegativeTTL_Error(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called for negative TTL")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, _, err := limiter.Acquire(ctx, "key", 1, -time.Second)
	require.Error(t, err)

}

func TestRedisConcurrencyLimiterRelease_EmptyToken(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.Error(t, limiter.
		Release(ctx, "job",
			"0:"))
	require.Error(t, limiter.
		Release(ctx, "job",
			":abc",
		))
	require.Error(t, limiter.
		Release(ctx, "job",
			""))

}

func TestRedisConcurrencyLimiterRelease_RedisErrorFailsOpen(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		return errors.New("redis unavailable")
	})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewRedisConcurrencyLimiter(client, true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err := limiter.Release(ctx, "job", "0:some-id")
	require.NoError(t,
		err)

}

func TestRedisConcurrencySlotKey(t *testing.T) {
	t.Parallel()

	key := redisConcurrencySlotKey("group", 7)
	parts := strings.Split(key, ":")
	require.Len(t, parts,
		3)
	require.False(t, parts[0] !=
		"concurrency" ||
		parts[1] != "group",
	)

	_, err := strconv.Atoi(parts[2])
	require.NoError(t, err)
}
