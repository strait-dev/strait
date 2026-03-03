package worker

import (
	"testing"
	"time"
)

func TestNextRetryDelay(t *testing.T) {
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
		delay := NextRetryDelay(tt.attempt)
		if delay < tt.expectedMin || delay > tt.expectedMax {
			t.Errorf("attempt %d: delay %v not in range [%v, %v]", tt.attempt, delay, tt.expectedMin, tt.expectedMax)
		}
	}
}

func TestNextRetryDelay_Cap(t *testing.T) {
	delay := NextRetryDelay(100)
	maxAllowed := time.Hour + time.Hour/5
	if delay > maxAllowed {
		t.Errorf("attempt 100: delay %v exceeds max allowed %v", delay, maxAllowed)
	}
}

func TestNextRetryDelay_ZeroAttempt(t *testing.T) {
	delay := NextRetryDelay(0)
	if delay < 800*time.Millisecond || delay > 1200*time.Millisecond {
		t.Errorf("attempt 0: delay %v not in expected range", delay)
	}
}

func TestNextRetryDelay_Jitter(t *testing.T) {
	seen := make(map[time.Duration]bool)
	for i := 0; i < 100; i++ {
		delay := NextRetryDelay(1)
		seen[delay.Truncate(time.Millisecond)] = true
	}
	if len(seen) < 5 {
		t.Errorf("expected variance from jitter, but only got %d unique values", len(seen))
	}
}

func TestNextRetryAt(t *testing.T) {
	before := time.Now()
	retryAt := NextRetryAt(1)
	after := time.Now()

	minExpected := before.Add(800 * time.Millisecond)
	maxExpected := after.Add(1200 * time.Millisecond)

	if retryAt.Before(minExpected) || retryAt.After(maxExpected) {
		t.Errorf("NextRetryAt(1) = %v, expected between %v and %v", retryAt, minExpected, maxExpected)
	}
}
