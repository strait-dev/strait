package worker

import (
	"fmt"
	"testing"
	"time"
)

func TestNextRetryDelay(t *testing.T) {
	t.Parallel()
	tests := []struct {
		attempt     int
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{attempt: 1, expectedMin: 800 * time.Millisecond, expectedMax: 1200 * time.Millisecond},
		{attempt: 2, expectedMin: 1600 * time.Millisecond, expectedMax: 2400 * time.Millisecond},
		{attempt: 3, expectedMin: 3200 * time.Millisecond, expectedMax: 4800 * time.Millisecond},
		{attempt: 4, expectedMin: 6400 * time.Millisecond, expectedMax: 9600 * time.Millisecond},
		{attempt: 5, expectedMin: 12800 * time.Millisecond, expectedMax: 19200 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			delay := NextRetryDelay(tt.attempt)
			if delay < tt.expectedMin || delay > tt.expectedMax {
				t.Errorf("attempt %d: delay %v not in range [%v, %v]", tt.attempt, delay, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestNextRetryDelay_Cap(t *testing.T) {
	t.Parallel()
	delay := NextRetryDelay(100)
	maxAllowed := time.Hour + time.Hour/5
	if delay > maxAllowed {
		t.Errorf("attempt 100: delay %v exceeds max allowed %v", delay, maxAllowed)
	}
}

func TestNextRetryDelay_ZeroAttempt(t *testing.T) {
	t.Parallel()
	delay := NextRetryDelay(0)
	if delay < 800*time.Millisecond || delay > 1200*time.Millisecond {
		t.Errorf("attempt 0: delay %v not in expected range", delay)
	}
}

func TestNextRetryDelay_Jitter(t *testing.T) {
	t.Parallel()
	seen := make(map[time.Duration]bool)
	for range 100 {
		delay := NextRetryDelay(1)
		seen[delay.Truncate(time.Millisecond)] = true
	}
	if len(seen) < 5 {
		t.Errorf("expected variance from jitter, but only got %d unique values", len(seen))
	}
}

func TestNextRetryAt(t *testing.T) {
	t.Parallel()
	before := time.Now()
	retryAt := NextRetryAt(1)
	after := time.Now()

	minExpected := before.Add(800 * time.Millisecond)
	maxExpected := after.Add(1200 * time.Millisecond)

	if retryAt.Before(minExpected) || retryAt.After(maxExpected) {
		t.Errorf("NextRetryAt(1) = %v, expected between %v and %v", retryAt, minExpected, maxExpected)
	}
}

func TestNextRetryDelayWithStrategy_Linear(t *testing.T) {
	t.Parallel()
	tests := []struct {
		attempt     int
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{attempt: 1, expectedMin: 800 * time.Millisecond, expectedMax: 1200 * time.Millisecond},
		{attempt: 2, expectedMin: 1600 * time.Millisecond, expectedMax: 2400 * time.Millisecond},
		{attempt: 3, expectedMin: 2400 * time.Millisecond, expectedMax: 3600 * time.Millisecond},
		{attempt: 10, expectedMin: 8 * time.Second, expectedMax: 12 * time.Second},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			delay := NextRetryDelayWithStrategy(tt.attempt, RetryLinear, nil)
			if delay < tt.expectedMin || delay > tt.expectedMax {
				t.Errorf("linear attempt %d: delay %v not in range [%v, %v]", tt.attempt, delay, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestNextRetryDelayWithStrategy_Fixed(t *testing.T) {
	t.Parallel()
	for range 20 {
		delay := NextRetryDelayWithStrategy(5, RetryFixed, nil)
		if delay < 800*time.Millisecond || delay > 1200*time.Millisecond {
			t.Errorf("fixed attempt 5: delay %v not in range [800ms, 1200ms]", delay)
		}
	}
}

func TestNextRetryDelayWithStrategy_Custom(t *testing.T) {
	t.Parallel()
	customDelays := []int{5, 30, 120}

	// Attempt 1 -> 5s +- jitter
	delay1 := NextRetryDelayWithStrategy(1, RetryCustom, customDelays)
	if delay1 < 4*time.Second || delay1 > 6*time.Second {
		t.Errorf("custom attempt 1: delay %v not in expected range [4s, 6s]", delay1)
	}

	// Attempt 2 -> 30s +- jitter
	delay2 := NextRetryDelayWithStrategy(2, RetryCustom, customDelays)
	if delay2 < 24*time.Second || delay2 > 36*time.Second {
		t.Errorf("custom attempt 2: delay %v not in expected range [24s, 36s]", delay2)
	}

	// Attempt 3 -> 120s +- jitter
	delay3 := NextRetryDelayWithStrategy(3, RetryCustom, customDelays)
	if delay3 < 96*time.Second || delay3 > 144*time.Second {
		t.Errorf("custom attempt 3: delay %v not in expected range [96s, 144s]", delay3)
	}

	// Attempt beyond custom delays length -> uses last value (120s)
	delay4 := NextRetryDelayWithStrategy(10, RetryCustom, customDelays)
	if delay4 < 96*time.Second || delay4 > 144*time.Second {
		t.Errorf("custom attempt 10 (overflow): delay %v not in expected range [96s, 144s]", delay4)
	}
}

func TestNextRetryDelayWithStrategy_CustomEmpty(t *testing.T) {
	t.Parallel()
	// Empty custom delays should fallback to exponential
	delay := NextRetryDelayWithStrategy(1, RetryCustom, nil)
	if delay < 800*time.Millisecond || delay > 1200*time.Millisecond {
		t.Errorf("custom empty attempt 1: delay %v not in expected range", delay)
	}
}

func TestNextRetryDelayWithStrategy_DefaultIsExponential(t *testing.T) {
	t.Parallel()
	// Empty strategy string should use exponential
	delay := NextRetryDelayWithStrategy(3, "", nil)
	if delay < 3200*time.Millisecond || delay > 4800*time.Millisecond {
		t.Errorf("default (exponential) attempt 3: delay %v not in expected range [3.2s, 4.8s]", delay)
	}
}

func TestNextRetryAtWithStrategy(t *testing.T) {
	t.Parallel()
	before := time.Now()
	retryAt := NextRetryAtWithStrategy(1, RetryFixed, nil)
	after := time.Now()

	minExpected := before.Add(800 * time.Millisecond)
	maxExpected := after.Add(1200 * time.Millisecond)

	if retryAt.Before(minExpected) || retryAt.After(maxExpected) {
		t.Errorf("NextRetryAtWithStrategy(1, fixed) = %v, expected between %v and %v", retryAt, minExpected, maxExpected)
	}
}

func TestNextRetryDelayWithStrategy_LinearCap(t *testing.T) {
	t.Parallel()
	// Very high attempt should be capped at maxDelay (1 hour)
	delay := NextRetryDelayWithStrategy(100000, RetryLinear, nil)
	maxAllowed := time.Hour + time.Hour/5 // 1h + 20% jitter
	if delay > maxAllowed {
		t.Errorf("linear attempt 100000: delay %v exceeds max allowed %v", delay, maxAllowed)
	}
}

func TestNextRetryDelayWithStrategy_NegativeCustomDelays(t *testing.T) {
	t.Parallel()
	// Negative custom delays should be floored to base (1s) +-20% jitter.
	customDelays := []int{-5, -10, 30}
	delay := NextRetryDelayWithStrategy(1, RetryCustom, customDelays)
	if delay < 800*time.Millisecond || delay > 1200*time.Millisecond {
		t.Errorf("negative custom delay: got %v, want ~1s (floored to base)", delay)
	}
}

func TestNextRetryDelayWithStrategy_ZeroCustomDelay(t *testing.T) {
	t.Parallel()
	// Zero custom delays should be floored to base (1s) +-20% jitter.
	customDelays := []int{0, 5, 30}
	delay := NextRetryDelayWithStrategy(1, RetryCustom, customDelays)
	if delay < 800*time.Millisecond || delay > 1200*time.Millisecond {
		t.Errorf("zero custom delay: got %v, want ~1s (floored to base)", delay)
	}
}
