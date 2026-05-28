//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
)

// Integration tests for the fully-denormalized dequeue path.

func TestDequeueNFullyDenormalized_HappyPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fulldn-happy")
	q := mustQueue(t)

	for range 15 {
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

	for range 5 {
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

func TestDequeueNFullyDenormalized_RespectsMaxConcurrencyPerKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fulldn-key")
	q := mustQueue(t)

	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency_per_key = 1 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set max_concurrency_per_key: %v", err)
	}
	active := &domain.JobRun{
		ID:             newID(),
		JobID:          job.ID,
		ProjectID:      job.ProjectID,
		Status:         domain.StatusExecuting,
		Attempt:        1,
		ConcurrencyKey: "tenant-a",
	}
	if err := st.CreateRun(ctx, active); err != nil {
		t.Fatalf("CreateRun active: %v", err)
	}
	candidate := &domain.JobRun{
		ID:             newID(),
		JobID:          job.ID,
		ProjectID:      job.ProjectID,
		ConcurrencyKey: "tenant-a",
	}
	if err := q.Enqueue(ctx, candidate); err != nil {
		t.Fatalf("Enqueue candidate: %v", err)
	}

	batch, err := q.DequeueNFullyDenormalized(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNFullyDenormalized blocked: %v", err)
	}
	if len(batch) != 0 {
		t.Fatalf("blocked key yielded %d runs, want 0", len(batch))
	}

	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id=$1`, active.ID); err != nil {
		t.Fatalf("complete active: %v", err)
	}
	batch, err = q.DequeueNFullyDenormalized(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNFullyDenormalized unblocked: %v", err)
	}
	if len(batch) != 1 || batch[0].ID != candidate.ID {
		t.Fatalf("unblocked batch = %+v, want candidate %s", batch, candidate.ID)
	}
}

func TestDequeueNFullyDenormalized_RespectsDelayedAndRetryTiming(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fulldn-timing")
	q := mustQueue(t)

	futureSchedule := time.Now().Add(15 * time.Minute)
	futureRetry := time.Now().Add(20 * time.Minute)
	delayed := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, ScheduledAt: &futureSchedule}
	retryBlocked := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, NextRetryAt: &futureRetry}
	due := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	for _, run := range []*domain.JobRun{delayed, retryBlocked, due} {
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue %s: %v", run.ID, err)
		}
	}

	batch, err := q.DequeueNFullyDenormalized(ctx, 10)
	if err != nil {
		t.Fatalf("DequeueNFullyDenormalized: %v", err)
	}
	if len(batch) != 1 || batch[0].ID != due.ID {
		t.Fatalf("batch = %+v, want only due run %s", batch, due.ID)
	}

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_runs
		SET status = 'queued', scheduled_at = NULL, next_retry_at = NULL
		WHERE id = ANY($1)
	`, []string{delayed.ID, retryBlocked.ID}); err != nil {
		t.Fatalf("make delayed/retry due: %v", err)
	}
	batch, err = q.DequeueNFullyDenormalized(ctx, 10)
	if err != nil {
		t.Fatalf("DequeueNFullyDenormalized due: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("due delayed/retry batch len = %d, want 2", len(batch))
	}
}

func BenchmarkDequeueVariants(b *testing.B) {
	for _, tc := range []struct {
		name    string
		dequeue func(context.Context, *testing.B, queueCompat, int) []domain.JobRun
	}{
		{
			name: "legacy",
			dequeue: func(ctx context.Context, b *testing.B, q queueCompat, n int) []domain.JobRun {
				b.Helper()
				runs, err := q.DequeueN(ctx, n)
				if err != nil {
					b.Fatalf("DequeueN: %v", err)
				}
				return runs
			},
		},
		{
			name: "denormalized",
			dequeue: func(ctx context.Context, b *testing.B, q queueCompat, n int) []domain.JobRun {
				b.Helper()
				runs, err := q.DequeueNDenormalized(ctx, n)
				if err != nil {
					b.Fatalf("DequeueNDenormalized: %v", err)
				}
				return runs
			},
		},
		{
			name: "fully_denormalized",
			dequeue: func(ctx context.Context, b *testing.B, q queueCompat, n int) []domain.JobRun {
				b.Helper()
				runs, err := q.DequeueNFullyDenormalized(ctx, n)
				if err != nil {
					b.Fatalf("DequeueNFullyDenormalized: %v", err)
				}
				return runs
			},
		},
	} {
		b.Run(tc.name, func(b *testing.B) {
			ctx := context.Background()
			if err := testDB.CleanTables(ctx); err != nil {
				b.Fatalf("CleanTables() error = %v", err)
			}
			st := store.New(testDB.Pool)
			job := createBenchmarkJob(b, ctx, st, "project-bench-"+tc.name)
			q := queue.NewPostgresQueue(testDB.Pool)
			preloadQueueBenchmarkRuns(b, ctx, q, job, 512)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				runs := tc.dequeue(ctx, b, q, 32)
				if len(runs) == 0 {
					b.StopTimer()
					preloadQueueBenchmarkRuns(b, ctx, q, job, 256)
					b.StartTimer()
				}
			}
		})
	}
}

type queueCompat interface {
	DequeueN(context.Context, int) ([]domain.JobRun, error)
	DequeueNDenormalized(context.Context, int) ([]domain.JobRun, error)
	DequeueNFullyDenormalized(context.Context, int) ([]domain.JobRun, error)
	Enqueue(context.Context, *domain.JobRun) error
}

func createBenchmarkJob(tb testing.TB, ctx context.Context, st *store.Queries, projectID string) *domain.Job {
	tb.Helper()
	job := &domain.Job{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "bench-job-" + newID(),
		Slug:        "bench-job-" + newID(),
		EndpointURL: "https://example.com/queue-job",
		MaxAttempts: 3,
		TimeoutSecs: 300,
		Enabled:     true,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		tb.Fatalf("CreateJob() error = %v", err)
	}
	return job
}

func preloadQueueBenchmarkRuns(tb testing.TB, ctx context.Context, q queueCompat, job *domain.Job, n int) {
	tb.Helper()
	for i := 0; i < n; i++ {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: i % 10}
		if err := q.Enqueue(ctx, run); err != nil {
			tb.Fatalf("Enqueue benchmark run: %v", err)
		}
	}
}
