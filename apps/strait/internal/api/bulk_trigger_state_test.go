package api

import (
	"context"
	"testing"

	"strait/internal/domain"
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

	if len(state.results) != 2 {
		t.Fatalf("results = %d, want 2", len(state.results))
	}
	if got := state.results[0]; got.ID != "run-existing" || got.Status != string(domain.StatusCompleted) || !got.IdempotencyHit {
		t.Fatalf("existing result = %#v", got)
	}
	if got := state.results[1]; got.ID != "run-new" || got.Status != string(domain.StatusQueued) || got.IdempotencyHit {
		t.Fatalf("created result = %#v", got)
	}
	if state.created != 1 {
		t.Fatalf("created = %d, want 1", state.created)
	}
	if state.enqueuedInBatch != 1 {
		t.Fatalf("enqueuedInBatch = %d, want 1", state.enqueuedInBatch)
	}
}

func TestBulkTriggerStateBuffersRunWhenBatchCanUseEnqueueBatch(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "run-buffered"}
	state := &bulkTriggerState{
		server: &Server{
			queue: &mockQueue{
				enqueueFn: func(context.Context, *domain.JobRun) error {
					t.Fatal("Enqueue must not be called for non-idempotent runs outside a transaction")
					return nil
				},
			},
		},
		ctx:               context.Background(),
		hasIdempotencyKey: false,
	}

	handled, err := state.enqueueOrBufferRun(run, BulkTriggerItem{}, 0)
	if err != nil {
		t.Fatalf("enqueueOrBufferRun() error = %v", err)
	}
	if handled {
		t.Fatal("handled = true, want false")
	}
	if len(state.pendingRuns) != 1 || state.pendingRuns[0] != run {
		t.Fatalf("pendingRuns = %#v, want buffered run", state.pendingRuns)
	}
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
	if err != nil {
		t.Fatalf("enqueueOrBufferRun() error = %v", err)
	}
	if handled {
		t.Fatal("handled = true, want false")
	}
	if captured != run {
		t.Fatalf("captured run = %#v, want %#v", captured, run)
	}
	if len(state.pendingRuns) != 0 {
		t.Fatalf("pendingRuns = %d, want 0", len(state.pendingRuns))
	}
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
					t.Fatal("Enqueue must not be called when EnqueueBatch is available")
					return nil
				},
			},
		},
		ctx:         context.Background(),
		pendingRuns: runs,
	}

	if err := state.enqueuePendingRuns(); err != nil {
		t.Fatalf("enqueuePendingRuns() error = %v", err)
	}
	if len(captured) != len(runs) {
		t.Fatalf("captured runs = %d, want %d", len(captured), len(runs))
	}
	for i := range runs {
		if captured[i] != runs[i] {
			t.Fatalf("captured[%d] = %#v, want %#v", i, captured[i], runs[i])
		}
	}
}
