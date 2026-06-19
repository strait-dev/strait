package queue

import (
	"context"
	"errors"
	"math"
	randv2 "math/rand/v2"
	"time"

	"strait/internal/domain"
)

const defaultInternalEnqueueRetryJitter = 0.25

// SingleEnqueuer is the minimal enqueue surface used by internal retry helpers.
type SingleEnqueuer interface {
	Enqueue(ctx context.Context, run *domain.JobRun) error
}

// EnqueueRetryConfig controls bounded retry for internal enqueue producers.
type EnqueueRetryConfig struct {
	MaxElapsed time.Duration
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	JitterFrac float64
	sleep      func(ctx context.Context, delay time.Duration) error
	randFloat  func() float64
}

// DefaultInternalEnqueueRetryConfig returns the bounded retry policy used by
// scheduler, workflow, and worker-generated enqueues.
func DefaultInternalEnqueueRetryConfig() EnqueueRetryConfig {
	return EnqueueRetryConfig{
		MaxElapsed: defaultInternalEnqueueRetryBudget(),
		BaseDelay:  defaultInternalEnqueueRetryBase(),
		MaxDelay:   defaultInternalEnqueueRetryMax(),
		JitterFrac: defaultInternalEnqueueRetryJitter,
	}
}

// EnqueueWithRetry retries only backpressure-throttled enqueue attempts. All
// other errors are returned immediately.
func EnqueueWithRetry(ctx context.Context, q SingleEnqueuer, run *domain.JobRun, cfg EnqueueRetryConfig) error {
	if q == nil {
		return nil
	}
	if cfg.MaxElapsed <= 0 {
		cfg.MaxElapsed = defaultInternalEnqueueRetryBudget()
	}
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = defaultInternalEnqueueRetryBase()
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = defaultInternalEnqueueRetryMax()
	}
	if cfg.JitterFrac < 0 {
		cfg.JitterFrac = 0
	}
	if cfg.sleep == nil {
		cfg.sleep = sleepWithContext
	}
	if cfg.randFloat == nil {
		cfg.randFloat = randv2.Float64
	}

	deadline := time.Now().Add(cfg.MaxElapsed)
	attempt := 0
	for {
		err := q.Enqueue(ctx, run)
		if err == nil || !errors.Is(err, ErrEnqueueThrottled) {
			return err
		}

		delay := backpressureRetryDelay(err, attempt, cfg)
		if time.Now().Add(delay).After(deadline) {
			return err
		}
		if err := cfg.sleep(ctx, delay); err != nil {
			return err
		}
		attempt++
	}
}

func defaultInternalEnqueueRetryBudget() time.Duration {
	return 1500 * time.Millisecond
}

func defaultInternalEnqueueRetryBase() time.Duration {
	return 50 * time.Millisecond
}

func defaultInternalEnqueueRetryMax() time.Duration {
	return 250 * time.Millisecond
}

func backpressureRetryDelay(err error, attempt int, cfg EnqueueRetryConfig) time.Duration {
	delay := cfg.BaseDelay
	if attempt > 0 {
		delay *= time.Duration(1 << minInt(attempt, 8))
	}
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}
	if throttled, ok := AsThrottled(err); ok && throttled.RetryAfter > delay {
		delay = throttled.RetryAfter
	}
	if cfg.JitterFrac <= 0 || delay <= 0 {
		return delay
	}

	jitterRange := float64(delay) * cfg.JitterFrac
	offset := (cfg.randFloat()*2 - 1) * jitterRange
	delay += time.Duration(math.Round(offset))
	if delay < 0 {
		return 0
	}
	return delay
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
