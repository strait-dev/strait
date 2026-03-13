//go:build integration

package queue_test

import (
	"context"
	"testing"

	"strait/internal/domain"
)

func TestDequeue_RespectsMaxConcurrencyPerKey(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-dequeue-conc-key")
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency_per_key = 1 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set max_concurrency_per_key error = %v", err)
	}

	active := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		Status: domain.StatusExecuting, Attempt: 1,
		ConcurrencyKey: "tenant-a",
	}
	if err := st.CreateRun(ctx, active); err != nil {
		t.Fatalf("CreateRun() active error = %v", err)
	}

	candidate := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		ConcurrencyKey: "tenant-a",
	}
	if err := q.Enqueue(ctx, candidate); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if dequeued != nil {
		t.Fatalf("Dequeue() returned run %s, want nil (blocked by concurrency key)", dequeued.ID)
	}
}

func TestDequeue_DifferentKeyAllowed(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-dequeue-diff-key")
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency_per_key = 1 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set max_concurrency_per_key error = %v", err)
	}

	active := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		Status: domain.StatusExecuting, Attempt: 1,
		ConcurrencyKey: "tenant-a",
	}
	if err := st.CreateRun(ctx, active); err != nil {
		t.Fatalf("CreateRun() active error = %v", err)
	}

	candidate := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		ConcurrencyKey: "tenant-b",
	}
	if err := q.Enqueue(ctx, candidate); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if dequeued == nil {
		t.Fatalf("Dequeue() returned nil, want run (different concurrency key should not block)")
	}
	if dequeued.ID != candidate.ID {
		t.Fatalf("Dequeue() returned run %s, want %s", dequeued.ID, candidate.ID)
	}
}

func TestDequeue_NoConcurrencyKeyBypasses(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-dequeue-no-key")
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency_per_key = 1 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set max_concurrency_per_key error = %v", err)
	}

	active := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		Status: domain.StatusExecuting, Attempt: 1,
		ConcurrencyKey: "tenant-a",
	}
	if err := st.CreateRun(ctx, active); err != nil {
		t.Fatalf("CreateRun() active error = %v", err)
	}

	candidate := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		ConcurrencyKey: "",
	}
	if err := q.Enqueue(ctx, candidate); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if dequeued == nil {
		t.Fatalf("Dequeue() returned nil, want run (empty concurrency key bypasses guard)")
	}
	if dequeued.ID != candidate.ID {
		t.Fatalf("Dequeue() returned run %s, want %s", dequeued.ID, candidate.ID)
	}
}

func TestDequeueN_RespectsMaxConcurrencyPerKey(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-dequeuen-conc-key")
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency_per_key = 1 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set max_concurrency_per_key error = %v", err)
	}

	active := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		Status: domain.StatusExecuting, Attempt: 1,
		ConcurrencyKey: "tenant-a",
	}
	if err := st.CreateRun(ctx, active); err != nil {
		t.Fatalf("CreateRun() active error = %v", err)
	}

	candidate := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		ConcurrencyKey: "tenant-a",
	}
	if err := q.Enqueue(ctx, candidate); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	runs, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN() error = %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("DequeueN() returned %d runs, want 0 (blocked by concurrency key)", len(runs))
	}
}

func TestDequeueNByProject_RespectsMaxConcurrencyPerKey(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-dequeuenbp-conc-key")
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency_per_key = 1 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set max_concurrency_per_key error = %v", err)
	}

	active := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		Status: domain.StatusExecuting, Attempt: 1,
		ConcurrencyKey: "tenant-a",
	}
	if err := st.CreateRun(ctx, active); err != nil {
		t.Fatalf("CreateRun() active error = %v", err)
	}

	candidate := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		ConcurrencyKey: "tenant-a",
	}
	if err := q.Enqueue(ctx, candidate); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	runs, err := q.DequeueNByProject(ctx, 1, job.ProjectID)
	if err != nil {
		t.Fatalf("DequeueNByProject() error = %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("DequeueNByProject() returned %d runs, want 0 (blocked by concurrency key)", len(runs))
	}
}
