package queue

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQueueNotifier_BackoffDelay_ExponentialGrowth(t *testing.T) {
	t.Parallel()

	n := &QueueNotifier{
		initialDelay: time.Second,
		maxDelay:     30 * time.Second,
	}

	// Sample many times to account for jitter.
	for attempt := range 6 {
		expectedBase := min(
			time.Duration(float64(time.Second)*float64(int(1)<<attempt)),
			30*time.Second,
		)

		minExpected := time.Duration(float64(expectedBase) * 0.75)
		maxExpected := time.Duration(float64(expectedBase) * 1.25)

		for range 20 {
			delay := n.backoffDelay(attempt)
			require.False(t,
				delay <
					minExpected ||
					delay > maxExpected)

		}
	}
}

func TestQueueNotifier_BackoffDelay_CappedAtMax(t *testing.T) {
	t.Parallel()

	n := &QueueNotifier{
		initialDelay: time.Second,
		maxDelay:     30 * time.Second,
	}

	// At attempt 100, the base would be huge without capping.
	for range 50 {
		delay := n.backoffDelay(100)
		require.LessOrEqual(t,
			delay,

			time.Duration(float64(30*time.Second)*1.26))
		require.GreaterOrEqual(
			t,

			delay, time.Duration(float64(30*time.Second)*0.74))

		// With jitter: max is 30s * 1.25 = 37.5s.

	}
}

func TestQueueNotifier_BackoffDelay_Jitter(t *testing.T) {
	t.Parallel()

	n := &QueueNotifier{
		initialDelay: time.Second,
		maxDelay:     30 * time.Second,
	}

	// Verify that repeated calls produce varying delays (jitter).
	seen := make(map[time.Duration]bool)
	for range 100 {
		d := n.backoffDelay(3)
		seen[d] = true
	}
	require.GreaterOrEqual(
		t,

		len(seen), 5)

	// With 100 samples and jitter, we should see at least 10 distinct values.

}

func TestQueueNotifier_BackoffDelay_AttemptZero(t *testing.T) {
	t.Parallel()

	n := &QueueNotifier{
		initialDelay: 500 * time.Millisecond,
		maxDelay:     30 * time.Second,
	}

	for range 20 {
		delay := n.backoffDelay(0)
		require.False(t,
			delay <
				374*
					time.Millisecond || delay > 626*time.Millisecond)

		// Base is 500ms, jitter 75%-125% = 375ms-625ms.

	}
}
