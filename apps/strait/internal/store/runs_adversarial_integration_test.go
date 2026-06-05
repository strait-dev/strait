//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

// Adversarial tests for concurrent and edge-case run operations.

// TestAdv_ConcurrentCreateRuns verifies that multiple goroutines can create
// runs for the same job without conflict.
func TestAdv_ConcurrentCreateRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-adv-concurrent-create")

	const n = 20
	errs := make(chan error, n)
	var wg conc.WaitGroup

	for range n {
		wg.Go(func() {
			run := baseRun(job, newID())
			errs <- q.CreateRun(ctx, run)
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)

	}

	runs, err := q.ListRunsByJob(ctx, job.ID, 1000, 0)
	require.NoError(t, err)
	require.Len(t, runs, n)

}

// TestAdv_ConcurrentStatusTransitions verifies that concurrent status updates
// are idempotent: all goroutines succeed (the first mutates, the rest see
// the target status and return nil) and the final state is correct.
func TestAdv_ConcurrentStatusTransitions(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-adv-concurrent-status")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))

	const n = 10
	results := make(chan error, n)
	start := make(chan struct{})
	var wg conc.WaitGroup

	for range n {
		wg.Go(func() {
			<-start
			results <- q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, nil)
		})
	}

	close(start)
	wg.Wait()
	close(results)

	for err := range results {
		require.NoError(t, err)

	}

	// Verify the final state is completed.
	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusCompleted,
		got.
			Status)

}

// TestAdv_DeleteJobWhileRunsExist ensures deletion fails when active runs exist.
func TestAdv_DeleteJobWhileRunsExist(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-adv-delete-active")
	run := baseRun(job, newID())
	run.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		run))

	err := q.DeleteJob(ctx, job.ID)
	require.True(t, errors.Is(err, store.
		ErrJobHasActiveRuns,
	))

}

// TestAdv_UpdateRunStatus_InvalidTransition verifies that an invalid
// from->to transition is rejected.
func TestAdv_UpdateRunStatus_InvalidTransition(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-adv-invalid-transition")
	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	require.NoError(t, q.CreateRun(ctx,
		run))

	// completed -> executing is not valid in practice, but depends on store implementation.
	err := q.UpdateRunStatus(ctx, run.ID, domain.StatusCompleted, domain.StatusExecuting, nil)
	// Should either error or return a conflict-type error because completed is terminal.
	if err == nil {
		// Check if it actually changed.
		got, _ := q.GetRun(ctx, run.ID)
		require.False(t, got !=

			nil && got.
			Status ==
			domain.StatusExecuting,
		)

	}
}

// TestAdv_GetRunAfterJobDelete verifies runs are cleaned up with job deletion.
func TestAdv_GetRunAfterJobDelete(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-adv-run-after-delete")
	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.DeleteJob(ctx,
		job.ID))

	_, err := q.GetRun(ctx, run.ID)
	require.Error(t, err)

}

// TestAdv_ConcurrentBatchInserts ensures concurrent batch buffer inserts succeed.
func TestAdv_ConcurrentBatchInserts(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-adv-batch-concurrent")

	const n = 15
	errs := make(chan error, n)
	var wg conc.WaitGroup

	for range n {
		wg.Go(func() {
			item := &domain.BatchBufferItem{
				JobID:       job.ID,
				ProjectID:   job.ProjectID,
				BatchKey:    "concurrent-key",
				Payload:     []byte(`{"ok":true}`),
				Tags:        []byte(`{}`),
				TriggeredBy: "manual",
			}
			errs <- q.InsertBatchBufferItem(ctx, item)
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)

	}

	count, _ := q.CountBatchBufferItems(ctx, job.ID, "concurrent-key")
	require.Equal(t, n, count)

}

// TestAdv_DrainBatchConcurrent ensures concurrent drains do not return duplicates.
func TestAdv_DrainBatchConcurrent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-adv-drain-concurrent")
	for range 10 {
		item := &domain.BatchBufferItem{
			JobID:       job.ID,
			ProjectID:   job.ProjectID,
			BatchKey:    "drain-race",
			Payload:     []byte(`{}`),
			Tags:        []byte(`{}`),
			TriggeredBy: "manual",
		}
		require.NoError(t, q.InsertBatchBufferItem(ctx,
			item))

	}

	type result struct {
		items []domain.BatchBufferItem
		err   error
	}
	results := make(chan result, 2)
	var wg conc.WaitGroup

	for range 2 {
		wg.Go(func() {
			items, err := q.DrainBatchBuffer(ctx, job.ID, "drain-race", 10)
			results <- result{items, err}
		})
	}
	wg.Wait()
	close(results)

	totalDrained := 0
	for r := range results {
		require.Nil(t, r.
			err)

		totalDrained += len(r.items)
	}
	require.EqualValues(t, 10, totalDrained)

}

// TestAdv_CreateRunWithEmptyPayload validates that empty payloads are handled.
func TestAdv_CreateRunWithEmptyPayload(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-adv-empty-payload")
	run := baseRun(job, newID())
	run.Payload = nil
	require.NoError(t, q.CreateRun(ctx,
		run))

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

}

// TestAdv_ConcurrentJobMemoryUpserts ensures concurrent memory upserts
// for different keys succeed.
func TestAdv_ConcurrentJobMemoryUpserts(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-adv-mem-concurrent")

	const n = 10
	errs := make(chan error, n)
	var wg conc.WaitGroup

	for i := range n {
		wg.Go(func() {
			mem := &domain.JobMemory{
				JobID:     job.ID,
				ProjectID: job.ProjectID,
				MemoryKey: "key-" + string(rune('A'+i)),
				Value:     []byte(`"value"`),
				SizeBytes: 7,
			}
			errs <- store.New(testDB.Pool).UpsertJobMemoryWithQuota(ctx, mem, 1024, 100)
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)

	}
}
