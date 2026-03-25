package worker

import (
	"context"
	"math"
	"testing"

	"strait/internal/domain"
)

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

		// Invariant 1: result must never exceed 10.
		if result > 10 {
			t.Errorf("boostPriority(%d, %d) = %d, exceeds max priority 10", current, boost, result)
		}

		// Invariant 2: if both inputs are non-negative and boost > 0,
		// the result should be >= current (monotonically increasing).
		if current >= 0 && boost > 0 && result < current && result != 10 {
			// The only valid reason result < current is if we hit the cap.
			// But if result != 10, it means we went down -- that's a bug.
			t.Errorf("boostPriority(%d, %d) = %d, decreased without hitting cap", current, boost, result)
		}

		// Invariant 3: if both inputs are in valid range [0,10],
		// result must equal min(current+boost, 10).
		if current >= 0 && current <= 10 && boost >= 0 && boost <= 10 {
			expected := min(current+boost, 10)
			if result != expected {
				t.Errorf("boostPriority(%d, %d) = %d, want %d (both inputs in [0,10])", current, boost, result, expected)
			}
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

		// Must never overflow to negative.
		if result < 0 {
			t.Errorf("boostPriority(%d, %d) = %d, overflowed to negative", current, boost, result)
		}

		// Must never exceed 10.
		if result > 10 {
			t.Errorf("boostPriority(%d, %d) = %d, exceeds max priority 10", current, boost, result)
		}
	})
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
						t.Errorf("priority field is not int: %T", p)
						continue
					}
					if pInt > 10 {
						t.Errorf("retry priority %d exceeds cap of 10 (input: priority=%d, boost=%d)", pInt, priority, boost)
					}
				}
			}
		}

		// Original run must not be mutated.
		if run.Priority != priority {
			t.Errorf("run.Priority mutated from %d to %d", priority, run.Priority)
		}
	})
}
