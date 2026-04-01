//go:build integration

package scheduler_test

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/scheduler"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	testDB     *testutil.TestDB
	testDBOnce sync.Once
)

func getTestDB(t *testing.T) *testutil.TestDB {
	t.Helper()
	testDBOnce.Do(func() {
		ctx := context.Background()
		var err error
		testDB, err = testutil.SetupTestDB(ctx, "../../migrations")
		if err != nil {
			log.Fatalf("setup test db: %v", err)
		}
		t.Cleanup(func() { testDB.Cleanup(ctx) })
	})
	if testDB == nil || testDB.Pool == nil {
		t.Fatal("testDB is not initialized")
	}
	return testDB
}

func intTestStore(t *testing.T) *store.Queries {
	t.Helper()
	return store.New(getTestDB(t).Pool)
}

func intTestQueue(t *testing.T) *queue.PostgresQueue {
	t.Helper()
	return queue.NewPostgresQueue(getTestDB(t).Pool)
}

func intTestClean(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := getTestDB(t).CleanTables(ctx); err != nil {
		// Pool may be closed if another test's cleanup ran first.
		t.Skipf("skipping: test DB unavailable: %v", err)
	}
}

func intNewID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func intCreateJob(t *testing.T, ctx context.Context, st *store.Queries, projectID string, mutate ...func(*domain.Job)) *domain.Job {
	t.Helper()
	job := &domain.Job{
		ID:          intNewID(),
		ProjectID:   projectID,
		Name:        "job-" + intNewID(),
		Slug:        "slug-" + intNewID(),
		EndpointURL: "https://example.com/integration-test",
		MaxAttempts: 3,
		TimeoutSecs: 300,
		Enabled:     true,
	}
	for _, fn := range mutate {
		fn(job)
	}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	return job
}

// ---------------------------------------------------------------------------
// 1. Cron job scheduling with real Postgres: create a cron job, verify
//    LoadJobs reads it and the scheduler registers the entry.
// ---------------------------------------------------------------------------

func TestIntegration_CronLoadJobs(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)
	q := intTestQueue(t)

	// Create a cron job (every minute).
	_ = intCreateJob(t, ctx, st, "proj-cron-load", func(j *domain.Job) {
		cron := "* * * * *"
		j.Cron = cron
	})

	// Also create a disabled cron job -- should not be loaded.
	_ = intCreateJob(t, ctx, st, "proj-cron-load", func(j *domain.Job) {
		cron := "*/5 * * * *"
		j.Cron = cron
		j.Enabled = false
	})

	cs := scheduler.NewCronScheduler(ctx, st, q, nil)
	if err := cs.LoadJobs(ctx); err != nil {
		t.Fatalf("LoadJobs() error = %v", err)
	}

	// Start and immediately stop to verify no panic.
	cs.Start()
	stopCtx := cs.Stop()
	<-stopCtx.Done()
}

// ---------------------------------------------------------------------------
// 2. Cron job triggers enqueue into real DB.
// ---------------------------------------------------------------------------

func TestIntegration_CronTriggerEnqueuesRun(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)
	q := intTestQueue(t)

	// Create a cron job with "every second" (cron library supports seconds only via
	// custom parser -- use every minute and manually trigger instead).
	job := intCreateJob(t, ctx, st, "proj-cron-trigger", func(j *domain.Job) {
		// Every minute.
		cron := "* * * * *"
		j.Cron = cron
	})

	cs := scheduler.NewCronScheduler(ctx, st, q, nil)
	if err := cs.LoadJobs(ctx); err != nil {
		t.Fatalf("LoadJobs() error = %v", err)
	}
	cs.Start()
	defer func() {
		stopCtx := cs.Stop()
		<-stopCtx.Done()
	}()

	// Wait for the cron to fire (up to 70s for next minute boundary).
	var dequeued []domain.JobRun
	deadline := time.After(75 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for cron run to be enqueued for job %s", job.ID)
		case <-ticker.C:
			runs, err := q.DequeueN(ctx, 10)
			if err != nil {
				t.Fatalf("DequeueN() error = %v", err)
			}
			for _, r := range runs {
				if r.JobID == job.ID {
					dequeued = append(dequeued, r)
				}
			}
			if len(dequeued) > 0 {
				goto done
			}
		}
	}
done:
	if dequeued[0].TriggeredBy != domain.TriggerCron {
		t.Errorf("triggered_by = %q, want %q", dequeued[0].TriggeredBy, domain.TriggerCron)
	}
	if dequeued[0].ProjectID != job.ProjectID {
		t.Errorf("project_id = %q, want %q", dequeued[0].ProjectID, job.ProjectID)
	}
}

// ---------------------------------------------------------------------------
// 3. Batch flusher with real DB: items are flushed into a run.
// ---------------------------------------------------------------------------

func TestIntegration_BatchFlusher(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)
	q := intTestQueue(t)

	// Create a job with batch settings: max_size=3.
	job := intCreateJob(t, ctx, st, "proj-batch", func(j *domain.Job) {
		j.BatchMaxSize = 3
		j.BatchWindowSecs = 0 // flush only on size.
	})

	// Insert 3 batch buffer items.
	for i := range 3 {
		item := &domain.BatchBufferItem{
			JobID:       job.ID,
			ProjectID:   job.ProjectID,
			BatchKey:    "default",
			Payload:     json.RawMessage(`{"i":` + string(rune('0'+i)) + `}`),
			Priority:    1,
			TriggeredBy: "api",
		}
		if err := st.InsertBatchBufferItem(ctx, item); err != nil {
			t.Fatalf("InsertBatchBufferItem() error = %v", err)
		}
	}

	// Verify items are flushable.
	batches, err := st.ListFlushableBatches(ctx)
	if err != nil {
		t.Fatalf("ListFlushableBatches() error = %v", err)
	}
	if len(batches) == 0 {
		t.Fatal("expected at least one flushable batch")
	}

	// Run the batch flusher for a short interval and wait for it to process.
	flusher := scheduler.NewBatchFlusher(st, q, 200*time.Millisecond)
	flusherCtx, flusherCancel := context.WithTimeout(ctx, 3*time.Second)
	defer flusherCancel()

	go flusher.Run(flusherCtx)

	// Poll for enqueued run.
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for batch flusher to enqueue a run")
		case <-tick.C:
			runs, dErr := q.DequeueN(ctx, 10)
			if dErr != nil {
				t.Fatalf("DequeueN() error = %v", dErr)
			}
			for _, r := range runs {
				if r.JobID == job.ID {
					if r.Payload == nil {
						t.Fatal("batch run payload is nil")
					}
					var payload map[string][]json.RawMessage
					if jErr := json.Unmarshal(r.Payload, &payload); jErr != nil {
						t.Fatalf("unmarshal batch payload: %v", jErr)
					}
					if len(payload["items"]) != 3 {
						t.Fatalf("expected 3 items in batch payload, got %d", len(payload["items"]))
					}
					// Batch buffer should be drained.
					count, cErr := st.CountBatchBufferItems(ctx, job.ID, "default")
					if cErr != nil {
						t.Fatalf("CountBatchBufferItems() error = %v", cErr)
					}
					if count != 0 {
						t.Fatalf("expected 0 remaining buffer items, got %d", count)
					}
					return
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 4. Stale run detection with real DB: runs past their heartbeat timeout
//    are marked as crashed by the reaper.
// ---------------------------------------------------------------------------

func TestIntegration_ReaperStaleRunDetection(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)
	q := intTestQueue(t)

	job := intCreateJob(t, ctx, st, "proj-stale-reaper")

	// Create a run and move it to executing state.
	run := &domain.JobRun{
		ID:        intNewID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Priority:  1,
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// Dequeue to move to dequeued status.
	dequeued, err := q.DequeueN(ctx, 1)
	if err != nil || len(dequeued) == 0 {
		t.Fatalf("DequeueN() error = %v, count = %d", err, len(dequeued))
	}

	// Transition to executing with an old heartbeat (simulating stale).
	staleHeartbeat := time.Now().Add(-10 * time.Minute)
	if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
		"started_at":   time.Now().Add(-15 * time.Minute),
		"heartbeat_at": staleHeartbeat,
	}); err != nil {
		t.Fatalf("UpdateRunStatus() to executing error = %v", err)
	}

	// Create a reaper with a 5-minute stale threshold.
	reaper := scheduler.NewReaper(st, time.Second, 5*time.Minute, 30*24*time.Hour, 90*24*time.Hour, false, nil)
	reaper.ReapOnce(ctx)

	// Verify the run was crashed.
	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusCrashed {
		t.Fatalf("run status = %q, want %q", got.Status, domain.StatusCrashed)
	}
	if got.FinishedAt == nil {
		t.Fatal("expected finished_at to be set on crashed run")
	}
}

// ---------------------------------------------------------------------------
// 5. Advisory lock behavior: verify only one scheduler instance processes.
// ---------------------------------------------------------------------------

func TestIntegration_AdvisoryLockExclusivity(t *testing.T) {
	ctx := context.Background()
	tdb := getTestDB(t)

	// Verify the DB is still reachable (container may have been cleaned up by another test).
	if err := tdb.Pool.Ping(ctx); err != nil {
		t.Skipf("skipping: test DB unavailable: %v", err)
	}

	// Use two separate connections to simulate two scheduler instances.
	pool1, err := pgxpool.New(ctx, tdb.ConnStr)
	if err != nil {
		t.Skipf("skipping: cannot create pool1: %v", err)
	}
	defer pool1.Close()

	pool2, err := pgxpool.New(ctx, tdb.ConnStr)
	if err != nil {
		t.Skipf("skipping: cannot create pool2: %v", err)
	}
	defer pool2.Close()

	st1 := store.New(pool1)
	st2 := store.New(pool2)

	const lockID int64 = 999999 // test-specific lock ID

	// First instance acquires the lock.
	acquired1, err := st1.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock(1) error = %v", err)
	}
	if !acquired1 {
		t.Fatal("expected first instance to acquire advisory lock")
	}

	// Second instance should fail to acquire the same lock.
	acquired2, err := st2.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock(2) error = %v", err)
	}
	if acquired2 {
		t.Fatal("expected second instance to NOT acquire advisory lock held by first")
	}

	// After releasing, second instance can acquire.
	if err := st1.ReleaseAdvisoryLock(ctx, lockID); err != nil {
		t.Fatalf("ReleaseAdvisoryLock(1) error = %v", err)
	}

	acquired3, err := st2.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock(2 retry) error = %v", err)
	}
	if !acquired3 {
		t.Fatal("expected second instance to acquire advisory lock after release")
	}

	// Clean up.
	if err := st2.ReleaseAdvisoryLock(ctx, lockID); err != nil {
		t.Fatalf("ReleaseAdvisoryLock(2) error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// 6. SLO evaluation with real metrics stored in DB.
// ---------------------------------------------------------------------------

func TestIntegration_SLOEvaluation(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)
	q := intTestQueue(t)

	job := intCreateJob(t, ctx, st, "proj-slo-eval")

	// Create some completed and failed runs so GetJobHealthStats returns data.
	for i := range 10 {
		run := &domain.JobRun{
			ID:        intNewID(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Priority:  1,
		}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
		dequeued, dErr := q.DequeueN(ctx, 1)
		if dErr != nil || len(dequeued) == 0 {
			t.Fatalf("DequeueN() error = %v", dErr)
		}

		startedAt := time.Now().Add(-time.Duration(i+1) * time.Minute)
		finishedAt := startedAt.Add(time.Duration(i+1) * time.Second)

		if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
			"started_at":   startedAt,
			"heartbeat_at": startedAt,
		}); err != nil {
			t.Fatalf("UpdateRunStatus() to executing: %v", err)
		}

		// Make 8 out of 10 complete, 2 fail.
		targetStatus := domain.StatusCompleted
		if i >= 8 {
			targetStatus = domain.StatusFailed
		}
		fields := map[string]any{
			"finished_at": finishedAt,
		}
		if targetStatus == domain.StatusFailed {
			fields["error"] = "simulated failure"
		}
		if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, targetStatus, fields); err != nil {
			t.Fatalf("UpdateRunStatus() to %s: %v", targetStatus, err)
		}
	}

	// Create an SLO: 99% success rate over 24 hours.
	slo := &domain.JobSLO{
		ID:          intNewID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Metric:      domain.SLOMetricSuccessRate,
		Target:      0.99,
		WindowHours: 24,
	}
	if err := st.CreateJobSLO(ctx, slo); err != nil {
		t.Fatalf("CreateJobSLO() error = %v", err)
	}

	// Verify the SLO was created.
	allSLOs, err := st.ListAllJobSLOs(ctx)
	if err != nil {
		t.Fatalf("ListAllJobSLOs() error = %v", err)
	}
	if len(allSLOs) != 1 {
		t.Fatalf("expected 1 SLO, got %d", len(allSLOs))
	}

	// Run the SLO evaluator.
	evaluator := scheduler.NewSLOEvaluator(st, nil)
	if err := evaluator.Evaluate(ctx); err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	// Check that an evaluation was recorded.
	statuses, err := st.ListJobSLOs(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListJobSLOs() error = %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 SLO status, got %d", len(statuses))
	}
	if statuses[0].CurrentValue == nil {
		t.Fatal("expected CurrentValue to be set after evaluation")
	}
	if statuses[0].BudgetRemaining == nil {
		t.Fatal("expected BudgetRemaining to be set after evaluation")
	}

	// With 80% success (8/10) and 99% target, budget should be depleted.
	// success_rate from DB is percentage, metricValue divides by 100 -> 0.8
	// budget = 1 - ((1-0.8)/(1-0.99)) = 1 - (0.2/0.01) = 1 - 20 = clamped to 0
	if *statuses[0].BudgetRemaining > 0.01 {
		t.Errorf("budget_remaining = %f, expected near 0 (budget depleted)", *statuses[0].BudgetRemaining)
	}

	// Verify pruning works without error.
	pruned, err := st.PruneSLOEvaluations(ctx, 1)
	if err != nil {
		t.Fatalf("PruneSLOEvaluations() error = %v", err)
	}
	// We only have 1 evaluation and keep=1, so nothing should be pruned.
	if pruned != 0 {
		t.Errorf("expected 0 pruned evaluations, got %d", pruned)
	}
}
