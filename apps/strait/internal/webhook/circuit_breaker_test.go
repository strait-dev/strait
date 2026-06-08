package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestHashURL_UsesSHA256Hex(t *testing.T) {
	t.Parallel()

	raw := "org-1\x00https://hooks.example.com/webhook"
	wantSum := sha256.Sum256([]byte(raw))
	want := hex.EncodeToString(wantSum[:])

	got := hashURL(raw)
	require.Equal(t,
		want, got)
	require.Len(t, got,
		64)
}

func TestCircuitBreakerKeysUseHashedURL(t *testing.T) {
	t.Parallel()

	raw := "org-1\x00https://hooks.example.com/webhook"
	cb := &RedisWebhookCircuitBreaker{}
	hash := hashURL(raw)

	require.Equal(t, webhookCircuitFailuresPrefix+hash, cb.failureKey(raw))
	require.Equal(t, webhookCircuitOpenPrefix+hash, cb.openKey(raw))
}

func BenchmarkHashURL(b *testing.B) {
	raw := "org-1\x00https://hooks.example.com/webhook"

	b.ReportAllocs()
	for b.Loop() {
		if hashURL(raw) == "" {
			b.Fatal("hashURL returned empty hash")
		}
	}
}

func BenchmarkCircuitBreakerKeys(b *testing.B) {
	cb := &RedisWebhookCircuitBreaker{}
	raw := "org-1\x00https://hooks.example.com/webhook"

	b.Run("failure", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if cb.failureKey(raw) == "" {
				b.Fatal("failureKey returned empty key")
			}
		}
	})

	b.Run("open", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if cb.openKey(raw) == "" {
				b.Fatal("openKey returned empty key")
			}
		}
	})
}

type redisProcessFunc func(ctx context.Context, cmd redis.Cmder) error

type redisMockHook struct {
	process redisProcessFunc
}

func (h redisMockHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h redisMockHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if h.process != nil {
			return h.process(ctx, cmd)
		}
		return next(ctx, cmd)
	}
}

func (h redisMockHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if h.process != nil {
			for _, cmd := range cmds {
				if err := h.process(ctx, cmd); err != nil {
					return err
				}
			}
			return nil
		}
		return next(ctx, cmds)
	}
}

func newMockRedisClient(process redisProcessFunc) *redis.Client {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	client.AddHook(redisMockHook{process: process})
	return client
}

type redisCircuitState struct {
	mu       sync.Mutex
	failures map[string][]int64
	open     map[string]bool
}

func newRedisCircuitState() *redisCircuitState {
	return &redisCircuitState{
		failures: make(map[string][]int64),
		open:     make(map[string]bool),
	}
}

func (s *redisCircuitState) process(_ context.Context, cmd redis.Cmder) error {
	args := cmd.Args()
	if len(args) == 0 {
		return fmt.Errorf("missing redis command")
	}

	name := strings.ToLower(fmt.Sprint(args[0]))
	s.mu.Lock()
	defer s.mu.Unlock()

	switch name {
	case "exists":
		key := fmt.Sprint(args[1])
		val := int64(0)
		if s.open[key] {
			val = 1
		}
		if c, ok := cmd.(*redis.IntCmd); ok {
			c.SetVal(val)
			return nil
		}
		return fmt.Errorf("exists command type")
	case "zremrangebyscore":
		key := fmt.Sprint(args[1])
		maxScore, err := strconv.ParseInt(fmt.Sprint(args[3]), 10, 64)
		if err != nil {
			return err
		}
		current := s.failures[key]
		remaining := current[:0]
		removed := int64(0)
		for _, score := range current {
			if score <= maxScore {
				removed++
				continue
			}
			remaining = append(remaining, score)
		}
		s.failures[key] = remaining
		if c, ok := cmd.(*redis.IntCmd); ok {
			c.SetVal(removed)
			return nil
		}
		return fmt.Errorf("zremrangebyscore command type")
	case "zcard":
		key := fmt.Sprint(args[1])
		if c, ok := cmd.(*redis.IntCmd); ok {
			c.SetVal(int64(len(s.failures[key])))
			return nil
		}
		return fmt.Errorf("zcard command type")
	case "set":
		key := fmt.Sprint(args[1])
		s.open[key] = true
		if c, ok := cmd.(*redis.StatusCmd); ok {
			c.SetVal("OK")
			return nil
		}
		return fmt.Errorf("set command type")
	case "zadd":
		key := fmt.Sprint(args[1])
		scoreFloat, err := strconv.ParseFloat(fmt.Sprint(args[2]), 64)
		if err != nil {
			return err
		}
		score := int64(scoreFloat)
		s.failures[key] = append(s.failures[key], score)
		if c, ok := cmd.(*redis.IntCmd); ok {
			c.SetVal(1)
			return nil
		}
		return fmt.Errorf("zadd command type")
	case "expire":
		if c, ok := cmd.(*redis.BoolCmd); ok {
			c.SetVal(true)
			return nil
		}
		return fmt.Errorf("expire command type")
	case "del":
		for _, raw := range args[1:] {
			key := fmt.Sprint(raw)
			delete(s.open, key)
			delete(s.failures, key)
		}
		if c, ok := cmd.(*redis.IntCmd); ok {
			c.SetVal(1)
			return nil
		}
		return fmt.Errorf("del command type")
	default:
		return fmt.Errorf("unsupported command: %s", name)
	}
}

func TestRedisWebhookCircuitBreaker_DisabledAllowsDelivery(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called when breaker disabled")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, false)
	canDeliver, err := breaker.CanDeliver(t.Context(), "https://example.com/webhook")
	require.NoError(t,
		err)
	require.True(t, canDeliver)
}

func TestRedisWebhookCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	t.Parallel()

	state := newRedisCircuitState()
	client := newMockRedisClient(state.process)
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(
		client,
		true,
		WithWebhookCircuitBreakerThreshold(2),
		WithWebhookCircuitBreakerWindow(time.Minute),
	)

	url := "https://example.com/webhook"
	breaker.RecordFailure(t.Context(), url)
	breaker.RecordFailure(t.Context(), url)

	canDeliver, err := breaker.CanDeliver(t.Context(), url)
	require.NoError(t,
		err)
	require.False(t,
		canDeliver)
}

func TestRedisWebhookCircuitBreaker_RecordSuccessResetsOpenCircuit(t *testing.T) {
	t.Parallel()

	state := newRedisCircuitState()
	client := newMockRedisClient(state.process)
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, true, WithWebhookCircuitBreakerThreshold(1))
	url := "https://example.com/webhook"

	breaker.RecordFailure(t.Context(), url)
	canDeliver, err := breaker.CanDeliver(t.Context(), url)
	require.NoError(t,
		err)
	require.False(t,
		canDeliver)

	breaker.RecordSuccess(t.Context(), url)
	canDeliver, err = breaker.CanDeliver(t.Context(), url)
	require.NoError(t,
		err)
	require.True(t, canDeliver)
}

func TestRedisWebhookCircuitBreaker_CanDeliver_NilClient(t *testing.T) {
	t.Parallel()

	breaker := NewRedisWebhookCircuitBreaker(nil, true)
	canDeliver, err := breaker.CanDeliver(t.Context(), "https://example.com/webhook")
	require.NoError(t,
		err)
	require.True(t, canDeliver)
}

func TestRedisWebhookCircuitBreaker_CanDeliver_EmptyURL(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called for empty URL")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, true)
	canDeliver, err := breaker.CanDeliver(t.Context(), "")
	require.NoError(t,
		err)
	require.True(t, canDeliver)
}

func TestRedisWebhookCircuitBreaker_CanDeliver_OpenCircuit(t *testing.T) {
	t.Parallel()

	state := newRedisCircuitState()
	client := newMockRedisClient(state.process)
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, true, WithWebhookCircuitBreakerThreshold(2))
	url := "https://example.com/open-test"

	// Push failures to open the circuit.
	breaker.RecordFailure(t.Context(), url)
	breaker.RecordFailure(t.Context(), url)

	// Circuit should be open now -- CanDeliver returns false.
	canDeliver, err := breaker.CanDeliver(t.Context(), url)
	require.NoError(t,
		err)
	require.False(t,
		canDeliver)

	// Second call should also return false (the open key still exists).
	canDeliver, err = breaker.CanDeliver(t.Context(), url)
	require.NoError(t,
		err)
	require.False(t,
		canDeliver)
}

func TestRedisWebhookCircuitBreaker_CanDeliver_ClosedCircuit(t *testing.T) {
	t.Parallel()

	state := newRedisCircuitState()
	client := newMockRedisClient(state.process)
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, true, WithWebhookCircuitBreakerThreshold(5))
	url := "https://example.com/closed-test"

	// Record fewer failures than threshold.
	breaker.RecordFailure(t.Context(), url)
	breaker.RecordFailure(t.Context(), url)

	canDeliver, err := breaker.CanDeliver(t.Context(), url)
	require.NoError(t,
		err)
	require.True(t, canDeliver)
}

func TestRedisWebhookCircuitBreaker_CanDeliver_FailureWindowExpiry(t *testing.T) {
	t.Parallel()

	state := newRedisCircuitState()
	client := newMockRedisClient(state.process)
	t.Cleanup(func() { _ = client.Close() })

	now := time.Now()
	breaker := NewRedisWebhookCircuitBreaker(
		client,
		true,
		WithWebhookCircuitBreakerThreshold(3),
		WithWebhookCircuitBreakerWindow(time.Minute),
	)
	// Override the time function to control failure window.
	breaker.now = func() time.Time { return now }

	url := "https://example.com/window-test"

	// Record 2 failures at the current time (below threshold of 3).
	breaker.RecordFailure(t.Context(), url)
	breaker.RecordFailure(t.Context(), url)

	// With 2 failures below threshold, delivery is allowed.
	canDeliver, err := breaker.CanDeliver(t.Context(), url)
	require.NoError(t,
		err)
	require.True(t, canDeliver)

	// Move time past the failure window so old failures are pruned by CanDeliver.
	breaker.now = func() time.Time { return now.Add(2 * time.Minute) }

	// CanDeliver should prune the old failures and see 0 in-window failures.
	canDeliver, err = breaker.CanDeliver(t.Context(), url)
	require.NoError(t,
		err)
	require.True(t, canDeliver)
}

func TestRedisWebhookCircuitBreaker_RecordSuccess_DisabledNoOp(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called when breaker disabled")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, false)
	breaker.RecordSuccess(t.Context(), "https://example.com/webhook")
}

func TestRedisWebhookCircuitBreaker_RecordSuccess_NilClient(t *testing.T) {
	t.Parallel()

	breaker := NewRedisWebhookCircuitBreaker(nil, true)
	// Should not panic.
	breaker.RecordSuccess(t.Context(), "https://example.com/webhook")
}

func TestRedisWebhookCircuitBreaker_RecordSuccess_EmptyURL(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called for empty URL")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, true)
	breaker.RecordSuccess(t.Context(), "")
}

func TestRedisWebhookCircuitBreaker_RecordFailure_DisabledNoOp(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called when breaker disabled")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, false)
	breaker.RecordFailure(t.Context(), "https://example.com/webhook")
}

func TestRedisWebhookCircuitBreaker_RecordFailure_NilClient(t *testing.T) {
	t.Parallel()

	breaker := NewRedisWebhookCircuitBreaker(nil, true)
	// Should not panic.
	breaker.RecordFailure(t.Context(), "https://example.com/webhook")
}

func TestRedisWebhookCircuitBreaker_RecordFailure_EmptyURL(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(func(context.Context, redis.Cmder) error {
		require.Fail(t, "redis should not be called for empty URL")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, true)
	breaker.RecordFailure(t.Context(), "")
}

func TestWithWebhookCircuitBreakerOpenDuration(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(nil)
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, true,
		WithWebhookCircuitBreakerOpenDuration(5*time.Minute),
	)
	require.Equal(t,
		5*time.Minute,
		breaker.openDuration,
	)
}

func TestWithWebhookCircuitBreakerOpenDuration_ZeroIgnored(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(nil)
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, true,
		WithWebhookCircuitBreakerOpenDuration(0),
	)
	require.Equal(t,
		defaultWebhookOpenDuration,
		breaker.
			openDuration,
	)
}

func TestWithWebhookCircuitBreakerOpenDuration_NegativeIgnored(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(nil)
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, true,
		WithWebhookCircuitBreakerOpenDuration(-time.Second),
	)
	require.Equal(t,
		defaultWebhookOpenDuration,
		breaker.
			openDuration,
	)
}

func TestWithWebhookCircuitBreakerThreshold_ZeroIgnored(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(nil)
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, true,
		WithWebhookCircuitBreakerThreshold(0),
	)
	require.Equal(t,
		defaultWebhookFailureThreshold,
		breaker.
			failureThreshold,
	)
}

func TestWithWebhookCircuitBreakerWindow_ZeroIgnored(t *testing.T) {
	t.Parallel()

	client := newMockRedisClient(nil)
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, true,
		WithWebhookCircuitBreakerWindow(0),
	)
	require.Equal(t,
		defaultWebhookFailureWindow,
		breaker.
			failureWindow,
	)
}

func TestRedisWebhookCircuitBreaker_RecordSuccessAfterMultipleFailures(t *testing.T) {
	t.Parallel()

	state := newRedisCircuitState()
	client := newMockRedisClient(state.process)
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, true, WithWebhookCircuitBreakerThreshold(3))
	url := "https://example.com/recovery"

	// Record failures to open the circuit.
	breaker.RecordFailure(t.Context(), url)
	breaker.RecordFailure(t.Context(), url)
	breaker.RecordFailure(t.Context(), url)

	canDeliver, err := breaker.CanDeliver(t.Context(), url)
	require.NoError(t,
		err)
	require.False(t,
		canDeliver)

	// A success resets the circuit.
	breaker.RecordSuccess(t.Context(), url)

	canDeliver, err = breaker.CanDeliver(t.Context(), url)
	require.NoError(t,
		err)
	require.True(t, canDeliver)
}

func TestRedisWebhookCircuitBreaker_CanDeliver_ExactThresholdOpensCircuit(t *testing.T) {
	t.Parallel()

	state := newRedisCircuitState()
	client := newMockRedisClient(state.process)
	t.Cleanup(func() { _ = client.Close() })

	threshold := 3
	breaker := NewRedisWebhookCircuitBreaker(
		client, true,
		WithWebhookCircuitBreakerThreshold(threshold),
		WithWebhookCircuitBreakerWindow(time.Minute),
	)

	url := "https://example.com/candeliver-threshold"
	failureKey := breaker.failureKey(url)

	now := time.Now()
	breaker.now = func() time.Time { return now }

	// Inject failures directly into the sorted set (bypass RecordFailure
	// so RecordFailure doesn't set the open key itself).
	state.mu.Lock()
	for i := range threshold {
		state.failures[failureKey] = append(state.failures[failureKey], now.Add(-time.Duration(i)*time.Second).UnixMilli())
	}
	state.mu.Unlock()

	// CanDeliver must detect failures >= threshold and open the circuit.
	canDeliver, err := breaker.CanDeliver(t.Context(), url)
	require.NoError(t,
		err)
	require.False(t,
		canDeliver)

	// Verify the open key was set by CanDeliver (not by RecordFailure).
	state.mu.Lock()
	openKey := breaker.openKey(url)
	isOpen := state.open[openKey]
	state.mu.Unlock()
	require.True(t, isOpen)
}

func TestRedisWebhookCircuitBreaker_CanDeliver_BelowThreshold_StaysClosed(t *testing.T) {
	t.Parallel()

	state := newRedisCircuitState()
	client := newMockRedisClient(state.process)
	t.Cleanup(func() { _ = client.Close() })

	threshold := 3
	breaker := NewRedisWebhookCircuitBreaker(
		client, true,
		WithWebhookCircuitBreakerThreshold(threshold),
		WithWebhookCircuitBreakerWindow(time.Minute),
	)

	url := "https://example.com/below-threshold"
	failureKey := breaker.failureKey(url)
	now := time.Now()
	breaker.now = func() time.Time { return now }

	// Inject threshold-1 failures directly.
	state.mu.Lock()
	for i := range threshold - 1 {
		state.failures[failureKey] = append(state.failures[failureKey], now.Add(-time.Duration(i)*time.Second).UnixMilli())
	}
	state.mu.Unlock()

	canDeliver, err := breaker.CanDeliver(t.Context(), url)
	require.NoError(t,
		err)
	require.True(t, canDeliver)

	// Verify open key was NOT set.
	state.mu.Lock()
	isOpen := state.open[breaker.openKey(url)]
	state.mu.Unlock()
	require.False(t,
		isOpen)
}

func TestRedisWebhookCircuitBreaker_FailureWindowBoundary(t *testing.T) {
	t.Parallel()

	state := newRedisCircuitState()
	client := newMockRedisClient(state.process)
	t.Cleanup(func() { _ = client.Close() })

	window := time.Minute
	threshold := 2
	breaker := NewRedisWebhookCircuitBreaker(
		client, true,
		WithWebhookCircuitBreakerThreshold(threshold),
		WithWebhookCircuitBreakerWindow(window),
	)

	url := "https://example.com/window-boundary"
	failureKey := breaker.failureKey(url)

	now := time.Now()
	breaker.now = func() time.Time { return now }

	// Inject a failure at exactly now - window (should be pruned by ZRemRangeByScore
	// because cutoff = now - window, and score <= cutoff is removed).
	staleScore := now.Add(-window).UnixMilli()
	// Inject a failure 1ms inside the window (should survive pruning).
	freshScore := now.Add(-window + time.Millisecond).UnixMilli()

	state.mu.Lock()
	state.failures[failureKey] = []int64{staleScore, freshScore}
	state.mu.Unlock()

	canDeliver, err := breaker.CanDeliver(t.Context(), url)
	require.NoError(t,
		err)
	require.True(t, canDeliver)

	// Verify the stale failure was pruned and only the fresh one remains.
	state.mu.Lock()
	remaining := len(state.failures[failureKey])
	state.mu.Unlock()
	require.Equal(t, 1, remaining)
}

func TestRedisWebhookCircuitBreaker_RecordFailure_IntermediateState(t *testing.T) {
	t.Parallel()

	state := newRedisCircuitState()
	client := newMockRedisClient(state.process)
	t.Cleanup(func() { _ = client.Close() })

	threshold := 3
	breaker := NewRedisWebhookCircuitBreaker(
		client, true,
		WithWebhookCircuitBreakerThreshold(threshold),
		WithWebhookCircuitBreakerWindow(time.Minute),
	)

	url := "https://example.com/intermediate"
	openKey := breaker.openKey(url)

	// Record threshold-1 failures.
	for range threshold - 1 {
		breaker.RecordFailure(t.Context(), url)
	}

	// Open key should NOT be set yet.
	state.mu.Lock()
	isOpen := state.open[openKey]
	state.mu.Unlock()
	require.False(t,
		isOpen)

	// Record the threshold-th failure.
	breaker.RecordFailure(t.Context(), url)

	// Open key should now be set.
	state.mu.Lock()
	isOpen = state.open[openKey]
	state.mu.Unlock()
	require.True(t, isOpen)
}

func TestRedisWebhookCircuitBreaker_RecordFailureCountsSameTimestampFailures(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	now := time.Now()
	breaker := NewRedisWebhookCircuitBreaker(
		client,
		true,
		WithWebhookCircuitBreakerThreshold(2),
		WithWebhookCircuitBreakerWindow(time.Minute),
	)
	breaker.now = func() time.Time { return now }

	url := "https://example.com/same-timestamp"
	breaker.RecordFailure(t.Context(), url)
	breaker.RecordFailure(t.Context(), url)

	failures, err := client.ZCard(t.Context(), breaker.failureKey(url)).Result()
	require.NoError(t,
		err)
	require.EqualValues(t, 2, failures)

	canDeliver, err := breaker.CanDeliver(t.Context(), url)
	require.NoError(t,
		err)
	require.False(t,
		canDeliver)
}

func TestRedisWebhookCircuitBreaker_DifferentURLsIndependent(t *testing.T) {
	t.Parallel()

	state := newRedisCircuitState()
	client := newMockRedisClient(state.process)
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, true, WithWebhookCircuitBreakerThreshold(1))

	urlA := "https://example.com/a"
	urlB := "https://example.com/b"

	// Trip circuit for URL A only.
	breaker.RecordFailure(t.Context(), urlA)

	canA, err := breaker.CanDeliver(t.Context(), urlA)
	require.NoError(t,
		err)
	require.False(t,
		canA)

	canB, err := breaker.CanDeliver(t.Context(), urlB)
	require.NoError(t,
		err)
	require.True(t, canB)
}
