//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
)

// --------------------------------------------------------------------------.
// Adversarial tests for concurrent and edge-case run operations.
// --------------------------------------------------------------------------.

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
		if err != nil {
			t.Fatalf("concurrent CreateRun() error = %v", err)
		}
	}

	runs, err := q.ListRunsByJob(ctx, job.ID, 1000, 0)
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(runs) != n {
		t.Fatalf("len = %d, want %d", len(runs), n)
	}
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
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

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
		if err != nil {
			t.Fatalf("UpdateRunStatus() error = %v", err)
		}
	}

	// Verify the final state is completed.
	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusCompleted {
		t.Fatalf("status = %s, want completed", got.Status)
	}
}

// TestAdv_DeleteJobWhileRunsExist ensures deletion fails when active runs exist.
func TestAdv_DeleteJobWhileRunsExist(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-adv-delete-active")
	run := baseRun(job, newID())
	run.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	err := q.DeleteJob(ctx, job.ID)
	if !errors.Is(err, store.ErrJobHasActiveRuns) {
		t.Fatalf("DeleteJob() error = %v, want ErrJobHasActiveRuns", err)
	}
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
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	// completed -> executing is not valid in practice, but depends on store implementation.
	err := q.UpdateRunStatus(ctx, run.ID, domain.StatusCompleted, domain.StatusExecuting, nil)
	// Should either error or return a conflict-type error because completed is terminal.
	if err == nil {
		// Check if it actually changed.
		got, _ := q.GetRun(ctx, run.ID)
		if got != nil && got.Status == domain.StatusExecuting {
			t.Fatal("should not allow transitioning from completed to executing")
		}
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
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if err := q.DeleteJob(ctx, job.ID); err != nil {
		t.Fatalf("DeleteJob() error = %v", err)
	}

	_, err := q.GetRun(ctx, run.ID)
	if err == nil {
		t.Fatal("expected error getting run after job deletion")
	}
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
		if err != nil {
			t.Fatalf("concurrent InsertBatchBufferItem() error = %v", err)
		}
	}

	count, _ := q.CountBatchBufferItems(ctx, job.ID, "concurrent-key")
	if count != n {
		t.Fatalf("count = %d, want %d", count, n)
	}
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
		if err := q.InsertBatchBufferItem(ctx, item); err != nil {
			t.Fatalf("InsertBatchBufferItem() error = %v", err)
		}
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
		if r.err != nil {
			t.Fatalf("DrainBatchBuffer() error = %v", r.err)
		}
		totalDrained += len(r.items)
	}

	if totalDrained != 10 {
		t.Fatalf("total drained = %d, want 10", totalDrained)
	}
}

// TestAdv_CreateRunWithEmptyPayload validates that empty payloads are handled.
func TestAdv_CreateRunWithEmptyPayload(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-adv-empty-payload")
	run := baseRun(job, newID())
	run.Payload = nil
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() with nil payload error = %v", err)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil run")
	}
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
		if err != nil {
			t.Fatalf("concurrent UpsertJobMemoryWithQuota() error = %v", err)
		}
	}
}
