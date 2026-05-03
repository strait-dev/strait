package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

// TestReaper_ReapStale_TransientDBFailure_ContinuesOnNextTick verifies that
// when ListStaleRuns returns a transient error, reapStale logs and returns
// without panicking, and a subsequent call (simulating the next tick) succeeds
// and processes runs normally.
func TestReaper_ReapStale_TransientDBFailure_ContinuesOnNextTick(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	var transitioned atomic.Int32

	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			n := callCount.Add(1)
			// First call: simulate transient DB failure.
			if n == 1 {
				return nil, errors.New("connection reset by peer")
			}
			// Second call: return a stale run so the reaper processes it.
			return []domain.JobRun{
				{ID: "run-stale", JobID: "job-1", Status: domain.StatusExecuting},
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, to domain.RunStatus, _ map[string]any) error {
			if to == domain.StatusCrashed {
				transitioned.Add(1)
			}
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)

	// First tick: DB fails — must not panic, must return cleanly.
	r.reapStale(context.Background())

	if callCount.Load() != 1 {
		t.Fatalf("expected 1 ListStaleRuns call after first tick, got %d", callCount.Load())
	}
	if transitioned.Load() != 0 {
		t.Fatalf("expected 0 transitions after DB failure, got %d", transitioned.Load())
	}

	// Second tick: DB succeeds — run must be processed.
	r.reapStale(context.Background())

	if callCount.Load() != 2 {
		t.Fatalf("expected 2 total ListStaleRuns calls, got %d", callCount.Load())
	}
	if transitioned.Load() != 1 {
		t.Fatalf("expected 1 crash transition after recovery, got %d", transitioned.Load())
	}
}
