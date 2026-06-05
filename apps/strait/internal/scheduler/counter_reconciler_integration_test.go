//go:build integration

package scheduler_test

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/scheduler"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupReconciler(t *testing.T) (*testutil.TestDB, *store.Queries, *queue.PgQueQueue, *domain.Job) {
	t.Helper()
	ctx := context.Background()
	tdb := getTestDB(t)
	intTestClean(t, ctx)
	st := store.New(tdb.Pool)
	q := queue.NewPgQueQueue(tdb.Pool, queue.NewPostgresRunWriter(tdb.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "counter-" + uuid.Must(uuid.NewV7()).String(),
		ReceiveWindow: 100,
	})
	tickerCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go q.RunTicker(tickerCtx)

	job := &domain.Job{
		ID:             uuid.Must(uuid.NewV7()).String(),
		ProjectID:      "recon-" + uuid.Must(uuid.NewV7()).String(),
		Name:           "recon-job",
		Slug:           "recon-" + uuid.Must(uuid.NewV7()).String()[:8],
		EndpointURL:    "https://example.com/x",
		MaxAttempts:    3,
		TimeoutSecs:    60,
		MaxConcurrency: 1000,
		Enabled:        true,
	}
	require.NoError(t, st.CreateJob(ctx,
		job))

	return tdb, st, q, job
}

func TestCounterReconciler_HappyPath_ZeroDrift(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	tdb, _, q, job := setupReconciler(t)
	ctx := context.Background()

	for range 5 {
		r := &domain.JobRun{
			ID:        uuid.Must(uuid.NewV7()).String(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Priority:  1,
		}
		require.NoError(t, q.Enqueue(ctx, r))

	}
	_ = intClaimRuns(t, ctx, q, 3)
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO job_active_counts (job_id, concurrency_key, count, updated_at)
		VALUES ($1, '', 3, NOW())
		ON CONFLICT (job_id, concurrency_key)
		DO UPDATE SET count = EXCLUDED.count, updated_at = NOW()`,
		job.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"seed active count: %v", err)
	}

	r := scheduler.NewCounterReconciler(tdb.Pool, scheduler.CounterReconcilerConfig{})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	concWG.Go(func() {
		r.Run(runCtx)
		close(done)
	})
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done
	assert.EqualValues(t, 0, r.TotalDrift())

}

func TestCounterReconciler_InducedDrift_ActiveCounts(t *testing.T) {
	tdb, _, q, job := setupReconciler(t)
	ctx := context.Background()

	for range 5 {
		r := &domain.JobRun{
			ID:        uuid.Must(uuid.NewV7()).String(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
		}
		require.NoError(t, q.Enqueue(ctx, r))

	}
	_ = intClaimRuns(t, ctx, q, 5)
	if _, err := tdb.Pool.Exec(ctx,
		`UPDATE job_run_state SET job_max_concurrency = $2 WHERE job_id = $1`,
		job.ID,
		job.MaxConcurrency,
	); err != nil {
		require.Failf(t, "test failure",

			"enable active count truth: %v", err)
	}

	// Manually corrupt the counter to simulate drift.
	_, err := tdb.Pool.Exec(ctx,
		`INSERT INTO job_active_counts (job_id, concurrency_key, count, updated_at)
		 VALUES ($1, '', 15, NOW())
		 ON CONFLICT (job_id, concurrency_key)
		 DO UPDATE SET count = EXCLUDED.count, updated_at = NOW()`,
		job.ID,
	)
	require.NoError(t, err)

	r := scheduler.NewCounterReconciler(tdb.Pool, scheduler.CounterReconcilerConfig{})
	require.NoError(t, r.RunOnceForTest(ctx))

	// Verify the counter is now correct.
	var count int
	err = tdb.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(count), 0) FROM job_active_counts WHERE job_id = $1`,
		job.ID,
	).Scan(&count)
	require.NoError(t, err)
	assert.EqualValues(t, 5, count)
	assert.GreaterOrEqual(t, r.
		TotalDrift(), int64(10))

}

func TestCounterReconciler_RemovesStaleActiveCountRows(t *testing.T) {
	tdb, _, _, job := setupReconciler(t)
	ctx := context.Background()

	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO job_active_counts (job_id, concurrency_key, count, updated_at)
		VALUES ($1, '', 0, NOW()),
		       ($2, '', 7, NOW())`,
		job.ID,
		uuid.Must(uuid.NewV7()).String(),
	); err != nil {
		require.Failf(t, "test failure",

			"seed stale active counts: %v", err)
	}

	r := scheduler.NewCounterReconciler(tdb.Pool, scheduler.CounterReconcilerConfig{})
	require.NoError(t, r.RunOnceForTest(ctx))

	var rows int
	require.NoError(t, tdb.Pool.
		QueryRow(
			ctx, `SELECT COUNT(*) FROM job_active_counts`,
		).
		Scan(&rows))
	require.EqualValues(t, 0, rows)
	require.EqualValues(t, 7, r.TotalDrift())

}

func TestCounterReconciler_TransactionalReconcileRepairsActiveAndDLQDrift(t *testing.T) {
	tdb, _, q, job := setupReconciler(t)
	ctx := context.Background()

	for range 3 {
		run := &domain.JobRun{
			ID:        uuid.Must(uuid.NewV7()).String(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
		}
		require.NoError(t, q.Enqueue(ctx, run))

	}
	_ = intClaimRuns(t, ctx, q, 3)
	if _, err := tdb.Pool.Exec(ctx,
		`UPDATE job_run_state SET job_max_concurrency = $2 WHERE job_id = $1`,
		job.ID,
		job.MaxConcurrency,
	); err != nil {
		require.Failf(t, "test failure",

			"enable active count truth: %v", err)
	}
	for range 2 {
		_, err := tdb.Pool.Exec(ctx, `
			INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
			VALUES ($1, $2, $3, 'dead_letter', 1, 'manual', NOW(), NOW())
		`, uuid.Must(uuid.NewV7()).String(), job.ID, job.ProjectID)
		require.NoError(t, err)

	}

	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO job_active_counts (job_id, concurrency_key, count, updated_at)
		VALUES ($1, '', 99, NOW())
		ON CONFLICT (job_id, concurrency_key)
		DO UPDATE SET count = EXCLUDED.count, updated_at = NOW()`, job.ID); err != nil {
		require.Failf(t, "test failure",

			"corrupt active count: %v", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `UPDATE dlq_counts SET count = 42 WHERE job_id = $1`, job.ID); err != nil {
		require.Failf(t, "test failure",

			"corrupt dlq count: %v", err)
	}

	r := scheduler.NewCounterReconciler(tdb.Pool, scheduler.CounterReconcilerConfig{})
	require.NoError(t, r.RunOnceForTest(ctx))

	var activeCount, dlqCount int
	require.NoError(t, tdb.Pool.
		QueryRow(
			ctx, `SELECT COALESCE(SUM(count), 0) FROM job_active_counts WHERE job_id = $1`,

			job.ID).Scan(&activeCount))
	require.NoError(t, tdb.Pool.
		QueryRow(
			ctx, `SELECT COALESCE(SUM(count), 0) FROM dlq_counts WHERE job_id = $1`,

			job.ID).Scan(&dlqCount))
	assert.EqualValues(t, 3, activeCount)
	assert.EqualValues(t, 2, dlqCount)
	assert.NotEqual(t, 0, r.TotalDrift())

}

func TestCounterReconciler_DoesNotTakeJobRunsTableLock(t *testing.T) {
	tdb, _, _, _ := setupReconciler(t)
	ctx := context.Background()

	writerTx, err := tdb.Pool.Begin(ctx)
	require.NoError(t, err)

	defer writerTx.Rollback(ctx) //nolint:errcheck
	if _, err := writerTx.Exec(ctx, `LOCK TABLE job_runs IN ROW EXCLUSIVE MODE`); err != nil {
		require.Failf(t, "test failure",

			"hold writer table lock: %v", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	r := scheduler.NewCounterReconciler(tdb.Pool, scheduler.CounterReconcilerConfig{})
	require.NoError(t, r.RunOnceForTest(runCtx))

}

func TestCounterReconciler_InducedDrift_DLQCounts(t *testing.T) {
	tdb, _, _, job := setupReconciler(t)
	ctx := context.Background()

	// Directly insert dead_letter rows.
	for range 3 {
		_, err := tdb.Pool.Exec(ctx, `
			INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at)
			VALUES ($1, $2, $3, 'dead_letter', 1, 'manual', NOW())
		`, uuid.Must(uuid.NewV7()).String(), job.ID, job.ProjectID)
		require.NoError(t, err)

	}

	// Corrupt the dlq counter.
	_, err := tdb.Pool.Exec(ctx,
		`UPDATE dlq_counts SET count = 100 WHERE job_id = $1`,
		job.ID,
	)
	require.NoError(t, err)

	r := scheduler.NewCounterReconciler(tdb.Pool, scheduler.CounterReconcilerConfig{})
	require.NoError(t, r.RunOnceForTest(ctx))

	var count int
	err = tdb.Pool.QueryRow(ctx, `SELECT count FROM dlq_counts WHERE job_id = $1`, job.ID).Scan(&count)
	require.NoError(t, err)
	assert.EqualValues(t, 3, count)

}

func TestCounterReconciler_BypassTriggerRepaired(t *testing.T) {
	tdb, _, _, job := setupReconciler(t)
	ctx := context.Background()

	// Disable the trigger, insert rows (counter unchanged), re-enable.
	_, err := tdb.Pool.Exec(ctx, `ALTER TABLE job_run_state DISABLE TRIGGER job_run_state_active_counts_trg`)
	require.NoError(t, err)

	for range 4 {
		runID := uuid.Must(uuid.NewV7()).String()
		_, err := tdb.Pool.Exec(ctx, `
				INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, started_at, job_max_concurrency)
				VALUES ($1, $2, $3, 'executing', 1, 'manual', NOW(), NOW(), 1000)
			`, runID, job.ID, job.ProjectID)
		require.NoError(t, err)

		if _, err := tdb.Pool.Exec(ctx, `
			INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
			SELECT run_id, ready_generation, attempt, NOW()
			FROM job_run_state
			WHERE run_id = $1
		`, runID); err != nil {
			require.Failf(t, "test failure",

				"insert active claim: %v", err)
		}
	}
	_, err = tdb.Pool.Exec(ctx, `ALTER TABLE job_run_state ENABLE TRIGGER job_run_state_active_counts_trg`)
	require.NoError(t, err)

	// Counter should be 0 (trigger was off during inserts).
	var before int
	_ = tdb.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(count),0) FROM job_active_counts WHERE job_id=$1`, job.ID).Scan(&before)
	require.EqualValues(t, 0, before)

	// Reconcile.
	r := scheduler.NewCounterReconciler(tdb.Pool, scheduler.CounterReconcilerConfig{})
	require.NoError(t, r.RunOnceForTest(ctx))

	var after int
	_ = tdb.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(count),0) FROM job_active_counts WHERE job_id=$1`, job.ID).Scan(&after)
	assert.EqualValues(t, 4, after)

}

func TestCounterReconciler_RemovesStaleDLQCountRows(t *testing.T) {
	tdb, _, _, job := setupReconciler(t)
	ctx := context.Background()

	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO dlq_counts (project_id, job_id, count, updated_at)
		VALUES ($1, $2, 0, NOW()),
		       ($3, $4, 11, NOW())`,
		job.ProjectID,
		job.ID,
		"stale-"+uuid.Must(uuid.NewV7()).String(),
		uuid.Must(uuid.NewV7()).String(),
	); err != nil {
		require.Failf(t, "test failure",

			"seed stale dlq counts: %v", err)
	}

	r := scheduler.NewCounterReconciler(tdb.Pool, scheduler.CounterReconcilerConfig{})
	require.NoError(t, r.RunOnceForTest(ctx))

	var rows int
	require.NoError(t, tdb.Pool.
		QueryRow(
			ctx, `SELECT COUNT(*) FROM dlq_counts`,
		).Scan(&rows))
	require.EqualValues(t, 0, rows)
	require.EqualValues(t, 11, r.TotalDrift())

}

// TestCounterReconciler_PropertyRandomOps runs a random sequence of queue
// operations and asserts that after reconcile the counters always equal
// ground truth.
func TestCounterReconciler_PropertyRandomOps(t *testing.T) {
	tdb, _, q, job := setupReconciler(t)
	ctx := context.Background()

	rng := rand.New(rand.NewPCG(42, 42))
	var runIDs []string
	const ops = 200

	for range ops {
		switch rng.IntN(5) {
		case 0: // enqueue
			r := &domain.JobRun{
				ID:        uuid.Must(uuid.NewV7()).String(),
				JobID:     job.ID,
				ProjectID: job.ProjectID,
			}
			if err := q.Enqueue(ctx, r); err == nil {
				runIDs = append(runIDs, r.ID)
			}
		case 1: // dequeue
			_, _ = q.DequeueN(ctx, 1+rng.IntN(3))
		case 2: // complete a random dequeued run
			if len(runIDs) > 0 {
				id := runIDs[rng.IntN(len(runIDs))]
				_, _ = tdb.Pool.Exec(ctx, `UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id=$1 AND status IN ('dequeued','executing')`, id)
			}
		case 3: // fail to dead_letter
			if len(runIDs) > 0 {
				id := runIDs[rng.IntN(len(runIDs))]
				_, _ = tdb.Pool.Exec(ctx, `UPDATE job_runs SET status='dead_letter', finished_at=NOW() WHERE id=$1 AND status='queued'`, id)
			}
		case 4: // mask a dlq row
			_, _ = tdb.Pool.Exec(ctx, `UPDATE job_runs SET visible_until=NOW() WHERE status='dead_letter' AND visible_until IS NULL AND job_id=$1`, job.ID)
		}
	}

	// Reconcile and assert zero drift (meaning the trigger stayed correct).
	r := scheduler.NewCounterReconciler(tdb.Pool, scheduler.CounterReconcilerConfig{})
	require.NoError(t, r.RunOnceForTest(ctx))

	var activeCount, truthCount int
	require.NoError(t, tdb.Pool.
		QueryRow(
			ctx, `SELECT COALESCE(SUM(count),0) FROM job_active_counts WHERE job_id=$1`,

			job.ID).Scan(&activeCount))
	require.NoError(t, tdb.Pool.
		QueryRow(
			ctx, `
		SELECT COUNT(*)::int
		FROM job_run_state s
		JOIN job_run_active_claims c
		  ON c.run_id = s.run_id
		 AND c.ready_generation = s.ready_generation
		LEFT JOIN job_run_terminal_state terminal ON terminal.run_id = s.run_id
		WHERE s.job_id = $1
		  AND terminal.run_id IS NULL
		  AND (s.job_max_concurrency IS NOT NULL OR s.job_max_concurrency_per_key IS NOT NULL)
	`,

			job.ID).
		Scan(&truthCount))
	assert.Equal(t, truthCount,

		activeCount,
	)

}
