package webhook

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

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
		max, err := strconv.ParseInt(fmt.Sprint(args[3]), 10, 64)
		if err != nil {
			return err
		}
		current := s.failures[key]
		remaining := current[:0]
		removed := int64(0)
		for _, score := range current {
			if score <= max {
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
		t.Fatal("redis should not be called when breaker disabled")
		return nil
	})
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, false)
	canDeliver, err := breaker.CanDeliver(t.Context(), "https://example.com/webhook")
	if err != nil {
		t.Fatalf("CanDeliver error = %v", err)
	}
	if !canDeliver {
		t.Fatal("expected delivery allowed when breaker disabled")
	}
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
	if err != nil {
		t.Fatalf("CanDeliver error = %v", err)
	}
	if canDeliver {
		t.Fatal("expected delivery blocked after threshold")
	}
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
	if err != nil {
		t.Fatalf("CanDeliver error = %v", err)
	}
	if canDeliver {
		t.Fatal("expected delivery blocked before reset")
	}

	breaker.RecordSuccess(t.Context(), url)
	canDeliver, err = breaker.CanDeliver(t.Context(), url)
	if err != nil {
		t.Fatalf("CanDeliver after success error = %v", err)
	}
	if !canDeliver {
		t.Fatal("expected delivery allowed after success reset")
	}
}
