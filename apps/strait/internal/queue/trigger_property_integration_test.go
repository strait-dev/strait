//go:build integration

package queue_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
)

// Property tests for the trigger-maintained counters.
//
// The job_active_counts and dlq_counts triggers must
// preserve the invariant:
//
//   counter row value == COUNT(*) over matching job_runs predicate
//
// at every quiescent point, under arbitrary sequences of operations. These
// tests exercise insert/update/delete/savepoint-rollback paths and assert
// the invariant holds after each operation batch.

// assertActiveCountsInvariant returns true if job_active_counts matches
// ground truth.
func assertActiveCountsInvariant(t *testing.T, ctx context.Context, jobID string) {
	t.Helper()
	var counterSum, truthSum int
	err := testDB.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(count), 0) FROM job_active_counts WHERE job_id = $1`, jobID,
	).Scan(&counterSum)
	if err != nil {
		t.Fatalf("counter query: %v", err)
	}
	err = testDB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_runs WHERE job_id = $1 AND status IN ('dequeued','executing')`, jobID,
	).Scan(&truthSum)
	if err != nil {
		t.Fatalf("truth query: %v", err)
	}
	if counterSum != truthSum {
		t.Fatalf("active counts invariant broken: counter=%d truth=%d", counterSum, truthSum)
	}
}

// assertDLQCountsInvariant checks dlq_counts against job_runs.
func assertDLQCountsInvariant(t *testing.T, ctx context.Context, jobID string) {
	t.Helper()
	var counter, truth int
	err := testDB.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(count), 0) FROM dlq_counts WHERE job_id = $1`, jobID,
	).Scan(&counter)
	if err != nil {
		t.Fatalf("counter: %v", err)
	}
	err = testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM job_runs
		WHERE job_id = $1
		  AND status = 'dead_letter'
		  AND (visible_until IS NULL OR visible_until > NOW())
	`, jobID).Scan(&truth)
	if err != nil {
		t.Fatalf("truth: %v", err)
	}
	if counter != truth {
		t.Fatalf("dlq counts invariant broken: counter=%d truth=%d", counter, truth)
	}
}

func TestTriggerAlgebra_RandomOps(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-trig-prop")
	q := mustQueue(t)

	rng := rand.New(rand.NewPCG(11, 13))
	const ops = 500

	for i := range ops {
		switch rng.IntN(8) {
		case 0, 1: // enqueue (more frequent)
			r := &domain.JobRun{
				ID:        newID(),
				JobID:     job.ID,
				ProjectID: job.ProjectID,
			}
			_ = q.Enqueue(ctx, r)
		case 2: // dequeue
			_, _ = q.DequeueN(ctx, 1+rng.IntN(3))
		case 3: // complete an executing run
			_, _ = testDB.Pool.Exec(ctx,
				`UPDATE job_runs SET status='completed', finished_at=NOW()
				 WHERE id IN (SELECT id FROM job_runs WHERE job_id=$1 AND status IN ('dequeued','executing') LIMIT 1)`,
				job.ID)
		case 4: // fail a queued run to dead_letter
			_, _ = testDB.Pool.Exec(ctx,
				`UPDATE job_runs SET status='dead_letter', finished_at=NOW()
				 WHERE id IN (SELECT id FROM job_runs WHERE job_id=$1 AND status='queued' LIMIT 1)`,
				job.ID)
		case 5: // mask a visible dead_letter row
			_, _ = testDB.Pool.Exec(ctx,
				`UPDATE job_runs SET visible_until=NOW()
				 WHERE id IN (SELECT id FROM job_runs WHERE job_id=$1 AND status='dead_letter' AND visible_until IS NULL LIMIT 1)`,
				job.ID)
		case 6: // hard-delete a terminal row
			_, _ = testDB.Pool.Exec(ctx,
				`DELETE FROM job_runs WHERE id IN (SELECT id FROM job_runs WHERE job_id=$1 AND status='completed' LIMIT 1)`,
				job.ID)
		case 7: // requeue a dead_letter row
			_, _ = testDB.Pool.Exec(ctx,
				`UPDATE job_runs SET status='queued', finished_at=NULL, visible_until=NULL
				 WHERE id IN (SELECT id FROM job_runs WHERE job_id=$1 AND status='dead_letter' LIMIT 1)`,
				job.ID)
		}
		// Invariant check after every op batch.
		if i%20 == 0 {
			assertActiveCountsInvariant(t, ctx, job.ID)
			assertDLQCountsInvariant(t, ctx, job.ID)
		}
	}
	// Final assertion.
	assertActiveCountsInvariant(t, ctx, job.ID)
	assertDLQCountsInvariant(t, ctx, job.ID)
}

func TestTriggerAlgebra_SavepointRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-trig-sp")
	q := mustQueue(t)

	run := mustEnqueueRun(t, ctx, q, job)
	// Baseline: counter is 0 (run is queued, not active).
	assertActiveCountsInvariant(t, ctx, job.ID)

	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	// Move to executing inside a savepoint, then rollback to the savepoint.
	// The counter must not be incremented after rollback.
	if _, err := tx.Exec(ctx, "SAVEPOINT sp1"); err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE job_runs SET status='executing', started_at=NOW() WHERE id=$1`, run.ID,
	); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT sp1"); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Counter should still be 0 (the executing UPDATE was rolled back).
	assertActiveCountsInvariant(t, ctx, job.ID)
}

func TestTriggerAlgebra_InsertThenDeleteInSameTxn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-trig-txn")

	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	runID := newID()
	_, err = tx.Exec(ctx, `
		INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, started_at)
		VALUES ($1, $2, $3, 'executing', 1, 'manual', NOW(), NOW())
	`, runID, job.ID, job.ProjectID)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, err = tx.Exec(ctx, `DELETE FROM job_runs WHERE id=$1`, runID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Net zero effect.
	assertActiveCountsInvariant(t, ctx, job.ID)
}

func TestTriggerAlgebra_MaskDoesNotDoubleCountDLQ(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-trig-mask")

	// Insert a dead_letter row.
	runID := newID()
	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
		VALUES ($1, $2, $3, 'dead_letter', 1, 'manual', NOW(), NOW())
	`, runID, job.ID, job.ProjectID)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	assertDLQCountsInvariant(t, ctx, job.ID)

	// Mask it.
	_, err = testDB.Pool.Exec(ctx, `UPDATE job_runs SET visible_until=NOW() WHERE id=$1`, runID)
	if err != nil {
		t.Fatalf("mask: %v", err)
	}
	assertDLQCountsInvariant(t, ctx, job.ID)

	// And delete it. Counter stays at 0.
	_, err = testDB.Pool.Exec(ctx, `DELETE FROM job_runs WHERE id=$1`, runID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	assertDLQCountsInvariant(t, ctx, job.ID)
}

// TestTriggerAlgebra_ConcurrentSameRowTransitions exercises the race where
// two goroutines attempt compatible status transitions on different rows of
// the same job. The trigger must converge to the correct aggregate.
func TestTriggerAlgebra_ConcurrentSameJobTransitions(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-trig-conc")
	q := mustQueue(t)

	for range 20 {
		mustEnqueueRun(t, ctx, q, job)
	}

	// 4 concurrent workers claim and complete runs.
	errCh := make(chan error, 4)
	for range 4 {
		concWG.Go(func() {
			for range 10 {
				batch, err := q.DequeueN(ctx, 2)
				if err != nil {
					errCh <- err
					return
				}
				for _, r := range batch {
					_, err := testDB.Pool.Exec(ctx,
						`UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id=$1`, r.ID)
					if err != nil {
						errCh <- err
						return
					}
				}
			}
			errCh <- nil
		})
	}
	for range 4 {
		if err := <-errCh; err != nil {
			t.Fatalf("worker: %v", err)
		}
	}
	// Final invariant after the storm.
	assertActiveCountsInvariant(t, ctx, job.ID)
}

// TestTriggerAlgebra_MixedBagInvariantPreserved is the broadest invariant
// check: run a long sequence of random ops and assert the invariant at
// the end. Failure here typically indicates a trigger bug unrelated to
// any specific op.
func TestTriggerAlgebra_MixedBagInvariantPreserved(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	q := mustQueue(t)

	// Three jobs under one project for variety.
	jobs := []*domain.Job{
		mustCreateJob(t, ctx, st, "project-trig-mix"),
	}
	for i := 1; i < 3; i++ {
		jobs = append(jobs, mustCreateJob(t, ctx, st, fmt.Sprintf("project-trig-mix-%d", i)))
	}

	rng := rand.New(rand.NewPCG(1, 2))
	for range 400 {
		job := jobs[rng.IntN(len(jobs))]
		switch rng.IntN(6) {
		case 0:
			r := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
			_ = q.Enqueue(ctx, r)
		case 1:
			_, _ = q.DequeueN(ctx, 1+rng.IntN(2))
		case 2:
			_, _ = testDB.Pool.Exec(ctx,
				`UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id IN (SELECT id FROM job_runs WHERE job_id=$1 AND status IN ('dequeued','executing') LIMIT 1)`,
				job.ID)
		case 3:
			_, _ = testDB.Pool.Exec(ctx,
				`UPDATE job_runs SET status='dead_letter', finished_at=NOW() WHERE id IN (SELECT id FROM job_runs WHERE job_id=$1 AND status='queued' LIMIT 1)`,
				job.ID)
		case 4:
			_, _ = testDB.Pool.Exec(ctx,
				`UPDATE job_runs SET visible_until=NOW() WHERE id IN (SELECT id FROM job_runs WHERE job_id=$1 AND status='dead_letter' AND visible_until IS NULL LIMIT 1)`,
				job.ID)
		case 5:
			_, _ = testDB.Pool.Exec(ctx,
				`DELETE FROM job_runs WHERE id IN (SELECT id FROM job_runs WHERE job_id=$1 AND status='completed' LIMIT 1)`,
				job.ID)
		}
	}
	for _, j := range jobs {
		assertActiveCountsInvariant(t, ctx, j.ID)
		assertDLQCountsInvariant(t, ctx, j.ID)
	}
}
