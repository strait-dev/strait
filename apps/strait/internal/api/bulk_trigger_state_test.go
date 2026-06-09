package api

import (
	"context"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestBulkTriggerStateAppendRunResultsUpdatesCounters(t *testing.T) {
	t.Parallel()

	state := &bulkTriggerState{}

	state.appendExistingRun(&domain.JobRun{
		ID:     "run-existing",
		Status: domain.StatusCompleted,
	}, true)
	state.appendCreatedRun(&domain.JobRun{
		ID:     "run-new",
		Status: domain.StatusQueued,
	})
	require.Len(t,
		state.results,

		2)

	require.Equal(t, BulkTriggerResult{
		ID:             "run-existing",
		Status:         string(domain.StatusCompleted),
		IdempotencyHit: true,
	}, state.results[0])
	require.Equal(t, BulkTriggerResult{
		ID:     "run-new",
		Status: string(domain.StatusQueued),
	}, state.results[1])
	require.Equal(t, 1, state.
		created)
	require.Equal(t, 1, state.
		enqueuedInBatch)
}

func TestBulkTriggerStateBuffersRunWhenBatchCanUseEnqueueBatch(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "run-buffered"}
	state := &bulkTriggerState{
		server: &Server{
			queue: &mockQueue{
				enqueueFn: func(context.Context, *domain.JobRun) error {
					require.Fail(t,

						"Enqueue must not be called for non-idempotent runs outside a transaction")
					return nil
				},
			},
		},
		ctx:               context.Background(),
		hasIdempotencyKey: false,
	}

	handled, err := state.enqueueOrBufferRun(run, BulkTriggerItem{}, 0)
	require.NoError(t, err)
	require.False(t, handled)
	require.False(t, len(state.
		pendingRuns) != 1 || state.
		pendingRuns[0] != run)
}

func TestBulkTriggerStateEnqueuesImmediatelyWhenIdempotencyPresent(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "run-idempotent"}
	var captured *domain.JobRun
	state := &bulkTriggerState{
		server: &Server{
			queue: &mockQueue{
				enqueueFn: func(_ context.Context, got *domain.JobRun) error {
					captured = got
					return nil
				},
			},
		},
		ctx:               context.Background(),
		hasIdempotencyKey: true,
	}

	handled, err := state.enqueueOrBufferRun(run, BulkTriggerItem{IdempotencyKey: "idem-1"}, 0)
	require.NoError(t, err)
	require.False(t, handled)
	require.Equal(t, run, captured)
	require.Empty(t,
		state.pendingRuns)
}

func TestBulkTriggerStateEnqueuePendingRunsUsesBatch(t *testing.T) {
	t.Parallel()

	runs := []*domain.JobRun{{ID: "run-1"}, {ID: "run-2"}}
	var captured []*domain.JobRun
	state := &bulkTriggerState{
		server: &Server{
			queue: &mockQueue{
				enqueueBatchFn: func(_ context.Context, got []*domain.JobRun) (int64, error) {
					captured = got
					return int64(len(got)), nil
				},
				enqueueFn: func(context.Context, *domain.JobRun) error {
					require.Fail(t,

						"Enqueue must not be called when EnqueueBatch is available")
					return nil
				},
			},
		},
		ctx:         context.Background(),
		pendingRuns: runs,
	}
	require.NoError(t, state.
		enqueuePendingRuns())
	require.Len(t,
		captured,

		len(runs))

	for i := range runs {
		require.Equal(t, runs[i],

			captured[i])
	}
}
