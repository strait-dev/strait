package worker

import (
	"math/rand/v2"
	"time"
)

// Retry strategies.
const (
	RetryExponential = "exponential" // default: base * 2^(attempt-1) with jitter
	RetryLinear      = "linear"      // base * attempt with jitter
	RetryFixed       = "fixed"       // constant delay with jitter
	RetryCustom      = "custom"      // user-supplied per-attempt delays
)

// NextRetryDelay computes the delay before the next retry attempt.
// Uses exponential backoff with +-20% jitter to prevent thundering herd.
//
// Formula: min(base * 2^(attempt-1), maxDelay) +- 20% jitter
//
// attempt 1 -> ~1s
// attempt 2 -> ~2s
// attempt 3 -> ~4s
// attempt 4 -> ~8s
// ...capped at maxDelay (default 1 hour).
func NextRetryDelay(attempt int) time.Duration {
	return NextRetryDelayWithStrategy(attempt, "", nil)
}

// NextRetryDelayWithStrategy computes the delay using the specified strategy.
// Supported strategies: "exponential" (default), "linear", "fixed", "custom".
// For "custom", customDelays provides per-attempt delays in seconds.
func NextRetryDelayWithStrategy(attempt int, strategy string, customDelays []int) time.Duration {
	const (
		base     = 1 * time.Second
		maxDelay = 1 * time.Hour
	)

	if attempt < 1 {
		attempt = 1
	}

	var delay time.Duration

	switch strategy {
	case RetryLinear:
		// Linear: base * attempt, capped at maxDelay.
		delay = min(base*time.Duration(attempt), maxDelay)

	case RetryFixed:
		// Fixed: constant base delay.
		delay = base

	case RetryCustom:
		if len(customDelays) > 0 {
			idx := attempt - 1
			if idx >= len(customDelays) {
				idx = len(customDelays) - 1
			}
			delay = min(time.Duration(customDelays[idx])*time.Second, maxDelay)
		} else {
			// Fallback to exponential if no custom delays provided.
			delay = exponentialDelay(attempt, base, maxDelay)
		}

	default:
		// Exponential (default).
		delay = exponentialDelay(attempt, base, maxDelay)
	}

	return addJitter(delay)
}

// exponentialDelay computes base * 2^(attempt-1), capped at maxDelay.
func exponentialDelay(attempt int, base, maxDelay time.Duration) time.Duration {
	delay := base
	for i := 1; i < attempt; i++ {
		if delay >= maxDelay/2 {
			delay = maxDelay
			break
		}
		delay *= 2
	}
	return delay
}

// addJitter applies +-20% jitter to a delay.
func addJitter(delay time.Duration) time.Duration {
	jitterRange := float64(delay) * 0.2
	jitter := time.Duration(rand.Float64()*2*jitterRange - jitterRange) //nolint:gosec // jitter doesn't need crypto rand
	return delay + jitter
}

// NextRetryAt returns the absolute time for the next retry.
func NextRetryAt(attempt int) time.Time {
	return time.Now().Add(NextRetryDelay(attempt))
}

// NextRetryAtWithStrategy returns the absolute time for the next retry using
// the specified strategy.
func NextRetryAtWithStrategy(attempt int, strategy string, customDelays []int) time.Time {
	return time.Now().Add(NextRetryDelayWithStrategy(attempt, strategy, customDelays))
}
