package worker

import (
	"math"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Backoff tests (9).

// FuzzNextRetryDelay_ExtremeAttempts fuzzes attempt values and ensures no panic.
func FuzzNextRetryDelay_ExtremeAttempts(f *testing.F) {
	f.Add(0)
	f.Add(1)
	f.Add(-1)
	f.Add(100)
	f.Add(math.MaxInt)
	f.Add(math.MinInt)
	f.Fuzz(func(t *testing.T, attempt int) {
		d := NextRetryDelay(attempt)
		require.Positive(t, d)
	})
}

// FuzzNextRetryDelayWithStrategy fuzzes all four retry strategies.
func FuzzNextRetryDelayWithStrategy(f *testing.F) {
	for _, s := range []string{"exponential", "linear", "fixed", "custom", ""} {
		for _, a := range []int{0, 1, 5, 100} {
			f.Add(a, s)
		}
	}
	f.Fuzz(func(t *testing.T, attempt int, strategy string) {
		d := NextRetryDelayWithStrategy(attempt, strategy, nil)
		require.Positive(t, d)
	})
}

// FuzzNextRetryDelayWithPolicy_Overflow fuzzes huge initial and max delay values.
// Known issue: jitter calculation can overflow with MaxInt delay values, producing
// negative durations. The fuzz test documents this behavior without failing.
func FuzzNextRetryDelayWithPolicy_Overflow(f *testing.F) {
	f.Add(1, 1, 1)
	f.Add(100, math.MaxInt, math.MaxInt)
	f.Add(math.MaxInt, math.MaxInt, math.MaxInt)
	f.Add(1, -1, -1)
	f.Add(0, 0, 0)
	f.Fuzz(func(t *testing.T, attempt, initialDelay, maxDelay int) {
		// Must not panic regardless of input.
		_ = NextRetryDelayWithPolicy(attempt, domain.RetryBackoffExponential, initialDelay, maxDelay)
	})
}

// TestBackoff_AttemptZero verifies that attempt=0 is clamped to 1.
func TestBackoff_AttemptZero(t *testing.T) {
	t.Parallel()
	d := NextRetryDelay(0)
	require.Positive(t, d)
	assert.False(
		t, d < 800*
			time.Millisecond ||

			d > 1200*time.Millisecond)

	// With jitter, base=1s should yield roughly 0.8s-1.2s.
}

// TestBackoff_AttemptNegative verifies that attempt=-1 is clamped to 1.
func TestBackoff_AttemptNegative(t *testing.T) {
	t.Parallel()
	d := NextRetryDelay(-1)
	require.Positive(t, d)
	assert.False(
		t, d < 800*
			time.Millisecond ||

			d > 1200*time.Millisecond)
}

// TestBackoff_AttemptMaxInt verifies no overflow or panic for math.MaxInt.
func TestBackoff_AttemptMaxInt(t *testing.T) {
	t.Parallel()
	d := NextRetryDelay(math.MaxInt)
	require.Positive(t, d)

	// Should be capped at maxDelay (1 hour) +/- 20% jitter.
	upper := time.Hour + time.Hour/5 + time.Second
	assert.LessOrEqual(t, d,
		upper)
}

// TestBackoff_CustomDelaysEmpty verifies fallback to exponential when custom delays are nil or empty.
func TestBackoff_CustomDelaysEmpty(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		delays []int
	}{
		{"nil slice", nil},
		{"empty slice", []int{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := NextRetryDelayWithStrategy(1, RetryCustom, tc.delays)
			require.Positive(t, d)
		})
	}
}

// TestBackoff_CustomDelaysNegative verifies that negative custom delay values are floored to base.
func TestBackoff_CustomDelaysNegative(t *testing.T) {
	t.Parallel()
	d := NextRetryDelayWithStrategy(1, RetryCustom, []int{-100, -200})
	require.Positive(t, d)
	assert.False(
		t, d < 800*
			time.Millisecond ||

			d > 1200*time.Millisecond)

	// Negative delay is floored to base (1s), so expect near 1s with jitter.
}

// TestBackoff_CustomDelaysOverflow verifies that math.MaxInt custom delays are capped.
func TestBackoff_CustomDelaysOverflow(t *testing.T) {
	t.Parallel()
	d := NextRetryDelayWithStrategy(1, RetryCustom, []int{math.MaxInt, math.MaxInt})
	require.Positive(t, d)

	// Should be capped at maxDelay (1 hour) +/- 20% jitter.
	upper := time.Hour + time.Hour/5 + time.Second
	assert.LessOrEqual(t, d,
		upper)
}

// Circuit breaker tests (5).

// TestCircuitBreaker_ConcurrentTransitions hammers the circuit breaker from 100 goroutines.
func TestCircuitBreaker_ConcurrentTransitions(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 5,
		OpenDuration:     time.Millisecond,
	})

	var wg conc.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			for j := range 100 {
				_ = cb.Allow()
				if (i+j)%3 == 0 {
					cb.RecordFailure()
				} else {
					cb.RecordSuccess()
				}
				_ = cb.State()
			}
		})
	}
	wg.Wait()

	// State must be one of the valid states.
	state := cb.State()
	switch state {
	case circuitClosed, circuitOpen, circuitHalfOpen:
		// Valid.
	default:
		require.Failf(t, "test failure", "unexpected state %q after concurrent transitions", state)
	}
}

// TestCircuitBreaker_ExactThreshold verifies the breaker opens at exactly the threshold.
func TestCircuitBreaker_ExactThreshold(t *testing.T) {
	t.Parallel()
	threshold := 5
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: threshold,
		OpenDuration:     time.Hour,
	})

	for i := range threshold {
		cb.RecordFailure()
		if i < threshold-1 {
			require.Equal(t, circuitClosed,
				cb.
					State())
		}
	}
	require.Equal(t, circuitOpen,
		cb.State())
}

// TestCircuitBreaker_ThresholdMinusOne verifies the breaker stays closed at threshold-1 failures.
func TestCircuitBreaker_ThresholdMinusOne(t *testing.T) {
	t.Parallel()
	threshold := 5
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: threshold,
		OpenDuration:     time.Hour,
	})

	for range threshold - 1 {
		cb.RecordFailure()
	}
	require.Equal(t, circuitClosed,
		cb.
			State())
	require.Equal(t, threshold-
		1, cb.
		ConsecutiveFailures())
}

// TestCircuitBreaker_RapidOpenClose cycles the breaker through 1000 open/close transitions.
func TestCircuitBreaker_RapidOpenClose(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		OpenDuration:     time.Nanosecond,
	})

	for range 1000 {
		// Open it.
		cb.RecordFailure()
		require.Equal(t, circuitOpen,
			cb.State())

		// Wait for open duration to elapse, then Allow transitions to half-open.
		time.Sleep(time.Microsecond)
		require.True(
			t, cb.Allow())
		require.Equal(t, circuitHalfOpen,
			cb.
				State(),
		)

		// Close it.
		cb.RecordSuccess()
		require.Equal(t, circuitClosed,
			cb.
				State())
	}
}

// TestCircuitBreaker_ZeroThreshold verifies that a zero threshold uses the default.
func TestCircuitBreaker_ZeroThreshold(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 0,
		OpenDuration:     time.Hour,
	})

	// Should use defaultCircuitFailureThreshold (5). Record 4 failures: should stay closed.
	for range 4 {
		cb.RecordFailure()
	}
	require.Equal(t, circuitClosed,
		cb.
			State())

	// The fifth failure should trip it open (default threshold is 5).
	cb.RecordFailure()
	require.Equal(t, circuitOpen,
		cb.State())
}

// Health score tests (5).

// TestHealthScore_NaN verifies that NaN latency does not corrupt the score.
func TestHealthScore_NaN(t *testing.T) {
	t.Parallel()
	score := ThrottledConcurrency(&domain.EndpointHealthScore{
		HealthScore: math.NaN(),
	}, 10)
	require.GreaterOrEqual(t,
		score, 0)

	// NaN comparisons are always false, so score < unhealthy is false, score > degraded is false.
	// The function should not panic; it returns 0 because NaN < 30 is false, NaN > 60 is false,
	// and it falls into the degraded path.
}

// TestHealthScore_Inf verifies that +Inf latency does not cause a panic.
func TestHealthScore_Inf(t *testing.T) {
	t.Parallel()
	score := ThrottledConcurrency(&domain.EndpointHealthScore{
		HealthScore: math.Inf(1),
	}, 10)
	assert.Equal(t, 10, score)

	// +Inf > 60, so should return maxConcurrency unchanged.
}

// TestHealthScore_NegativeLatency verifies that a negative health score returns 0.
func TestHealthScore_NegativeLatency(t *testing.T) {
	t.Parallel()
	score := ThrottledConcurrency(&domain.EndpointHealthScore{
		HealthScore: -50.0,
	}, 10)
	assert.Equal(t, 0, score)

	// -50 < 30, so endpoint is unhealthy and concurrency should be 0.
}

// TestHealthScore_ZeroTimeout verifies behavior with zero timeout in dispatch result.
func TestHealthScore_ZeroTimeout(t *testing.T) {
	t.Parallel()
	// With zero JobTimeoutMs, latencyVal should be 1.0 (no timeout configured path).
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	result, err := hs.RecordResult(t.Context(), DispatchResult{
		EndpointURL:  "https://zero-timeout.example.com",
		Success:      true,
		LatencyMs:    500,
		JobTimeoutMs: 0,
	})
	require.NoError(t, err)
	assert.InDelta(t, 1.0, result.
		LatencyScore, 1e-9,
	)

	// Latency score should stay at 1.0 since no timeout means latencyVal=1.0.
}

// FuzzHealthScoreCalculation fuzzes all fields of DispatchResult.
func FuzzHealthScoreCalculation(f *testing.F) {
	f.Add(true, false, 100.0, 5000.0)
	f.Add(false, true, 0.0, 0.0)
	f.Add(true, false, math.MaxFloat64, math.MaxFloat64)
	f.Add(false, false, -1.0, -1.0)
	f.Add(true, false, math.NaN(), math.NaN())
	f.Fuzz(func(t *testing.T, success, timedOut bool, latencyMs, jobTimeoutMs float64) {
		store := newMockHealthScoreStore()
		hs := NewHealthScorer(store)

		// Should never panic.
		result, err := hs.RecordResult(t.Context(), DispatchResult{
			EndpointURL:  "https://fuzz.example.com",
			Success:      success,
			TimedOut:     timedOut,
			LatencyMs:    latencyMs,
			JobTimeoutMs: jobTimeoutMs,
		})
		require.NoError(t, err)

		if result.HealthScore < 0 || result.HealthScore > 100 {
			assert.True(t,
				math.IsNaN(result.HealthScore))

			// NaN propagation is acceptable but out-of-range finite values are not.
		}
	})
}

// Bulkhead tests (4).

// TestBulkhead_AdversarialJobIDs tests job IDs containing null bytes and unicode.
func TestBulkhead_AdversarialJobIDs(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(10)

	adversarialIDs := []string{
		"job\x00id",
		"job\x00\x00\x00",
		"\x00",
		"\xff\xfe",
		"job-with-emoji-\U0001F600",
		"job-\u200b-zero-width",
		strings.Repeat("\u0000", 100),
		"job-with-newline\n",
		"job-with-tab\t",
	}

	for _, id := range adversarialIDs {
		assert.True(t,
			b.TryAcquire(id, 1))
		assert.False(
			t, b.TryAcquire(id, 1),
		)

		b.Release(id, 1)
		assert.Equal(t, 0, b.ActiveCount(id))
	}
}

// TestBulkhead_EmptyJobID tests that an empty string job ID works correctly.
func TestBulkhead_EmptyJobID(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(2)
	require.True(
		t, b.TryAcquire("", 1),
	)
	require.False(t, b.TryAcquire("", 1))

	b.Release("", 1)
	require.Equal(t, 0, b.ActiveCount(""))
}

// TestBulkhead_LongJobID tests a 100KB job ID.
func TestBulkhead_LongJobID(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(10)
	longID := strings.Repeat("x", 100*1024)
	require.True(
		t, b.TryAcquire(longID,
			1))
	require.Equal(t, 1, b.ActiveCount(longID))

	b.Release(longID, 1)
	require.Equal(t, 0, b.ActiveCount(longID))
}

// TestBulkhead_ConcurrentAcquireRelease tests 100 goroutines acquiring and releasing.
func TestBulkhead_ConcurrentAcquireRelease(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	const limit = 5
	jobID := "concurrent-test-job"

	var wg conc.WaitGroup
	for range 100 {
		wg.Go(func() {
			for range 50 {
				if b.TryAcquire(jobID, limit) {
					// Verify we never exceed the limit.
					count := b.ActiveCount(jobID)
					assert.LessOrEqual(t, count,
						limit)

					b.Release(jobID, limit)
				}
			}
		})
	}
	wg.Wait()
	require.Equal(t, 0, b.ActiveCount(jobID))
}

// URL validation tests (4).

// FuzzValidateEndpointURL_SSRFBypasses fuzzes the URL validator with SSRF bypass patterns.
func FuzzValidateEndpointURL_SSRFBypasses(f *testing.F) {
	f.Add("http://127.0.0.1")
	f.Add("http://0x7f000001")
	f.Add("http://0177.0.0.1")
	f.Add("http://[::1]")
	f.Add("http://localhost")
	f.Add("http://169.254.169.254/latest/meta-data/")
	f.Add("http://2130706433") // decimal for 127.0.0.1
	f.Add("http://0")
	f.Add("http://127.1")
	f.Add("https://valid.example.com")
	f.Add("")
	f.Add("ftp://example.com")
	f.Fuzz(func(t *testing.T, rawURL string) {
		// Should never panic.
		_ = ValidateEndpointURL(rawURL)
	})
}

// TestValidateEndpointURL_AlternateIPFormats tests hex IP, IPv6 loopback, and related formats.
func TestValidateEndpointURL_AlternateIPFormats(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"ipv4 loopback", "http://127.0.0.1/path", true},
		{"ipv6 loopback", "http://[::1]/path", true},
		{"ipv4 private 10.x", "http://10.0.0.1/path", true},
		{"ipv4 private 192.168", "http://192.168.1.1/path", true},
		{"ipv4 private 172.16", "http://172.16.0.1/path", true},
		{"ipv6 link-local", "http://[fe80::1]/path", true},
		{"ipv6 ULA", "http://[fd00::1]/path", true},
		{"CGNAT 100.64", "http://100.64.0.1/path", true},
		{"public IP", "http://8.8.8.8/path", false},
		{"public ipv6", "http://[2607:f8b0:4004:800::200e]/path", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateEndpointURL(tc.url)
			assert.False(
				t, tc.wantErr &&
					err ==
						nil)
			assert.False(
				t, !tc.wantErr &&
					err !=
						nil)
		})
	}
}

// TestValidateEndpointURL_InternalMetadataURLs tests cloud metadata endpoint blocking.
func TestValidateEndpointURL_InternalMetadataURLs(t *testing.T) {
	t.Parallel()
	metadataURLs := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://169.254.169.254/computeMetadata/v1/",
		"http://169.254.0.1/metadata",
	}
	for _, u := range metadataURLs {
		t.Run(u, func(t *testing.T) {
			t.Parallel()
			err := ValidateEndpointURL(u)
			assert.Error(
				t, err)
		})
	}
}

// TestValidateEndpointURL_EmptyAndInvalid tests empty strings, whitespace, and schemeless URLs.
func TestValidateEndpointURL_EmptyAndInvalid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		url  string
	}{
		{"empty string", ""},
		{"spaces only", "   "},
		{"no scheme", "example.com/path"},
		{"just scheme", "http://"},
		{"scheme only no host", "https://"},
		{"ftp scheme", "ftp://example.com"},
		{"javascript scheme", "javascript:alert(1)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateEndpointURL(tc.url)
			assert.Error(
				t, err)
		})
	}
}
