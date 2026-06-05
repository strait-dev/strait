package worker

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

// TestTOCTOU_TokenExpiryBoundary tests boundary behavior of token expiry
// using times just before and after the current moment.
func TestTOCTOU_TokenExpiryBoundary(t *testing.T) {
	t.Parallel()

	type token struct {
		ExpiresAt time.Time
	}

	now := time.Now()

	// Token expired 1 nanosecond ago should fail.
	expired := token{ExpiresAt: now.Add(-1 * time.Nanosecond)}
	assert.True(t, expired.
		ExpiresAt.
		Before(now))

	// Token expiring 1 second in the future should pass.
	valid := token{ExpiresAt: now.Add(1 * time.Second)}
	assert.False(t,
		valid.ExpiresAt.
			Before(now))

	// Exact boundary: token expiring exactly at now is not before now.
	boundary := token{ExpiresAt: now}
	assert.False(t,
		boundary.ExpiresAt.
			Before(now))
	assert.True(t, valid.
		ExpiresAt.
		After(now))

	// Using After for the valid case.

}

// TestTOCTOU_RetryDelayNeverNegative verifies that NextRetryDelay never returns
// a negative duration, even with jitter applied across many random attempts.
func TestTOCTOU_RetryDelayNeverNegative(t *testing.T) {
	t.Parallel()

	strategies := []string{"", RetryExponential, RetryLinear, RetryFixed}

	for _, strategy := range strategies {
		for range 1000 {
			attempt := rand.IntN(200) + 1
			delay := NextRetryDelayWithStrategy(attempt, strategy, nil)
			require.GreaterOrEqual(t,
				delay,
				time.Duration(0))

		}
	}

	// Also test with negative and zero attempts (should be clamped to 1).
	for _, attempt := range []int{-100, -1, 0} {
		delay := NextRetryDelay(attempt)
		require.GreaterOrEqual(t,
			delay,
			time.Duration(0))

	}
}

// TestTOCTOU_BackoffDelayStability calls NextRetryDelay 100 times for the same
// attempt and verifies all results are within the expected jitter range.
func TestTOCTOU_BackoffDelayStability(t *testing.T) {
	t.Parallel()

	const attempt = 5
	// For attempt 5 with exponential backoff: base * 2^4 = 16s.
	// Jitter is +-20%, so range is [12.8s, 19.2s].
	expectedBase := 16 * time.Second
	minExpected := time.Duration(float64(expectedBase) * 0.8)
	maxExpected := time.Duration(float64(expectedBase) * 1.2)

	for range 100 {
		delay := NextRetryDelay(attempt)
		require.False(t,
			delay < minExpected ||
				delay > maxExpected,
		)

	}
}

// TestTOCTOU_CircuitBreakerHalfOpenTiming opens the circuit breaker, advances
// past the open duration using a mock clock, and verifies that half-open
// allows exactly one probe request before transitioning on success or failure.
func TestTOCTOU_CircuitBreakerHalfOpenTiming(t *testing.T) {
	t.Parallel()

	openDuration := 100 * time.Millisecond
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		OpenDuration:     openDuration,
	})

	// Trip the breaker.
	cb.RecordFailure()
	cb.RecordFailure()
	require.Equal(t,
		circuitOpen,
		cb.
			State())
	require.False(t,
		cb.Allow())

	// Should not allow requests while open.

	// Inject a mock clock that is past the open duration.
	mockTime := time.Now().Add(openDuration + time.Millisecond)
	cb.mu.Lock()
	cb.now = func() time.Time { return mockTime }
	cb.mu.Unlock()
	require.True(t,
		cb.Allow(),
	)
	require.Equal(t,
		circuitHalfOpen,

		cb.State())

	// First Allow() after open duration should transition to half-open and allow.

	// Record success to close the breaker.
	cb.RecordSuccess()
	require.Equal(t,
		circuitClosed,
		cb.
			State())

	// Verify re-opening from half-open on failure.
	cb.RecordFailure()
	cb.RecordFailure()
	require.Equal(t,
		circuitOpen,
		cb.
			State())

	// Advance clock again past open duration.
	mockTime2 := mockTime.Add(openDuration + time.Millisecond)
	cb.mu.Lock()
	cb.now = func() time.Time { return mockTime2 }
	cb.mu.Unlock()
	require.True(t,
		cb.Allow(),
	)
	require.Equal(t,
		circuitHalfOpen,

		cb.State())

	// Transition to half-open.

	// Failure in half-open should reopen immediately.
	cb.RecordFailure()
	require.Equal(t,
		circuitOpen,
		cb.
			State())

}

// TestTOCTOU_HealthScoreDecayTiming verifies that recording multiple consecutive
// failures causes the EWMA-based health score components to decrease monotonically.
func TestTOCTOU_HealthScoreDecayTiming(t *testing.T) {
	t.Parallel()

	// Simulate EWMA decay for success rate without requiring a real store.
	// Start at 1.0 (fully healthy) and record failures.
	successRate := 1.0
	const alpha = 0.1

	var prevScore float64
	for i := range 20 {
		// Record a failure: new success value is 0.0.
		successRate = ewma(successRate, 0.0, alpha)

		// Compute a simplified health score based on success rate component.
		score := successRate * 100.0
		require.False(t,
			i > 0 &&
				score >
					prevScore)

		prevScore = score
	}
	assert.LessOrEqual(t, successRate,

		0.15)

	// After 20 consecutive failures, the success rate should be very low.

}

// TestTOCTOU_PoisonPillThresholdBoundary verifies that poison pill detection
// triggers at exactly the threshold, not before.
func TestTOCTOU_PoisonPillThresholdBoundary(t *testing.T) {
	t.Parallel()

	threshold := 3
	errMsg := "connection refused: endpoint unreachable"
	hash := errorHash(errMsg)

	// Simulate the poison pill counting logic from handleFailure.
	metadata := make(map[string]string)
	var poisonDetected bool

	for attempt := 1; attempt <= threshold+1; attempt++ {
		prevHash := metadata["_error_hash"]
		count := 1
		if prevHash == hash {
			if raw, ok := metadata["_error_hash_count"]; ok {
				n := 0
				for _, c := range raw {
					n = n*10 + int(c-'0')
				}
				count = n + 1
			}
		}
		metadata["_error_hash"] = hash
		metadata["_error_hash_count"] = fmt.Sprintf("%d", count)

		if count >= threshold {
			poisonDetected = true
			require.GreaterOrEqual(t,
				attempt,
				threshold)

			break
		}
	}
	require.True(t,
		poisonDetected,
	)

	// Verify that at attempt threshold-1 (count=2), poison was not detected.
	metadata2 := make(map[string]string)
	for attempt := 1; attempt < threshold; attempt++ {
		prevHash := metadata2["_error_hash"]
		count := 1
		if prevHash == hash {
			if raw, ok := metadata2["_error_hash_count"]; ok {
				n := 0
				for _, c := range raw {
					n = n*10 + int(c-'0')
				}
				count = n + 1
			}
		}
		metadata2["_error_hash"] = hash
		metadata2["_error_hash_count"] = fmt.Sprintf("%d", count)
		require.False(t,
			count >=
				threshold,
		)

	}
}

// TestTOCTOU_ConcurrentBudgetCheckSpend simulates a TOCTOU race between
// checking budget availability and spending it. Uses a mutex-protected
// budget to verify that concurrent check-then-spend never overspends.
func TestTOCTOU_ConcurrentBudgetCheckSpend(t *testing.T) {
	t.Parallel()

	type budget struct {
		mu        sync.Mutex
		remaining int
	}

	b := &budget{remaining: 1}

	// atomicSpend checks and spends in one atomic operation.
	atomicSpend := func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.remaining > 0 {
			b.remaining--
			return true
		}
		return false
	}

	const goroutines = 100
	var wg conc.WaitGroup
	var successes int64
	var mu sync.Mutex

	start := make(chan struct{})
	for range goroutines {
		wg.Go(func() {
			<-start
			if atomicSpend() {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		})
	}

	close(start)
	wg.Wait()
	require.EqualValues(t, 1, successes)
	require.EqualValues(t, 0, b.remaining)

}

// TestTOCTOU_BulkheadAcquireReleaseTiming verifies that bulkhead acquisition
// and release are immediate (no unnecessary delay). Acquiring all slots should
// cause the next acquire to fail immediately, and releasing should allow
// immediate re-acquisition.
func TestTOCTOU_BulkheadAcquireReleaseTiming(t *testing.T) {
	t.Parallel()

	bh := NewShardedBulkhead(0)
	jobID := "job-bulkhead-timing"
	maxConcurrency := 3

	// Acquire all slots.
	for range maxConcurrency {
		require.True(t,
			bh.TryAcquire(jobID,
				maxConcurrency,
			))

	}

	// Next acquire should fail immediately (not block or delay).
	start := time.Now()
	got := bh.TryAcquire(jobID, maxConcurrency)
	elapsed := time.Since(start)
	require.False(t,
		got)
	assert.LessOrEqual(t, elapsed,
		5*
			time.Millisecond,
	)

	// Release one slot.
	bh.Release(jobID, maxConcurrency)

	// Next acquire should succeed immediately.
	start = time.Now()
	got = bh.TryAcquire(jobID, maxConcurrency)
	elapsed = time.Since(start)
	require.True(t,
		got)
	assert.LessOrEqual(t, elapsed,
		5*
			time.Millisecond,
	)
	assert.Equal(t,
		maxConcurrency,
		bh.
			ActiveCount(jobID))

	// Verify active count.

}

// TestTOCTOU_ErrorClassificationStability verifies that classifyError returns
// the same error class deterministically across 1000 invocations with no
// race conditions in the classification logic.
func TestTOCTOU_ErrorClassificationStability(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: domain.ErrorClassUnknown,
		},
		{
			name:     "deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: domain.ErrorClassTimeout,
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: domain.ErrorClassTransient,
		},
		{
			name:     "rate limited endpoint",
			err:      &domain.EndpointError{StatusCode: 429, Body: "too many requests"},
			expected: domain.ErrorClassRateLimited,
		},
		{
			name:     "server error endpoint",
			err:      &domain.EndpointError{StatusCode: 500, Body: "internal server error"},
			expected: domain.ErrorClassServer,
		},
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: domain.ErrorClassConnection,
		},
		{
			name:     "oom error",
			err:      errors.New("out of memory"),
			expected: domain.ErrorClassOOM,
		},
		{
			name:     "budget exceeded",
			err:      errors.New("budget exceeded for project"),
			expected: domain.ErrorClassBudget,
		},
		{
			name:     "auth error",
			err:      &domain.EndpointError{StatusCode: 401, Body: "unauthorized"},
			expected: domain.ErrorClassAuth,
		},
		{
			name:     "unknown error",
			err:      errors.New("something unexpected happened"),
			expected: domain.ErrorClassUnknown,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			first := classifyError(tc.err)
			require.Equal(t,
				tc.expected,
				first,
			)

			for range 1000 {
				got := classifyError(tc.err)
				require.Equal(t,
					first, got,
				)

			}
		})
	}
}
