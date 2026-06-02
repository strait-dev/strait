//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestDequeueNDenormalized_HappyPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-denorm-happy")
	q := mustQueue(t)

	for range 20 {
		mustEnqueueRun(t, ctx, q, job)
	}

	batch, err := q.DequeueNDenormalized(ctx, 20)
	if err != nil {
		t.Fatalf("DequeueNDenormalized: %v", err)
	}
	if len(batch) != 20 {
		t.Errorf("got %d, want 20", len(batch))
	}
	for _, r := range batch {
		if r.Status != domain.StatusDequeued {
			t.Errorf("status = %q, want dequeued", r.Status)
		}
	}
}

func TestJobActiveCounts_TriggerMaintainsCounter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-denorm-trig")
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency = 1000 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set max concurrency: %v", err)
	}
	q := mustQueue(t)

	getCount := func() int {
		var c int
		err := testDB.Pool.QueryRow(ctx,
			`SELECT COALESCE(SUM(count), 0) FROM job_active_counts WHERE job_id = $1`,
			job.ID,
		).Scan(&c)
		if err != nil {
			t.Fatalf("getCount: %v", err)
		}
		return c
	}

	if getCount() != 0 {
		t.Fatalf("initial count = %d", getCount())
	}

	// Enqueue 5 -> all queued, counter still 0.
	for range 5 {
		mustEnqueueRun(t, ctx, q, job)
	}
	if getCount() != 0 {
		t.Errorf("counter after enqueue = %d, want 0", getCount())
	}

	// Dequeue 5 -> counter should be 5.
	batch, err := q.DequeueNDenormalized(ctx, 5)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if len(batch) != 5 {
		t.Fatalf("dequeued %d, want 5", len(batch))
	}
	if got := getCount(); got != 5 {
		t.Errorf("counter after dequeue = %d, want 5", got)
	}

	// Mark 2 completed -> counter should be 3.
	for _, r := range batch[:2] {
		_, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id=$1`, r.ID)
		if err != nil {
			t.Fatalf("complete: %v", err)
		}
	}
	if got := getCount(); got != 3 {
		t.Errorf("counter after complete = %d, want 3", got)
	}

	// Transition the remaining 3 from 'dequeued' to 'executing' — counter stays 3.
	for _, r := range batch[2:] {
		_, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status='executing' WHERE id=$1`, r.ID)
		if err != nil {
			t.Fatalf("exec: %v", err)
		}
	}
	if got := getCount(); got != 3 {
		t.Errorf("counter after dequeued->executing = %d, want 3", got)
	}
}

func TestDequeueNDenormalized_RespectsMaxConcurrency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-denorm-cc")
	// Set max concurrency to 2.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency = 2 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set cc: %v", err)
	}

	q := mustQueue(t)
	for range 10 {
		mustEnqueueRun(t, ctx, q, job)
	}

	// The counter-based dequeue enforces max_concurrency *across* calls:
	// a single batched call reads the counter once and may claim more
	// than the limit. Callers that need strict enforcement call Dequeue
	// one at a time.
	var claimed []string
	for range 5 {
		batch, err := q.DequeueNDenormalized(ctx, 1)
		if err != nil {
			t.Fatalf("dequeue: %v", err)
		}
		if len(batch) == 0 {
			break
		}
		claimed = append(claimed, batch[0].ID)
	}
	if len(claimed) != 2 {
		t.Errorf("claimed %d, want 2 (max concurrency, one at a time)", len(claimed))
	}

	// A further dequeue should yield zero because the two runs are active.
	batch2, err := q.DequeueNDenormalized(ctx, 1)
	if err != nil {
		t.Fatalf("dequeue2: %v", err)
	}
	if len(batch2) != 0 {
		t.Errorf("further dequeue got %d, want 0", len(batch2))
	}

	// Complete one and retry: one more slot opens.
	_, err = testDB.Pool.Exec(ctx, `UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id=$1`, claimed[0])
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	batch3, err := q.DequeueNDenormalized(ctx, 1)
	if err != nil {
		t.Fatalf("dequeue3: %v", err)
	}
	if len(batch3) != 1 {
		t.Errorf("after complete got %d, want 1", len(batch3))
	}
}

func TestJobActiveCounts_SeedFromExistingState(t *testing.T) {
	// After the migration runs, existing active runs must be seeded.
	// This test inserts an active run directly, then verifies the trigger
	// reconciles on subsequent transitions.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-denorm-seed")
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency = 1000 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set max concurrency: %v", err)
	}
	q := mustQueue(t)

	mustEnqueueRun(t, ctx, q, job)

	// Directly transition to executing (bypassing dequeue).
	_, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status='executing', started_at=NOW() WHERE job_id=$1`, job.ID)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}

	var c int
	err = testDB.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(count), 0) FROM job_active_counts WHERE job_id = $1`,
		job.ID,
	).Scan(&c)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if c != 1 {
		t.Errorf("counter after direct UPDATE = %d, want 1", c)
	}
}

func TestJobActiveCounts_DeleteDecrementsCounter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-denorm-delete")
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency = 1000 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set max concurrency: %v", err)
	}
	q := mustQueue(t)
	mustEnqueueRun(t, ctx, q, job)

	_, err := q.DequeueNDenormalized(ctx, 1)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}

	var c int
	_ = testDB.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(count),0) FROM job_active_counts WHERE job_id=$1`, job.ID).Scan(&c)
	if c != 1 {
		t.Fatalf("pre-delete count = %d, want 1", c)
	}

	// Hard-delete the run (bypasses reaper).
	_, err = testDB.Pool.Exec(ctx, `DELETE FROM job_runs WHERE job_id = $1`, job.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	_ = testDB.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(count),0) FROM job_active_counts WHERE job_id=$1`, job.ID).Scan(&c)
	if c != 0 {
		t.Errorf("post-delete count = %d, want 0", c)
	}
}
