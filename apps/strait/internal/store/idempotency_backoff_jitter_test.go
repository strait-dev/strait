package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestIdempotencyBackoffWithJitter_FirstAttemptIsZero documents the
// no-sleep semantics of attempt 0 (the initial try is unwaited).
func TestIdempotencyBackoffWithJitter_FirstAttemptIsZero(t *testing.T) {
	t.Parallel()
	require.EqualValues(t, 0, idempotencyBackoffWithJitter(0))
}

// TestIdempotencyBackoffWithJitter_OutOfRangeIsZero protects against
// accidental callers passing attempts past the configured table — the
// function must degrade to no-sleep rather than panic with an index out
// of range.
func TestIdempotencyBackoffWithJitter_OutOfRangeIsZero(t *testing.T) {
	t.Parallel()
	require.EqualValues(t, 0, idempotencyBackoffWithJitter(idempotencyMaxAttempts))
	require.EqualValues(t, 0, idempotencyBackoffWithJitter(-1))
}

// TestIdempotencyBackoffWithJitter_StaysWithinBounds verifies the
// jittered duration is within ±idempotencyBackoffJitter of the base
// value for each non-zero attempt. Run enough samples to cover the
// distribution; flakes here would indicate either the math is wrong or
// the jitter constant is mistuned.
func TestIdempotencyBackoffWithJitter_StaysWithinBounds(t *testing.T) {
	t.Parallel()

	for attempt := 1; attempt < idempotencyMaxAttempts; attempt++ {
		base := idempotencyBackoff[attempt]
		lo := time.Duration(float64(base) * (1 - idempotencyBackoffJitter))
		hi := time.Duration(float64(base) * (1 + idempotencyBackoffJitter))
		for range 2000 {
			got := idempotencyBackoffWithJitter(attempt)
			require.False(t,
				got <
					lo ||
					got >= hi)
		}
	}
}

// TestIdempotencyBackoffWithJitter_ProducesVariance proves the jitter is
// not a constant: across 200 samples the spread must exceed half of the
// theoretical max delta. This is the regression guard against accidental
// reversion to a constant-sleep schedule (the bug we're fixing).
func TestIdempotencyBackoffWithJitter_ProducesVariance(t *testing.T) {
	t.Parallel()

	const samples = 200
	min, max := time.Duration(1<<62), time.Duration(0)
	for range samples {
		got := idempotencyBackoffWithJitter(2) // 20ms base
		if got < min {
			min = got
		}
		if got > max {
			max = got
		}
	}

	spread := max - min
	// Theoretical max spread is 2 * 20ms * 0.2 = 8ms. Require at least
	// half of that — 4ms — across 200 samples to call it variant.
	minSpread := time.Duration(float64(idempotencyBackoff[2]) * idempotencyBackoffJitter)
	require.GreaterOrEqual(
		t,

		spread, minSpread)
}
