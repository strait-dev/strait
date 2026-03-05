package worker

import (
	"math/rand/v2"
	"time"
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
	const (
		base     = 1 * time.Second
		maxDelay = 1 * time.Hour
	)

	if attempt < 1 {
		attempt = 1
	}

	delay := base
	for i := 1; i < attempt; i++ {
		if delay >= maxDelay/2 {
			delay = maxDelay
			break
		}
		delay *= 2
	}

	// Add +-20% jitter.
	jitterRange := float64(delay) * 0.2
	jitter := time.Duration(rand.Float64()*2*jitterRange - jitterRange) //nolint:gosec // jitter doesn't need crypto rand

	return delay + jitter
}

// NextRetryAt returns the absolute time for the next retry.
func NextRetryAt(attempt int) time.Time {
	return time.Now().Add(NextRetryDelay(attempt))
}
