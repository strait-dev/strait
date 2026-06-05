package worker

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
			assert.False(t, delay <
				tt.expectedMin ||
				delay > tt.expectedMax)
		})
	}
}

func TestNextRetryDelay_Cap(t *testing.T) {
	t.Parallel()
	delay := NextRetryDelay(100)
	maxAllowed := time.Hour + time.Hour/5
	assert.LessOrEqual(t, delay,
		maxAllowed,
	)
}

func TestNextRetryDelay_ZeroAttempt(t *testing.T) {
	t.Parallel()
	delay := NextRetryDelay(0)
	assert.False(t, delay <
		800*time.
			Millisecond ||
		delay > 1200*time.Millisecond,
	)
}

func TestNextRetryDelay_Jitter(t *testing.T) {
	t.Parallel()
	seen := make(map[time.Duration]bool)
	for range 100 {
		delay := NextRetryDelay(1)
		seen[delay.Truncate(time.Millisecond)] = true
	}
	assert.GreaterOrEqual(t,
		len(seen),
		5)
}

func TestNextRetryAt(t *testing.T) {
	t.Parallel()
	before := time.Now()
	retryAt := NextRetryAt(1)
	after := time.Now()

	minExpected := before.Add(800 * time.Millisecond)
	maxExpected := after.Add(1200 * time.Millisecond)
	assert.False(t, retryAt.
		Before(
			minExpected,
		) || retryAt.After(maxExpected))
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
			assert.False(t, delay <
				tt.expectedMin ||
				delay > tt.expectedMax)
		})
	}
}

func TestNextRetryDelayWithStrategy_Fixed(t *testing.T) {
	t.Parallel()
	for range 20 {
		delay := NextRetryDelayWithStrategy(5, RetryFixed, nil)
		assert.False(t, delay <
			800*time.
				Millisecond ||
			delay > 1200*time.Millisecond,
		)
	}
}

func TestNextRetryDelayWithStrategy_Custom(t *testing.T) {
	t.Parallel()
	customDelays := []int{5, 30, 120}

	// Attempt 1 -> 5s +- jitter
	delay1 := NextRetryDelayWithStrategy(1, RetryCustom, customDelays)
	assert.False(t, delay1 <
		4*time.
			Second ||
		delay1 > 6*time.Second)

	// Attempt 2 -> 30s +- jitter
	delay2 := NextRetryDelayWithStrategy(2, RetryCustom, customDelays)
	assert.False(t, delay2 <
		24*time.
			Second ||
		delay2 > 36*time.Second)

	// Attempt 3 -> 120s +- jitter
	delay3 := NextRetryDelayWithStrategy(3, RetryCustom, customDelays)
	assert.False(t, delay3 <
		96*time.
			Second ||
		delay3 > 144*time.Second)

	// Attempt beyond custom delays length -> uses last value (120s)
	delay4 := NextRetryDelayWithStrategy(10, RetryCustom, customDelays)
	assert.False(t, delay4 <
		96*time.
			Second ||
		delay4 > 144*time.Second)
}

func TestNextRetryDelayWithStrategy_CustomEmpty(t *testing.T) {
	t.Parallel()
	// Empty custom delays should fallback to exponential
	delay := NextRetryDelayWithStrategy(1, RetryCustom, nil)
	assert.False(t, delay <
		800*time.
			Millisecond ||
		delay > 1200*time.Millisecond,
	)
}

func TestNextRetryDelayWithStrategy_DefaultIsExponential(t *testing.T) {
	t.Parallel()
	// Empty strategy string should use exponential
	delay := NextRetryDelayWithStrategy(3, "", nil)
	assert.False(t, delay <
		3200*time.
			Millisecond || delay > 4800*time.Millisecond,
	)
}

func TestNextRetryAtWithStrategy(t *testing.T) {
	t.Parallel()
	before := time.Now()
	retryAt := NextRetryAtWithStrategy(1, RetryFixed, nil)
	after := time.Now()

	minExpected := before.Add(800 * time.Millisecond)
	maxExpected := after.Add(1200 * time.Millisecond)
	assert.False(t, retryAt.
		Before(
			minExpected,
		) || retryAt.After(maxExpected))
}

func TestNextRetryDelayWithStrategy_LinearCap(t *testing.T) {
	t.Parallel()
	// Very high attempt should be capped at maxDelay (1 hour)
	delay := NextRetryDelayWithStrategy(100000, RetryLinear, nil)
	maxAllowed := time.Hour + time.Hour/5
	assert.LessOrEqual(t, delay,
		maxAllowed,
	)

	// 1h + 20% jitter
}

func TestNextRetryDelayWithStrategy_NegativeCustomDelays(t *testing.T) {
	t.Parallel()
	// Negative custom delays should be floored to base (1s) +-20% jitter.
	customDelays := []int{-5, -10, 30}
	delay := NextRetryDelayWithStrategy(1, RetryCustom, customDelays)
	assert.False(t, delay <
		800*time.
			Millisecond ||
		delay > 1200*time.Millisecond,
	)
}

func TestNextRetryDelayWithStrategy_ZeroCustomDelay(t *testing.T) {
	t.Parallel()
	// Zero custom delays should be floored to base (1s) +-20% jitter.
	customDelays := []int{0, 5, 30}
	delay := NextRetryDelayWithStrategy(1, RetryCustom, customDelays)
	assert.False(t, delay <
		800*time.
			Millisecond ||
		delay > 1200*time.Millisecond,
	)
}
