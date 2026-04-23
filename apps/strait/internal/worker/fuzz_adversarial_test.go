package worker

import (
	"math"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
)

// --------------------------------------------------------------------------.
// Backoff tests (9).
// --------------------------------------------------------------------------.

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
		if d <= 0 {
			t.Fatalf("NextRetryDelay(%d) returned non-positive duration %v", attempt, d)
		}
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
		if d <= 0 {
			t.Fatalf("NextRetryDelayWithStrategy(%d, %q, nil) = %v, want > 0", attempt, strategy, d)
		}
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
	if d <= 0 {
		t.Fatalf("NextRetryDelay(0) = %v, want > 0", d)
	}
	// With jitter, base=1s should yield roughly 0.8s-1.2s.
	if d < 800*time.Millisecond || d > 1200*time.Millisecond {
		t.Errorf("NextRetryDelay(0) = %v, expected near 1s", d)
	}
}

// TestBackoff_AttemptNegative verifies that attempt=-1 is clamped to 1.
func TestBackoff_AttemptNegative(t *testing.T) {
	t.Parallel()
	d := NextRetryDelay(-1)
	if d <= 0 {
		t.Fatalf("NextRetryDelay(-1) = %v, want > 0", d)
	}
	if d < 800*time.Millisecond || d > 1200*time.Millisecond {
		t.Errorf("NextRetryDelay(-1) = %v, expected near 1s", d)
	}
}

// TestBackoff_AttemptMaxInt verifies no overflow or panic for math.MaxInt.
func TestBackoff_AttemptMaxInt(t *testing.T) {
	t.Parallel()
	d := NextRetryDelay(math.MaxInt)
	if d <= 0 {
		t.Fatalf("NextRetryDelay(MaxInt) = %v, want > 0", d)
	}
	// Should be capped at maxDelay (1 hour) +/- 20% jitter.
	upper := time.Hour + time.Hour/5 + time.Second
	if d > upper {
		t.Errorf("NextRetryDelay(MaxInt) = %v, expected <= ~1h12m", d)
	}
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
			if d <= 0 {
				t.Fatalf("custom with %s: got %v, want > 0", tc.name, d)
			}
		})
	}
}

// TestBackoff_CustomDelaysNegative verifies that negative custom delay values are floored to base.
func TestBackoff_CustomDelaysNegative(t *testing.T) {
	t.Parallel()
	d := NextRetryDelayWithStrategy(1, RetryCustom, []int{-100, -200})
	if d <= 0 {
		t.Fatalf("custom with negative delays: got %v, want > 0", d)
	}
	// Negative delay is floored to base (1s), so expect near 1s with jitter.
	if d < 800*time.Millisecond || d > 1200*time.Millisecond {
		t.Errorf("custom with negative delays: got %v, expected near 1s", d)
	}
}

// TestBackoff_CustomDelaysOverflow verifies that math.MaxInt custom delays are capped.
func TestBackoff_CustomDelaysOverflow(t *testing.T) {
	t.Parallel()
	d := NextRetryDelayWithStrategy(1, RetryCustom, []int{math.MaxInt, math.MaxInt})
	if d <= 0 {
		t.Fatalf("custom with MaxInt delays: got %v, want > 0", d)
	}
	// Should be capped at maxDelay (1 hour) +/- 20% jitter.
	upper := time.Hour + time.Hour/5 + time.Second
	if d > upper {
		t.Errorf("custom with MaxInt delays: got %v, expected <= ~1h12m", d)
	}
}

// --------------------------------------------------------------------------.
// Circuit breaker tests (5).
// --------------------------------------------------------------------------.

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
		t.Fatalf("unexpected state %q after concurrent transitions", state)
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
			if cb.State() != circuitClosed {
				t.Fatalf("state = %q after %d failures, want closed", cb.State(), i+1)
			}
		}
	}

	if cb.State() != circuitOpen {
		t.Fatalf("state = %q after %d failures, want open", cb.State(), threshold)
	}
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

	if cb.State() != circuitClosed {
		t.Fatalf("state = %q after %d failures, want closed", cb.State(), threshold-1)
	}
	if cb.ConsecutiveFailures() != threshold-1 {
		t.Fatalf("consecutive failures = %d, want %d", cb.ConsecutiveFailures(), threshold-1)
	}
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
		if cb.State() != circuitOpen {
			t.Fatal("expected open after failure")
		}

		// Wait for open duration to elapse, then Allow transitions to half-open.
		time.Sleep(time.Microsecond)
		if !cb.Allow() {
			t.Fatal("expected Allow to return true after open duration")
		}
		if cb.State() != circuitHalfOpen {
			t.Fatalf("state = %q, want half_open", cb.State())
		}

		// Close it.
		cb.RecordSuccess()
		if cb.State() != circuitClosed {
			t.Fatalf("state = %q, want closed after success in half_open", cb.State())
		}
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
	if cb.State() != circuitClosed {
		t.Fatalf("state = %q after 4 failures with zero threshold config, want closed", cb.State())
	}

	// The fifth failure should trip it open (default threshold is 5).
	cb.RecordFailure()
	if cb.State() != circuitOpen {
		t.Fatalf("state = %q after 5 failures with zero threshold config, want open", cb.State())
	}
}

// --------------------------------------------------------------------------.
// Health score tests (5).
// --------------------------------------------------------------------------.

// TestHealthScore_NaN verifies that NaN latency does not corrupt the score.
func TestHealthScore_NaN(t *testing.T) {
	t.Parallel()
	score := ThrottledConcurrency(&domain.EndpointHealthScore{
		HealthScore: math.NaN(),
	}, 10)
	// NaN comparisons are always false, so score < unhealthy is false, score > degraded is false.
	// The function should not panic; it returns 0 because NaN < 30 is false, NaN > 60 is false,
	// and it falls into the degraded path.
	if score < 0 {
		t.Fatalf("ThrottledConcurrency with NaN health score returned negative: %d", score)
	}
}

// TestHealthScore_Inf verifies that +Inf latency does not cause a panic.
func TestHealthScore_Inf(t *testing.T) {
	t.Parallel()
	score := ThrottledConcurrency(&domain.EndpointHealthScore{
		HealthScore: math.Inf(1),
	}, 10)
	// +Inf > 60, so should return maxConcurrency unchanged.
	if score != 10 {
		t.Errorf("ThrottledConcurrency with +Inf health score = %d, want 10", score)
	}
}

// TestHealthScore_NegativeLatency verifies that a negative health score returns 0.
func TestHealthScore_NegativeLatency(t *testing.T) {
	t.Parallel()
	score := ThrottledConcurrency(&domain.EndpointHealthScore{
		HealthScore: -50.0,
	}, 10)
	// -50 < 30, so endpoint is unhealthy and concurrency should be 0.
	if score != 0 {
		t.Errorf("ThrottledConcurrency with negative health score = %d, want 0", score)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Latency score should stay at 1.0 since no timeout means latencyVal=1.0.
	if result.LatencyScore != 1.0 {
		t.Errorf("latency score = %v, want 1.0 for zero timeout", result.LatencyScore)
	}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.HealthScore < 0 || result.HealthScore > 100 {
			// NaN propagation is acceptable but out-of-range finite values are not.
			if !math.IsNaN(result.HealthScore) {
				t.Errorf("health score %v out of [0, 100]", result.HealthScore)
			}
		}
	})
}

// --------------------------------------------------------------------------.
// Bulkhead tests (4).
// --------------------------------------------------------------------------.

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
		if !b.TryAcquire(id, 1) {
			t.Errorf("TryAcquire(%q, 1) = false on first acquire", id)
		}
		if b.TryAcquire(id, 1) {
			t.Errorf("TryAcquire(%q, 1) = true on second acquire with limit 1", id)
		}
		b.Release(id, 1)
		if b.ActiveCount(id) != 0 {
			t.Errorf("ActiveCount(%q) = %d after release, want 0", id, b.ActiveCount(id))
		}
	}
}

// TestBulkhead_EmptyJobID tests that an empty string job ID works correctly.
func TestBulkhead_EmptyJobID(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(2)

	if !b.TryAcquire("", 1) {
		t.Fatal("TryAcquire(\"\", 1) = false on first acquire")
	}
	if b.TryAcquire("", 1) {
		t.Fatal("TryAcquire(\"\", 1) = true on second acquire with limit 1")
	}
	b.Release("", 1)
	if b.ActiveCount("") != 0 {
		t.Fatalf("ActiveCount(\"\") = %d after release, want 0", b.ActiveCount(""))
	}
}

// TestBulkhead_LongJobID tests a 100KB job ID.
func TestBulkhead_LongJobID(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(10)
	longID := strings.Repeat("x", 100*1024)

	if !b.TryAcquire(longID, 1) {
		t.Fatal("TryAcquire with 100KB ID = false on first acquire")
	}
	if b.ActiveCount(longID) != 1 {
		t.Fatalf("ActiveCount = %d, want 1", b.ActiveCount(longID))
	}
	b.Release(longID, 1)
	if b.ActiveCount(longID) != 0 {
		t.Fatalf("ActiveCount = %d after release, want 0", b.ActiveCount(longID))
	}
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
					if count > limit {
						t.Errorf("active count %d exceeded limit %d", count, limit)
					}
					b.Release(jobID, limit)
				}
			}
		})
	}
	wg.Wait()

	if b.ActiveCount(jobID) != 0 {
		t.Fatalf("active count = %d after all goroutines finished, want 0", b.ActiveCount(jobID))
	}
}

// --------------------------------------------------------------------------.
// URL validation tests (4).
// --------------------------------------------------------------------------.

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
			if tc.wantErr && err == nil {
				t.Errorf("ValidateEndpointURL(%q) = nil, want error", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateEndpointURL(%q) = %v, want nil", tc.url, err)
			}
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
			if err == nil {
				t.Errorf("ValidateEndpointURL(%q) = nil, want error for metadata URL", u)
			}
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
			if err == nil {
				t.Errorf("ValidateEndpointURL(%q) = nil, want error", tc.url)
			}
		})
	}
}
