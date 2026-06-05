package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"math/rand/v2"
	"testing"
	"time"
)

// TestProperty_RetryDelay_Bounded verifies that for any attempt number and any
// strategy, the returned delay is always within [800ms, 1h12m] (base 1s with
// +/-20% jitter, max 1h with +20% jitter).
func TestProperty_RetryDelay_Bounded(t *testing.T) {
	t.Parallel()

	strategies := []string{"", RetryExponential, RetryLinear, RetryFixed, RetryCustom}

	// With +/-20% jitter: minimum is 0.8*1s = 800ms, maximum is 1.2*1h.
	lowerBound := 800 * time.Millisecond
	upperBound := time.Duration(float64(time.Hour) * 1.2)

	for range 2000 {
		attempt := rand.IntN(100) + 1
		strategy := strategies[rand.IntN(len(strategies))]

		var customDelays []int
		if strategy == RetryCustom {
			n := rand.IntN(10) + 1
			customDelays = make([]int, n)
			for j := range customDelays {
				customDelays[j] = rand.IntN(7200) // 0 to 2h in seconds.
			}
		}

		delay := NextRetryDelayWithStrategy(attempt, strategy, customDelays)

		if delay < lowerBound {
			t.Fatalf("strategy=%q attempt=%d: delay %v < lower bound %v",
				strategy, attempt, delay, lowerBound)
		}
		if delay > upperBound {
			t.Fatalf("strategy=%q attempt=%d: delay %v > upper bound %v",
				strategy, attempt, delay, upperBound)
		}
	}
}

// TestProperty_Backoff_MonotonicallyNonDecreasing verifies that for exponential
// strategy, the base delay (before jitter) is non-decreasing as attempts grow.
// Because jitter adds randomness, we check that the exponentialDelay helper
// itself is monotonic.
func TestProperty_Backoff_MonotonicallyNonDecreasing(t *testing.T) {
	t.Parallel()

	base := 1 * time.Second
	maxDelay := 1 * time.Hour

	for range 1000 {
		startAttempt := rand.IntN(50) + 1
		prev := exponentialDelay(startAttempt, base, maxDelay)

		for attempt := startAttempt + 1; attempt <= startAttempt+20; attempt++ {
			curr := exponentialDelay(attempt, base, maxDelay)
			if curr < prev {
				t.Fatalf("exponentialDelay decreased: attempt %d=%v > attempt %d=%v",
					attempt-1, prev, attempt, curr)
			}
			prev = curr
		}
	}
}

// TestProperty_Concurrency_NeverExceedsMax verifies that random sequences of
// TryAcquire and Release calls on a ShardedBulkhead never allow ActiveCount
// to exceed the configured concurrency limit.
func TestProperty_Concurrency_NeverExceedsMax(t *testing.T) {
	t.Parallel()

	for range 500 {
		maxConc := rand.IntN(20) + 1
		bh := NewShardedBulkhead(0)
		jobID := "test-job"

		acquired := 0
		ops := rand.IntN(200) + 50

		for range ops {
			if rand.IntN(3) < 2 && acquired < maxConc+10 {
				// Try to acquire.
				ok := bh.TryAcquire(jobID, maxConc)
				if ok {
					acquired++
				}
			} else if acquired > 0 {
				// Release.
				bh.Release(jobID, maxConc)
				acquired--
			}

			count := bh.ActiveCount(jobID)
			if count > maxConc {
				t.Fatalf("ActiveCount %d exceeds max %d", count, maxConc)
			}
			if count < 0 {
				t.Fatalf("ActiveCount %d is negative", count)
			}
		}
	}
}

// TestProperty_ErrorHash_Deterministic verifies that errorHash produces the
// same output when called twice with the same input.
func TestProperty_ErrorHash_Deterministic(t *testing.T) {
	t.Parallel()

	charset := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 !@#$%^&*()")
	for range 2000 {
		length := rand.IntN(500)
		buf := make([]byte, length)
		for j := range buf {
			buf[j] = charset[rand.IntN(len(charset))]
		}
		msg := string(buf)

		h1 := errorHash(msg)
		h2 := errorHash(msg)
		if h1 != h2 {
			t.Fatalf("errorHash not deterministic for len=%d: %q != %q", length, h1, h2)
		}

		// Verify it is a valid 16-char hex string.
		if len(h1) != 16 {
			t.Fatalf("errorHash returned %d chars, want 16", len(h1))
		}
		if _, err := hex.DecodeString(h1); err != nil {
			t.Fatalf("errorHash returned non-hex: %q", h1)
		}
	}
}

// TestProperty_ErrorHash_MatchesReference verifies the hash output against a
// direct sha256 computation for inputs under 200 characters.
func TestProperty_ErrorHash_MatchesReference(t *testing.T) {
	t.Parallel()

	for range 1000 {
		length := rand.IntN(200)
		buf := make([]byte, length)
		for j := range buf {
			buf[j] = byte(rand.IntN(128))
		}
		msg := string(buf)

		got := errorHash(msg)
		h := sha256.Sum256([]byte(msg))
		want := hex.EncodeToString(h[:8])
		if got != want {
			t.Fatalf("errorHash(%q) = %q, want %q", msg, got, want)
		}
	}
}
