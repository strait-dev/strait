//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"
)

// Integration tests for the fully-denormalized dequeue path.

func TestDequeueNFullyDenormalized_HappyPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fulldn-happy")
	q := mustQueue(t)

	for i := 0; i < 15; i++ {
		mustEnqueueRun(t, ctx, q, job)
	}

	// Verify the denormalized columns were seeded on enqueue.
	var enabled, paused bool
	err := testDB.Pool.QueryRow(ctx,
		`SELECT COALESCE(job_enabled, false), COALESCE(job_paused, false) FROM job_runs WHERE job_id = $1 LIMIT 1`,
		job.ID,
	).Scan(&enabled, &paused)
	if err != nil {
		t.Fatalf("seed check: %v", err)
	}
	if !enabled || paused {
		t.Errorf("denormalized columns not seeded: enabled=%v paused=%v", enabled, paused)
	}

	batch, err := q.DequeueNFullyDenormalized(ctx, 15)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if len(batch) != 15 {
		t.Errorf("got %d, want 15", len(batch))
	}
}

func TestFanoutJobConfig_UpdatesQueuedRuns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fanout-paused")
	q := mustQueue(t)

	for i := 0; i < 5; i++ {
		mustEnqueueRun(t, ctx, q, job)
	}

	// Pause the job. Fan-out trigger should update the denormalized column
	// on the queued rows.
	_, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = true WHERE id = $1`, job.ID)
	if err != nil {
		t.Fatalf("pause: %v", err)
	}

	// Verify the column is now true on all queued rows.
	var pausedCount int
	err = testDB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_runs WHERE job_id = $1 AND job_paused = true`,
		job.ID,
	).Scan(&pausedCount)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if pausedCount != 5 {
		t.Errorf("fan-out updated %d rows, want 5", pausedCount)
	}

	// Fully-denormalized dequeue returns zero (job is paused).
	batch, err := q.DequeueNFullyDenormalized(ctx, 10)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if len(batch) != 0 {
		t.Errorf("paused job yielded %d runs, want 0", len(batch))
	}

	// Unpause, fan-out should clear.
	_, err = testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = false WHERE id = $1`, job.ID)
	if err != nil {
		t.Fatalf("unpause: %v", err)
	}
	batch, err = q.DequeueNFullyDenormalized(ctx, 10)
	if err != nil {
		t.Fatalf("dequeue post-unpause: %v", err)
	}
	if len(batch) != 5 {
		t.Errorf("after unpause got %d, want 5", len(batch))
	}
}

func TestFanoutJobConfig_DisabledHidesNewQueued(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fanout-disable")
	q := mustQueue(t)

	mustEnqueueRun(t, ctx, q, job)
	_, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET enabled = false WHERE id = $1`, job.ID)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	batch, err := q.DequeueNFullyDenormalized(ctx, 10)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if len(batch) != 0 {
		t.Errorf("disabled job yielded %d runs, want 0", len(batch))
	}
}

func TestFanoutJobConfig_UpdatesMaxConcurrency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fanout-mc")
	q := mustQueue(t)

	for range 5 {
		mustEnqueueRun(t, ctx, q, job)
	}

	// Set max_concurrency = 2; fan-out updates queued rows.
	_, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency = 2 WHERE id = $1`, job.ID)
	if err != nil {
		t.Fatalf("update mc: %v", err)
	}
	var seededMC int
	err = testDB.Pool.QueryRow(ctx,
		`SELECT COALESCE(job_max_concurrency, 0) FROM job_runs WHERE job_id = $1 LIMIT 1`,
		job.ID,
	).Scan(&seededMC)
	if err != nil {
		t.Fatalf("seed query: %v", err)
	}
	if seededMC != 2 {
		t.Errorf("fanout max_concurrency = %d, want 2", seededMC)
	}

	// The counter-based dequeue enforces max_concurrency across calls;
	// call one at a time and assert the limit is eventually hit.
	var claimed int
	for range 5 {
		batch, err := q.DequeueNFullyDenormalized(ctx, 1)
		if err != nil {
			t.Fatalf("dequeue: %v", err)
		}
		if len(batch) == 0 {
			break
		}
		claimed += len(batch)
	}
	if claimed != 2 {
		t.Errorf("claimed %d, want 2 (max_concurrency)", claimed)
	}
}
