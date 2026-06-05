package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
	require.EqualValues(t, 1,
		callCount.
			Load())
	require.EqualValues(t, 0,
		transitioned.
			Load())

	// Second tick: DB succeeds — run must be processed.
	r.reapStale(context.Background())
	require.EqualValues(t, 2,
		callCount.
			Load())
	require.EqualValues(t, 1,
		transitioned.
			Load())

}
