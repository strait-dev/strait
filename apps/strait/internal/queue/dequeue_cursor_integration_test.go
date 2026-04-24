//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
)

func TestDequeueNWithCursor_HappyPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-cursor-happy")
	q := mustQueue(t)

	const N = 50
	for i := 0; i < N; i++ {
		mustEnqueueRun(t, ctx, q, job)
	}

	cursor := queue.NewClaimCursor(60 * time.Second)
	seen := make(map[string]bool)
	for {
		batch, err := q.DequeueNWithCursor(ctx, 10, cursor)
		if err != nil {
			t.Fatalf("DequeueNWithCursor: %v", err)
		}
		if len(batch) == 0 {
			break
		}
		for _, r := range batch {
			if seen[r.ID] {
				t.Errorf("duplicate claim of %s", r.ID)
			}
			seen[r.ID] = true
		}
	}
	if len(seen) != N {
		t.Errorf("claimed %d runs, want %d", len(seen), N)
	}
}

func TestDequeueNWithCursor_ResetsOnEmpty(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-cursor-reset")
	q := mustQueue(t)

	for i := 0; i < 5; i++ {
		mustEnqueueRun(t, ctx, q, job)
	}

	cursor := queue.NewClaimCursor(60 * time.Second)

	// Drain once.
	batch, err := q.DequeueNWithCursor(ctx, 10, cursor)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if len(batch) != 5 {
		t.Fatalf("drained %d, want 5", len(batch))
	}

	// Second call is empty => cursor should reset.
	_, err = q.DequeueNWithCursor(ctx, 10, cursor)
	if err != nil {
		t.Fatalf("empty call: %v", err)
	}
	if _, _, ok := cursor.Snapshot(); ok {
		t.Error("cursor should be reset after empty claim")
	}
}

func TestDequeueNWithCursor_RetriesRemainReachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-cursor-retry")
	q := mustQueue(t)

	// Enqueue a single run, dequeue it, then requeue it with a past
	// created_at (simulating a retry). After a cursor reset the requeued
	// run must still be claimed.
	original := mustEnqueueRun(t, ctx, q, job)
	cursor := queue.NewClaimCursor(10 * time.Millisecond)

	batch, err := q.DequeueNWithCursor(ctx, 10, cursor)
	if err != nil {
		t.Fatalf("initial dequeue: %v", err)
	}
	if len(batch) != 1 {
		t.Fatalf("got %d runs, want 1", len(batch))
	}

	// Manually requeue the same row with an older created_at.
	past := time.Now().Add(-1 * time.Hour)
	_, err = testDB.Pool.Exec(ctx, `
		UPDATE job_runs
		SET status='queued', created_at=$2, started_at=NULL, next_retry_at=NULL
		WHERE id=$1
	`, original.ID, past)
	if err != nil {
		t.Fatalf("requeue: %v", err)
	}

	// Wait past the cursor interval so the Snapshot returns invalid.
	time.Sleep(30 * time.Millisecond)

	batch, err = q.DequeueNWithCursor(ctx, 10, cursor)
	if err != nil {
		t.Fatalf("post-retry dequeue: %v", err)
	}
	if len(batch) != 1 || batch[0].ID != original.ID {
		t.Errorf("expected to reclaim %s, got %v", original.ID, batch)
	}
}

func TestDequeueNWithCursor_NilCursorFallsBackToBaseBehaviour(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-cursor-nil")
	q := mustQueue(t)

	for i := 0; i < 3; i++ {
		mustEnqueueRun(t, ctx, q, job)
	}

	batch, err := q.DequeueNWithCursor(ctx, 5, nil)
	if err != nil {
		t.Fatalf("DequeueNWithCursor(nil): %v", err)
	}
	if len(batch) != 3 {
		t.Fatalf("got %d, want 3", len(batch))
	}
}

func TestDequeueNWithCursor_MultipleWorkersShareCursorClass(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-cursor-workers")
	q := mustQueue(t)

	const N = 40
	for i := 0; i < N; i++ {
		mustEnqueueRun(t, ctx, q, job)
	}

	// Two workers with their own cursors dequeue in parallel.
	type result struct {
		runs []domain.JobRun
		err  error
	}
	ch := make(chan result, 2)
	go func() {
		c := queue.NewClaimCursor(60 * time.Second)
		var all []domain.JobRun
		for {
			batch, err := q.DequeueNWithCursor(ctx, 8, c)
			if err != nil || len(batch) == 0 {
				ch <- result{all, err}
				return
			}
			all = append(all, batch...)
		}
	}()
	go func() {
		c := queue.NewClaimCursor(60 * time.Second)
		var all []domain.JobRun
		for {
			batch, err := q.DequeueNWithCursor(ctx, 8, c)
			if err != nil || len(batch) == 0 {
				ch <- result{all, err}
				return
			}
			all = append(all, batch...)
		}
	}()

	total := 0
	seen := make(map[string]bool)
	for i := 0; i < 2; i++ {
		r := <-ch
		if r.err != nil {
			t.Fatalf("worker err: %v", r.err)
		}
		for _, run := range r.runs {
			if seen[run.ID] {
				t.Errorf("duplicate claim of %s", run.ID)
			}
			seen[run.ID] = true
			total++
		}
	}
	if total != N {
		t.Errorf("total claimed = %d, want %d", total, N)
	}
}
