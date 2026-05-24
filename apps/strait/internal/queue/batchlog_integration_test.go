//go:build integration

package queue_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
)

func mustBatchlogQueue(t *testing.T, lease time.Duration) *queue.BatchlogQueue {
	t.Helper()
	return queue.NewBatchlogQueue(testDB.Pool, queue.NewPostgresQueue(testDB.Pool), queue.BatchlogConfig{
		TickInterval:  10 * time.Millisecond,
		LeaseDuration: lease,
		LeaseOwner:    "test-worker-" + newID(),
	})
}

func TestBatchlog_NoDuplicateClaimsUnderConcurrentWorkers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-batchlog-concurrent")
	q := mustBatchlogQueue(t, time.Second)

	for i := 0; i < 50; i++ {
		if err := q.Enqueue(ctx, &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}
	if _, err := q.SealDueBatches(ctx); err != nil {
		t.Fatalf("SealDueBatches: %v", err)
	}

	seen := sync.Map{}
	var wg sync.WaitGroup
	errCh := make(chan error, 5)
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runs, err := q.DequeueN(ctx, 10)
			if err != nil {
				errCh <- err
				return
			}
			for _, run := range runs {
				if _, loaded := seen.LoadOrStore(run.ID, true); loaded {
					errCh <- errDuplicateClaim{runID: run.ID}
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent dequeue: %v", err)
	}
}

func TestStaleLeaseBatchlog_RedeliversBeforeStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-batchlog-redeliver")
	q := mustBatchlogQueue(t, 15*time.Millisecond)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := q.SealDueBatches(ctx); err != nil {
		t.Fatalf("SealDueBatches: %v", err)
	}
	first, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("first DequeueN: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first DequeueN len = %d, want 1", len(first))
	}
	time.Sleep(30 * time.Millisecond)
	second, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("second DequeueN before reclaim: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("second DequeueN before reclaim len = %d, want 0", len(second))
	}
	if _, err := q.ReclaimExpiredLeases(ctx); err != nil {
		t.Fatalf("ReclaimExpiredLeases: %v", err)
	}
	second, err = q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("second DequeueN after reclaim: %v", err)
	}
	if len(second) != 1 || second[0].ID != run.ID {
		t.Fatalf("redelivered = %+v, want run %s", second, run.ID)
	}
}

func TestQueueEngine_BatchlogQueuedToExecutingDirectTransition(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-batchlog-direct")
	q := mustBatchlogQueue(t, time.Second)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := q.SealDueBatches(ctx); err != nil {
		t.Fatalf("SealDueBatches: %v", err)
	}
	claimed, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Status != domain.StatusQueued {
		t.Fatalf("claimed = %+v, want one virtually dequeued queued run", claimed)
	}
	if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusExecuting, map[string]any{
		"started_at": time.Now(),
	}); err != nil {
		t.Fatalf("queued->executing: %v", err)
	}
	var status string
	if err := testDB.Pool.QueryRow(ctx, `SELECT status FROM queue_entries WHERE run_id = $1`, run.ID).Scan(&status); err != nil {
		t.Fatalf("queue entry status: %v", err)
	}
	if status != "acked" {
		t.Fatalf("queue entry status = %q, want acked", status)
	}
}

func TestBatchlog_BackfillDoesNotDuplicateExistingQueuedRuns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-batchlog-backfill")
	legacy := queue.NewPostgresQueue(testDB.Pool)
	q := mustBatchlogQueue(t, time.Second)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := legacy.Enqueue(ctx, run); err != nil {
		t.Fatalf("legacy Enqueue: %v", err)
	}
	if _, err := q.BackfillDue(ctx); err != nil {
		t.Fatalf("BackfillDue first: %v", err)
	}
	if _, err := q.BackfillDue(ctx); err != nil {
		t.Fatalf("BackfillDue second: %v", err)
	}
	var count int
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM queue_entries WHERE run_id = $1`, run.ID).Scan(&count); err != nil {
		t.Fatalf("count queue entries: %v", err)
	}
	if count != 1 {
		t.Fatalf("queue entry count = %d, want 1", count)
	}
}

func TestQueueEntryBatchlog_CreatedAtWriteTimeForLegacyEnqueue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-batchlog-write-time")
	legacy := queue.NewPostgresQueue(testDB.Pool)
	q := mustBatchlogQueue(t, time.Second)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 9}
	if err := legacy.Enqueue(ctx, run); err != nil {
		t.Fatalf("legacy Enqueue: %v", err)
	}

	var status string
	var batchID *int64
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT status, batch_id
		FROM queue_entries
		WHERE run_id = $1
	`, run.ID).Scan(&status, &batchID); err != nil {
		t.Fatalf("queue entry from trigger: %v", err)
	}
	if status != "ready" {
		t.Fatalf("queue entry status = %q, want ready", status)
	}
	if batchID != nil {
		t.Fatalf("queue entry batch_id = %v, want nil before seal", *batchID)
	}

	if _, err := q.SealDueBatches(ctx); err != nil {
		t.Fatalf("SealDueBatches: %v", err)
	}
	claimed, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != run.ID {
		t.Fatalf("claimed = %+v, want run %s", claimed, run.ID)
	}
}

func TestQueueEntryBatchlog_CreatedWhenDelayedRunPromotesToQueued(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-batchlog-delay-trigger")
	legacy := queue.NewPostgresQueue(testDB.Pool)

	future := time.Now().Add(time.Hour)
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, ScheduledAt: &future}
	if err := legacy.Enqueue(ctx, run); err != nil {
		t.Fatalf("legacy delayed Enqueue: %v", err)
	}
	var count int
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM queue_entries WHERE run_id = $1`, run.ID).Scan(&count); err != nil {
		t.Fatalf("count pre-promotion queue entries: %v", err)
	}
	if count != 0 {
		t.Fatalf("pre-promotion queue entry count = %d, want 0", count)
	}

	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status = 'queued', scheduled_at = NULL WHERE id = $1`, run.ID); err != nil {
		t.Fatalf("promote delayed run: %v", err)
	}
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM queue_entries WHERE run_id = $1`, run.ID).Scan(&count); err != nil {
		t.Fatalf("count post-promotion queue entries: %v", err)
	}
	if count != 1 {
		t.Fatalf("post-promotion queue entry count = %d, want 1", count)
	}
}

func TestDelayedBatchlog_RunBecomesClaimableAtRightTime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-batchlog-delayed")
	q := mustBatchlogQueue(t, time.Second)

	future := time.Now().Add(50 * time.Millisecond)
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, ScheduledAt: &future}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue delayed: %v", err)
	}
	early, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("early DequeueN: %v", err)
	}
	if len(early) != 0 {
		t.Fatalf("early DequeueN len = %d, want 0", len(early))
	}
	time.Sleep(75 * time.Millisecond)
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status = 'queued', scheduled_at = NULL WHERE id = $1`, run.ID); err != nil {
		t.Fatalf("promote delayed: %v", err)
	}
	if _, err := q.SealDueBatches(ctx); err != nil {
		t.Fatalf("SealDueBatches after delay: %v", err)
	}
	due, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("due DequeueN: %v", err)
	}
	if len(due) != 1 || due[0].ID != run.ID {
		t.Fatalf("due = %+v, want run %s", due, run.ID)
	}
}

func BenchmarkBatchlog(b *testing.B) {
	ctx := context.Background()
	if err := testDB.CleanTables(ctx); err != nil {
		b.Fatalf("CleanTables() error = %v", err)
	}
	st := mustStoreBenchmark(b)
	job := mustCreateJobBenchmark(b, ctx, st, "project-batchlog-benchmark")
	q := queue.NewBatchlogQueue(testDB.Pool, queue.NewPostgresQueue(testDB.Pool), queue.BatchlogConfig{
		TickInterval:  10 * time.Millisecond,
		LeaseDuration: time.Second,
		LeaseOwner:    "benchmark-" + newID(),
	})
	preloadBatchlogRuns(b, ctx, q, job, 512)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runs, err := q.DequeueN(ctx, 32)
		if err != nil {
			b.Fatalf("DequeueN: %v", err)
		}
		if len(runs) == 0 {
			b.StopTimer()
			preloadBatchlogRuns(b, ctx, q, job, 256)
			b.StartTimer()
		}
	}
}

func TestBatchlog_DequeueDoesNotSealOrReclaimOnHotPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-batchlog-hotpath")
	q := mustBatchlogQueue(t, 15*time.Millisecond)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	unsealed, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN unsealed: %v", err)
	}
	if len(unsealed) != 0 {
		t.Fatalf("DequeueN unsealed len = %d, want 0", len(unsealed))
	}
	if _, err := q.SealDueBatches(ctx); err != nil {
		t.Fatalf("SealDueBatches: %v", err)
	}
	claimed, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN sealed: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("DequeueN sealed len = %d, want 1", len(claimed))
	}

	time.Sleep(30 * time.Millisecond)
	stillLeased, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN expired unreclaimed: %v", err)
	}
	if len(stillLeased) != 0 {
		t.Fatalf("DequeueN expired unreclaimed len = %d, want 0", len(stillLeased))
	}
	if _, err := q.ReclaimExpiredLeases(ctx); err != nil {
		t.Fatalf("ReclaimExpiredLeases: %v", err)
	}
	redelivered, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN reclaimed: %v", err)
	}
	if len(redelivered) != 1 || redelivered[0].ID != run.ID {
		t.Fatalf("redelivered = %+v, want run %s", redelivered, run.ID)
	}
}

func TestBatchlogLeaseCounts_BlockMaxConcurrencyBeforeStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-batchlog-lease-max")
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency = 1 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set max_concurrency: %v", err)
	}
	q := mustBatchlogQueue(t, time.Second)

	for range 2 {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	if _, err := q.SealDueBatches(ctx); err != nil {
		t.Fatalf("SealDueBatches: %v", err)
	}
	first, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("first DequeueN: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first DequeueN len = %d, want 1", len(first))
	}
	second, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("second DequeueN: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("second DequeueN len = %d, want 0 while first run is leased", len(second))
	}

	var leaseCount int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COALESCE(count, 0)
		FROM job_batchlog_lease_counts
		WHERE job_id = $1 AND concurrency_key = ''
	`, job.ID).Scan(&leaseCount); err != nil {
		t.Fatalf("lease count query: %v", err)
	}
	if leaseCount != 1 {
		t.Fatalf("lease count = %d, want 1", leaseCount)
	}
}

func TestBatchlogLeaseCounts_BlockMaxConcurrencyPerKeyBeforeStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-batchlog-lease-key")
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency_per_key = 1 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set max_concurrency_per_key: %v", err)
	}
	q := mustBatchlogQueue(t, time.Second)

	for _, key := range []string{"tenant-a", "tenant-a", "tenant-b"} {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, ConcurrencyKey: key}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue %s: %v", key, err)
		}
	}
	if _, err := q.SealDueBatches(ctx); err != nil {
		t.Fatalf("SealDueBatches: %v", err)
	}
	first, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("first DequeueN: %v", err)
	}
	if len(first) != 1 || first[0].ConcurrencyKey != "tenant-a" {
		t.Fatalf("first = %+v, want tenant-a", first)
	}
	second, err := q.DequeueN(ctx, 2)
	if err != nil {
		t.Fatalf("second DequeueN: %v", err)
	}
	if len(second) != 1 || second[0].ConcurrencyKey != "tenant-b" {
		t.Fatalf("second = %+v, want only tenant-b while tenant-a is leased", second)
	}
}

type errDuplicateClaim struct {
	runID string
}

func (e errDuplicateClaim) Error() string {
	return "duplicate claim for " + e.runID
}

func mustStoreBenchmark(tb testing.TB) *store.Queries {
	tb.Helper()
	if testDB == nil || testDB.Pool == nil {
		tb.Fatalf("testDB is not initialized")
	}
	return store.New(testDB.Pool)
}

func mustCreateJobBenchmark(tb testing.TB, ctx context.Context, st *store.Queries, projectID string) *domain.Job {
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

func preloadBatchlogRuns(tb testing.TB, ctx context.Context, q *queue.BatchlogQueue, job *domain.Job, n int) {
	tb.Helper()
	for i := 0; i < n; i++ {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: i % 10}
		if err := q.Enqueue(ctx, run); err != nil {
			tb.Fatalf("Enqueue benchmark run: %v", err)
		}
	}
	if _, err := q.SealDueBatches(ctx); err != nil {
		tb.Fatalf("SealDueBatches benchmark runs: %v", err)
	}
}
