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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		testDB, err = testutil.SetupSharedTestDB(ctx, "../../migrations", "scheduler-external")
		if err != nil {
			log.Fatalf("setup test db: %v", err)
		}
		// Do NOT use t.Cleanup here. The sync.Once means cleanup would fire when
		// the first test finishes, destroying the container for all other tests.
		// Testcontainers Reaper handles container cleanup at process exit.
	})
	require.False(t, testDB ==
		nil ||
		testDB.
			Pool ==
			nil)

	return testDB
}

func intTestStore(t *testing.T) *store.Queries {
	t.Helper()
	return store.New(getTestDB(t).Pool)
}

func intTestQueue(t *testing.T) *queue.PgQueQueue {
	t.Helper()
	db := getTestDB(t).Pool
	q := queue.NewPgQueQueue(db, queue.NewPostgresRunWriter(db), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "scheduler-" + intNewID(),
		ReceiveWindow: 100,
	})
	tickerCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go q.RunTicker(tickerCtx)
	return q
}

func intTestClean(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(t, getTestDB(t).CleanTables(ctx))

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
	require.NoError(t, st.CreateJob(ctx,
		job))

	return job
}

func intClaimRuns(t *testing.T, ctx context.Context, q *queue.PgQueQueue, want int) []domain.JobRun {
	t.Helper()
	claimed := make([]domain.JobRun, 0, want)
	deadline := time.Now().Add(5 * time.Second)
	for len(claimed) < want && time.Now().Before(deadline) {
		require.NoError(t, q.ForceTick(ctx, "http"))

		runs, err := q.DequeueN(ctx, want-len(claimed))
		require.NoError(t, err)

		claimed = append(claimed, runs...)
		if len(claimed) < want {
			time.Sleep(20 * time.Millisecond)
		}
	}
	require.Len(t, claimed, want)

	return claimed
}

// 1. Cron job scheduling with real Postgres: create a cron job, verify
//    LoadJobs reads it and the scheduler registers the entry.

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
	require.NoError(t, cs.LoadJobs(ctx))

	// Start and immediately stop to verify no panic.
	cs.Start()
	stopCtx := cs.Stop()
	<-stopCtx.Done()
}

// 2. Cron job triggers enqueue into real DB.

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
	require.NoError(t, cs.LoadJobs(ctx))

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
			require.Failf(t, "test failure", "timed out waiting for cron run to be enqueued for job %s", job.ID)
		case <-ticker.C:
			runs, err := q.DequeueN(ctx, 10)
			if err != nil {
				require.Failf(t, "test failure",

					"DequeueN() error = %v", err)
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
		assert.Failf(t, "test failure",

			"triggered_by = %q, want %q", dequeued[0].TriggeredBy, domain.TriggerCron)
	}
	assert.Equal(t, job.ProjectID,

		dequeued[0].ProjectID,
	)

}

// 3. Batch flusher with real DB: items are flushed into a run.

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
		require.NoError(t, st.InsertBatchBufferItem(ctx,
			item))

	}

	// Verify items are flushable.
	batches, err := st.ListFlushableBatches(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, batches)

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
			require.Fail(t, "timed out waiting for batch flusher to enqueue a run")
		case <-tick.C:
			runs, dErr := q.DequeueN(ctx, 10)
			if dErr != nil {
				require.Failf(t, "test failure",

					"DequeueN() error = %v", dErr)
			}
			for _, r := range runs {
				if r.JobID == job.ID {
					require.NotNil(t, r.Payload)

					var payload map[string][]json.RawMessage
					require.Nil(t, json.
						Unmarshal(
							r.Payload,
							&payload))
					require.Len(t, payload["items"], 3)

					// Batch buffer should be drained.
					count, cErr := st.CountBatchBufferItems(ctx, job.ID, "default")
					require.Nil(t, cErr)
					require.EqualValues(t, 0, count)

					return
				}
			}
		}
	}
}

// 4. Stale run detection with real DB: runs past their heartbeat timeout
//    retry while attempts remain, then crash once attempts are exhausted.

func TestIntegration_ReaperStaleRunDetection_RetriesWhenAttemptsRemain(t *testing.T) {
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
	require.NoError(t, q.Enqueue(ctx, run))

	// Dequeue to move to dequeued status.
	_ = intClaimRuns(t, ctx, q, 1)

	// Transition to executing with an old heartbeat (simulating stale).
	staleHeartbeat := time.Now().Add(-10 * time.Minute)
	startedAt := time.Now().Add(-15 * time.Minute)
	if _, err := getTestDB(t).Pool.Exec(ctx, `
		UPDATE job_run_state
		SET status = $2, started_at = $3, heartbeat_at = $4, finished_at = NULL, updated_at = NOW()
		WHERE run_id = $1
	`, run.ID, domain.StatusExecuting, startedAt, staleHeartbeat); err != nil {
		require.Failf(t, "test failure",

			"seed stale executing state: %v", err)
	}
	if _, err := getTestDB(t).Pool.Exec(ctx, `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		VALUES ($1, $2, FALSE)
	`, run.ID, staleHeartbeat); err != nil {
		require.Failf(t, "test failure",

			"age heartbeat row: %v", err)
	}

	// Create a reaper with a 5-minute stale threshold.
	reaper := scheduler.NewReaper(st, time.Second, 5*time.Minute, 30*24*time.Hour, 90*24*time.Hour, false, nil)
	reaper.ReapOnce(ctx)

	// Verify the stale execution was requeued instead of terminally crashed.
	got, err := st.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusQueued,

		got.Status,
	)
	require.EqualValues(t, 2, got.Attempt)
	require.Nil(t, got.FinishedAt)

	var retryAttempt int
	var nextRetryAt time.Time
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`
		SELECT attempt, next_retry_at
		FROM job_retries
		WHERE run_id = $1 AND cleared = FALSE
		ORDER BY id DESC
		LIMIT 1`,

			run.ID,
		).Scan(&retryAttempt, &nextRetryAt))
	require.EqualValues(t, 2, retryAttempt)
	require.True(t, nextRetryAt.
		After(time.
			Now().Add(-time.Second)))

}

func TestIntegration_ReaperStaleRunDetection_CrashesWhenAttemptsExhausted(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)
	q := intTestQueue(t)

	job := intCreateJob(t, ctx, st, "proj-stale-reaper-exhausted", func(j *domain.Job) {
		j.MaxAttempts = 1
	})

	run := &domain.JobRun{
		ID:        intNewID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Priority:  1,
	}
	require.NoError(t, q.Enqueue(ctx, run))

	_ = intClaimRuns(t, ctx, q, 1)

	staleHeartbeat := time.Now().Add(-10 * time.Minute)
	startedAt := time.Now().Add(-15 * time.Minute)
	if _, err := getTestDB(t).Pool.Exec(ctx, `
		UPDATE job_run_state
		SET status = $2, started_at = $3, heartbeat_at = $4, finished_at = NULL, updated_at = NOW()
		WHERE run_id = $1
	`, run.ID, domain.StatusExecuting, startedAt, staleHeartbeat); err != nil {
		require.Failf(t, "test failure",

			"seed stale executing state: %v", err)
	}
	if _, err := getTestDB(t).Pool.Exec(ctx, `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		VALUES ($1, $2, FALSE)
	`, run.ID, staleHeartbeat); err != nil {
		require.Failf(t, "test failure",

			"age heartbeat row: %v", err)
	}

	reaper := scheduler.NewReaper(st, time.Second, 5*time.Minute, 30*24*time.Hour, 90*24*time.Hour, false, nil)
	reaper.ReapOnce(ctx)

	got, err := st.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusCrashed,

		got.Status,
	)
	require.NotNil(t, got.FinishedAt)

}

// 5. Advisory lock behavior: verify only one scheduler instance processes.

func TestIntegration_AdvisoryLockExclusivity(t *testing.T) {
	ctx := context.Background()
	tdb := getTestDB(t)

	// Use two separate connections to simulate two scheduler instances.
	pool1, err := pgxpool.New(ctx, tdb.ConnStr)
	require.NoError(t, err)

	defer pool1.Close()

	pool2, err := pgxpool.New(ctx, tdb.ConnStr)
	require.NoError(t, err)

	defer pool2.Close()

	st1 := store.New(pool1)
	st2 := store.New(pool2)

	const lockID int64 = 999999 // test-specific lock ID

	// First instance acquires the lock.
	acquired1, err := st1.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.True(t, acquired1)

	// Second instance should fail to acquire the same lock.
	acquired2, err := st2.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.False(t, acquired2)
	require.NoError(t, st1.ReleaseAdvisoryLock(ctx,
		lockID))

	// After releasing, second instance can acquire.

	acquired3, err := st2.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.True(t, acquired3)
	require.NoError(t, st2.ReleaseAdvisoryLock(ctx,
		lockID))

	// Clean up.

}

// 6. SLO evaluation with real metrics stored in DB.

func TestIntegration_SLOEvaluation(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)

	job := intCreateJob(t, ctx, st, "proj-slo-eval")

	// Create some completed and failed runs so GetJobHealthStats returns data.
	for i := range 10 {
		run := &domain.JobRun{
			ID:        intNewID(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Priority:  1,
		}

		startedAt := time.Now().Add(-time.Duration(i+1) * time.Minute)
		finishedAt := startedAt.Add(time.Duration(i+1) * time.Second)

		// Make 8 out of 10 complete, 2 fail.
		targetStatus := domain.StatusCompleted
		if i >= 8 {
			targetStatus = domain.StatusFailed
		}
		run.Status = targetStatus
		run.StartedAt = &startedAt
		run.HeartbeatAt = &startedAt
		run.FinishedAt = &finishedAt
		if targetStatus == domain.StatusFailed {
			run.Error = "simulated failure"
		}
		require.NoError(t, st.CreateRun(ctx,
			run))

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
	require.NoError(t, st.CreateJobSLO(ctx,
		slo))

	// Verify the SLO was created.
	allSLOs, err := st.ListAllJobSLOs(ctx)
	require.NoError(t, err)
	require.Len(t, allSLOs, 1)

	// Run the SLO evaluator.
	evaluator := scheduler.NewSLOEvaluator(st, nil)
	require.NoError(t, evaluator.
		Evaluate(ctx))

	// Check that an evaluation was recorded.
	statuses, err := st.ListJobSLOs(ctx, job.ID)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.NotNil(t, statuses[0].CurrentValue)
	require.NotNil(t, statuses[0].BudgetRemaining)
	assert.LessOrEqual(t, *statuses[0].BudgetRemaining,

		0.01)

	// With 80% success (8/10) and 99% target, budget should be depleted.
	// success_rate from DB is percentage, metricValue divides by 100 -> 0.8
	// budget = 1 - ((1-0.8)/(1-0.99)) = 1 - (0.2/0.01) = 1 - 20 = clamped to 0

	// Verify pruning works without error.
	pruned, err := st.PruneSLOEvaluations(ctx, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 0, pruned)

	// We only have 1 evaluation and keep=1, so nothing should be pruned.

}
