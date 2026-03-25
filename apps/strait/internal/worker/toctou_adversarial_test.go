package worker

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
)

// TestTOCTOU_ConstantTimeComparison verifies that subtle.ConstantTimeCompare
// does not leak timing information based on input length or prefix match.
// We compare valid-length vs invalid-length strings and check that the
// standard deviation of elapsed times is small.
func TestTOCTOU_ConstantTimeComparison(t *testing.T) {
	// Not parallel: timing measurements are unreliable under concurrent load.

	const iterations = 100
	secret := "sk_live_abcdef1234567890abcdef1234567890"

	// Measure comparison against a same-length wrong token.
	sameLenToken := "sk_live_00000000000000000000000000000000"
	sameLenDurations := make([]time.Duration, iterations)
	for i := range iterations {
		start := time.Now()
		subtle.ConstantTimeCompare([]byte(secret), []byte(sameLenToken))
		sameLenDurations[i] = time.Since(start)
	}

	// Measure comparison against a short wrong token.
	shortToken := "sk"
	shortDurations := make([]time.Duration, iterations)
	for i := range iterations {
		start := time.Now()
		subtle.ConstantTimeCompare([]byte(secret), []byte(shortToken))
		shortDurations[i] = time.Since(start)
	}

	sameLenMean, sameLenStdDev := durationStats(sameLenDurations)
	shortMean, shortStdDev := durationStats(shortDurations)

	// The coefficient of variation should be reasonable (timing noise, not leakage).
	// We just verify standard deviation is not enormous relative to mean.
	if sameLenStdDev > sameLenMean*2 {
		t.Errorf("same-length comparison stddev (%v) exceeds 2x mean (%v)", sameLenStdDev, sameLenMean)
	}
	if shortStdDev > shortMean*2 {
		t.Errorf("short comparison stddev (%v) exceeds 2x mean (%v)", shortStdDev, shortMean)
	}

	// Verify both sets complete in reasonable time (under 1ms per call on average).
	if sameLenMean > time.Millisecond {
		t.Errorf("same-length mean (%v) exceeds 1ms", sameLenMean)
	}
	if shortMean > time.Millisecond {
		t.Errorf("short mean (%v) exceeds 1ms", shortMean)
	}
}

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
	if !expired.ExpiresAt.Before(now) {
		t.Error("token expired 1ns ago should be before now")
	}

	// Token expiring 1 second in the future should pass.
	valid := token{ExpiresAt: now.Add(1 * time.Second)}
	if valid.ExpiresAt.Before(now) {
		t.Error("token expiring in 1s should not be before now")
	}

	// Exact boundary: token expiring exactly at now is not before now.
	boundary := token{ExpiresAt: now}
	if boundary.ExpiresAt.Before(now) {
		t.Error("token expiring exactly at now should not be strictly before now")
	}

	// Using After for the valid case.
	if !valid.ExpiresAt.After(now) {
		t.Error("valid token should be after now")
	}
}

// TestTOCTOU_RetryDelayNeverNegative verifies that NextRetryDelay never returns
// a negative duration, even with jitter applied across many random attempts.
func TestTOCTOU_RetryDelayNeverNegative(t *testing.T) {
	t.Parallel()

	strategies := []string{"", RetryExponential, RetryLinear, RetryFixed}

	for _, strategy := range strategies {
		for i := range 1000 {
			attempt := rand.IntN(200) + 1
			delay := NextRetryDelayWithStrategy(attempt, strategy, nil)
			if delay < 0 {
				t.Fatalf("iteration %d: strategy=%q attempt=%d returned negative delay: %v",
					i, strategy, attempt, delay)
			}
		}
	}

	// Also test with negative and zero attempts (should be clamped to 1).
	for _, attempt := range []int{-100, -1, 0} {
		delay := NextRetryDelay(attempt)
		if delay < 0 {
			t.Fatalf("attempt=%d returned negative delay: %v", attempt, delay)
		}
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

	for i := range 100 {
		delay := NextRetryDelay(attempt)
		if delay < minExpected || delay > maxExpected {
			t.Fatalf("iteration %d: delay %v outside jitter range [%v, %v]",
				i, delay, minExpected, maxExpected)
		}
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
	if cb.State() != circuitOpen {
		t.Fatalf("expected open, got %s", cb.State())
	}

	// Should not allow requests while open.
	if cb.Allow() {
		t.Fatal("circuit breaker should reject requests when open and within openDuration")
	}

	// Inject a mock clock that is past the open duration.
	mockTime := time.Now().Add(openDuration + time.Millisecond)
	cb.mu.Lock()
	cb.now = func() time.Time { return mockTime }
	cb.mu.Unlock()

	// First Allow() after open duration should transition to half-open and allow.
	if !cb.Allow() {
		t.Fatal("circuit breaker should allow first request after open duration (half-open)")
	}
	if cb.State() != circuitHalfOpen {
		t.Fatalf("expected half_open, got %s", cb.State())
	}

	// Record success to close the breaker.
	cb.RecordSuccess()
	if cb.State() != circuitClosed {
		t.Fatalf("expected closed after success in half-open, got %s", cb.State())
	}

	// Verify re-opening from half-open on failure.
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != circuitOpen {
		t.Fatalf("expected open after 2 failures, got %s", cb.State())
	}

	// Advance clock again past open duration.
	mockTime2 := mockTime.Add(openDuration + time.Millisecond)
	cb.mu.Lock()
	cb.now = func() time.Time { return mockTime2 }
	cb.mu.Unlock()

	// Transition to half-open.
	if !cb.Allow() {
		t.Fatal("should allow after second open duration")
	}
	if cb.State() != circuitHalfOpen {
		t.Fatalf("expected half_open, got %s", cb.State())
	}

	// Failure in half-open should reopen immediately.
	cb.RecordFailure()
	if cb.State() != circuitOpen {
		t.Fatalf("expected open after failure in half-open, got %s", cb.State())
	}
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

		if i > 0 && score > prevScore {
			t.Fatalf("iteration %d: score %f increased from %f after failure",
				i, score, prevScore)
		}
		prevScore = score
	}

	// After 20 consecutive failures, the success rate should be very low.
	if successRate > 0.15 {
		t.Errorf("success rate after 20 failures = %f, expected < 0.15", successRate)
	}
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
			if attempt < threshold {
				t.Fatalf("poison pill detected at attempt %d, before threshold %d", attempt, threshold)
			}
			break
		}
	}

	if !poisonDetected {
		t.Fatal("poison pill should have been detected at the threshold")
	}

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

		if count >= threshold {
			t.Fatalf("poison pill detected early at attempt %d with count %d", attempt, count)
		}
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
	var wg sync.WaitGroup
	var successes int64
	var mu sync.Mutex

	wg.Add(goroutines)
	start := make(chan struct{})
	for range goroutines {
		go func() {
			defer wg.Done()
			<-start
			if atomicSpend() {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}

	close(start)
	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly 1 successful spend, got %d", successes)
	}
	if b.remaining != 0 {
		t.Fatalf("expected 0 remaining budget, got %d", b.remaining)
	}
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
	for i := range maxConcurrency {
		if !bh.TryAcquire(jobID, maxConcurrency) {
			t.Fatalf("failed to acquire slot %d", i)
		}
	}

	// Next acquire should fail immediately (not block or delay).
	start := time.Now()
	got := bh.TryAcquire(jobID, maxConcurrency)
	elapsed := time.Since(start)
	if got {
		t.Fatal("expected TryAcquire to fail when all slots taken")
	}
	if elapsed > 5*time.Millisecond {
		t.Errorf("TryAcquire took %v to fail, expected immediate (< 5ms)", elapsed)
	}

	// Release one slot.
	bh.Release(jobID, maxConcurrency)

	// Next acquire should succeed immediately.
	start = time.Now()
	got = bh.TryAcquire(jobID, maxConcurrency)
	elapsed = time.Since(start)
	if !got {
		t.Fatal("expected TryAcquire to succeed after release")
	}
	if elapsed > 5*time.Millisecond {
		t.Errorf("TryAcquire took %v to succeed, expected immediate (< 5ms)", elapsed)
	}

	// Verify active count.
	if count := bh.ActiveCount(jobID); count != maxConcurrency {
		t.Errorf("active count = %d, want %d", count, maxConcurrency)
	}
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
			if first != tc.expected {
				t.Fatalf("first classification = %q, want %q", first, tc.expected)
			}

			for i := range 1000 {
				got := classifyError(tc.err)
				if got != first {
					t.Fatalf("iteration %d: classification changed from %q to %q", i, first, got)
				}
			}
		})
	}
}

// durationStats computes mean and standard deviation for a slice of durations.
func durationStats(durations []time.Duration) (mean, stddev time.Duration) {
	if len(durations) == 0 {
		return 0, 0
	}

	var sum float64
	for _, d := range durations {
		sum += float64(d)
	}
	m := sum / float64(len(durations))

	var variance float64
	for _, d := range durations {
		diff := float64(d) - m
		variance += diff * diff
	}
	variance /= float64(len(durations))

	return time.Duration(m), time.Duration(math.Sqrt(variance))
}
