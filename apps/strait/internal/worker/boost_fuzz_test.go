package worker

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBoostPriority_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		current  int
		boost    int
		expected int
	}{
		{"zero_plus_one", 0, 1, 1},
		{"three_plus_two", 3, 2, 5},
		{"eight_plus_two_exact_max", 8, 2, 10},
		{"nine_plus_three_capped", 9, 3, 10},
		{"ten_plus_one_capped", 10, 1, 10},
		{"ten_plus_ten_capped", 10, 10, 10},
		{"zero_plus_ten_max", 0, 10, 10},
		{"five_plus_five_exact_max", 5, 5, 10},
		{"maxint_plus_one_overflow", math.MaxInt, 1, 10},
		{"maxint_plus_maxint_overflow", math.MaxInt, math.MaxInt, 10},
		{"large_current_plus_large_boost", 1000000, 1000000, 10},
		{"negative_current_plus_boost", -5, 3, -2},
		{"negative_current_large_boost", -5, 20, 10},
		{"zero_plus_zero", 0, 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := boostPriority(tc.current, tc.boost)
			require.Equal(t,
				tc.expected,
				got)
		})
	}
}

// TestBoostPriority_Overflow verifies that boosting beyond max is capped at 10.
func TestBoostPriority_Overflow(t *testing.T) {
	t.Parallel()
	got := boostPriority(9, 5)
	require.Equal(t, 10, got)
}

// TestBoostPriority_NegativeBoost verifies that a negative boost triggers the
// overflow guard (boosted < current) and caps at 10.
func TestBoostPriority_NegativeBoost(t *testing.T) {
	t.Parallel()
	got := boostPriority(5, -3)
	require.Equal(t, 10, got)

	// boosted = 5 + (-3) = 2, which is < current (5), so the overflow guard
	// activates and returns 10.
}

// TestBoostPriority_ZeroCurrent verifies boosting from zero priority.
func TestBoostPriority_ZeroCurrent(t *testing.T) {
	t.Parallel()
	got := boostPriority(0, 3)
	require.Equal(t, 3, got)
}

func FuzzBoostPriority(f *testing.F) {
	// Seed corpus with interesting values.
	seeds := []struct{ current, boost int }{
		{0, 0},
		{0, 1},
		{0, 10},
		{5, 5},
		{10, 1},
		{10, 10},
		{9, 3},
		{-1, 1},
		{-5, 3},
		{-100, 200},
		{math.MaxInt, 1},
		{math.MaxInt, math.MaxInt},
		{math.MinInt, 1},
		{math.MinInt, math.MaxInt},
		{0, math.MaxInt},
		{0, math.MinInt},
		{1000000, 1000000},
	}
	for _, s := range seeds {
		f.Add(s.current, s.boost)
	}

	f.Fuzz(func(t *testing.T, current, boost int) {
		result := boostPriority(current, boost)
		assert.LessOrEqual(t, result,
			10)
		assert.False(t,
			current >=
				0 && boost >
				0 && result <
				current && result != 10)

		// Invariant 1: result must never exceed 10.

		// Invariant 2: if both inputs are non-negative and boost > 0,
		// the result should be >= current (monotonically increasing).

		// The only valid reason result < current is if we hit the cap.
		// But if result != 10, it means we went down -- that's a bug.

		// Invariant 3: if both inputs are in valid range [0,10],
		// result must equal min(current+boost, 10).
		if current >= 0 && current <= 10 && boost >= 0 && boost <= 10 {
			expected := min(current+boost, 10)
			assert.Equal(t,
				expected,
				result)
		}
	})
}

func FuzzBoostPriorityOverflow(f *testing.F) {
	// Focus on overflow edge cases.
	f.Add(math.MaxInt-1, 2)
	f.Add(math.MaxInt, 1)
	f.Add(math.MaxInt/2, math.MaxInt/2+2)
	f.Add(1<<62, 1<<62)
	f.Add(math.MaxInt-10, 20)

	f.Fuzz(func(t *testing.T, current, boost int) {
		// Only test positive inputs where overflow is possible.
		if current < 0 || boost < 0 {
			return
		}

		result := boostPriority(current, boost)
		assert.GreaterOrEqual(t,
			result, 0)
		assert.LessOrEqual(t, result,
			10)

		// Must never overflow to negative.

		// Must never exceed 10.
	})
}

// TestProperty_Priority_HigherFirst verifies that boostPriority always returns
// a value >= the original priority and never exceeds the cap of 10.
func TestProperty_Priority_HigherFirst(t *testing.T) {
	t.Parallel()

	for range 2000 {
		current := rand.IntN(11) // 0-10.
		boost := rand.IntN(20)   // 0-19.

		result := boostPriority(current, boost)
		require.GreaterOrEqual(t,
			result, current,
		)
		require.LessOrEqual(t, result,
			10)
	}
}

func FuzzHandleFailureRetryPriority(f *testing.F) {
	// Fuzz the full handleFailure path with varied priority and boost values.
	f.Add(0, 1, 1)
	f.Add(5, 2, 1)
	f.Add(10, 3, 1)
	f.Add(0, 0, 1)
	f.Add(9, 10, 1)
	f.Add(0, 1, 3)   // max attempts, won't retry
	f.Add(0, 5, 10)  // high boost
	f.Add(-1, 3, 1)  // negative priority
	f.Add(10, 10, 1) // both at max

	f.Fuzz(func(t *testing.T, priority, boost, attempt int) {
		// Constrain to reasonable ranges to avoid meaningless inputs.
		if attempt < 1 || attempt > 100 {
			return
		}
		if boost < 0 || boost > 100 {
			return
		}

		store := &mockExecutorStore{}
		exec := NewExecutor(ExecutorConfig{
			Pool:         NewPool(4),
			Queue:        &mockExecQueue{},
			Store:        store,
			PollInterval: 1<<63 - 1, // max duration, never poll
		})

		maxAttempts := attempt + 1 // ensure at least one retry is possible
		run := &domain.JobRun{ID: "run-fuzz", JobID: "job-fuzz", Attempt: attempt, Priority: priority}
		job := &domain.Job{ID: "job-fuzz", EndpointURL: "http://example.com", RetryPriorityBoost: boost, MaxAttempts: maxAttempts}
		policy := executionPolicy{maxAttempts: maxAttempts, timeoutSecs: 30}

		// Must never panic.
		exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

		// Verify invariants on the status update.
		for _, c := range store.statusUpdates() {
			if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
				if p, ok := c.fields["priority"]; ok {
					pInt, isInt := p.(int)
					if !isInt {
						assert.Failf(t, "test failure",

							"priority field is not int: %T", p)
						continue
					}
					assert.LessOrEqual(t, pInt,
						10)
				}
			}
		}
		assert.Equal(t,
			priority,
			run.Priority)

		// Original run must not be mutated.
	})
}
