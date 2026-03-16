package composition

import (
	"context"
	"math"
	"math/rand/v2"
	"time"
)

// JitterStrategy controls how retry delays are randomized.
type JitterStrategy string

const (
	JitterFull JitterStrategy = "full"
	JitterNone JitterStrategy = "none"
)

// RetryOptions configures retry behavior.
type RetryOptions struct {
	// Attempts is the total number of attempts including the first call.
	Attempts int
	// DelayMs is the initial retry delay in milliseconds.
	DelayMs int
	// Factor is the exponential backoff multiplier.
	Factor float64
	// MaxDelayMs is the upper bound for backoff delay.
	MaxDelayMs int
	// Jitter controls delay randomization. Default is "full".
	Jitter JitterStrategy
	// ShouldRetry decides whether a failure should be retried.
	ShouldRetry func(err error, attempt int, maxAttempts int) bool
}

func (o *RetryOptions) defaults() {
	if o.Attempts <= 0 {
		o.Attempts = 3
	}
	if o.DelayMs <= 0 {
		o.DelayMs = 250
	}
	if o.Factor <= 0 {
		o.Factor = 2
	}
	if o.MaxDelayMs <= 0 {
		o.MaxDelayMs = 30000
	}
	if o.Jitter == "" {
		o.Jitter = JitterFull
	}
}

func computeDelay(baseDelay int, jitter JitterStrategy) int {
	if jitter == JitterFull {
		return int(math.Round(rand.Float64() * float64(baseDelay)))
	}
	return baseDelay
}

// WithRetry executes an operation with exponential backoff retries.
// It respects context cancellation.
func WithRetry[T any](ctx context.Context, fn func() (T, error), opts *RetryOptions) (T, error) {
	var o RetryOptions
	if opts != nil {
		o = *opts
	}
	o.defaults()

	var zero T
	delayMs := o.DelayMs

	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		if attempt >= o.Attempts {
			return zero, err
		}

		if o.ShouldRetry != nil && !o.ShouldRetry(err, attempt, o.Attempts) {
			return zero, err
		}

		waitMs := computeDelay(delayMs, o.Jitter)
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(time.Duration(waitMs) * time.Millisecond):
		}

		delayMs = int(math.Min(float64(o.MaxDelayMs), math.Max(1, math.Round(float64(delayMs)*o.Factor))))
	}
}
