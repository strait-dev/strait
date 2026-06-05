//go:build integration

package queue_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/testutil"

	"github.com/stretchr/testify/require"
)

func mustPgQueQueue(t *testing.T) *queue.PgQueQueue {
	t.Helper()
	return queue.NewPgQueQueue(testDB.Pool, queue.NewPostgresRunWriter(testDB.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "test-" + newID(),
		NackDelay:     10 * time.Millisecond,
		ReceiveWindow: 100,
	})
}

func assertCurrentGenerationActiveClaim(t *testing.T, ctx context.Context, runID string) {
	t.Helper()
	var readyGeneration int64
	var activeClaims int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT s.ready_generation, COUNT(c.run_id)
		FROM job_run_state s
		LEFT JOIN job_run_active_claims c
		  ON c.run_id = s.run_id
		 AND c.ready_generation = s.ready_generation
		WHERE s.run_id = $1
		GROUP BY s.ready_generation`,

		runID).Scan(&readyGeneration, &activeClaims))
	require.EqualValues(t, 1, activeClaims)

}

func TestPgQue_ConstructsQueue(t *testing.T) {
	require.NotNil(t,

		mustPgQueQueue(t))

}

func TestPgQue_EnqueueInTxRollbackLeavesNoClaimableEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-rollback")
	q := mustPgQueQueue(t)

	tx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.EnqueueInTx(ctx, tx, run); err != nil {
		_ = tx.Rollback(ctx)
		require.Failf(t, "test failure",

			"EnqueueInTx: %v", err)
	}
	require.NoError(t, tx.Rollback(ctx))

	claimed, err := q.DequeueN(ctx, 10)
	require.NoError(t, err)
	require.Len(t, claimed,

		0)

}

func TestPgQue_EnqueueReadyRunRecordsEmitMarker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-ready-marker")
	q := mustPgQueQueue(t)

	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Status:    domain.StatusQueued,
		Priority:  5,
	}
	require.NoError(t, q.Enqueue(ctx,
		run))

	var markers int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM strait_pgque_ready_events emit
		JOIN job_run_state s
		  ON s.run_id = emit.run_id
		 AND s.ready_generation = emit.ready_generation
		WHERE emit.run_id = $1`,

		run.ID).Scan(&markers))
	require.EqualValues(t, 1, markers)

	repaired, err := q.ReconcileReadyRuns(ctx, 10)
	require.NoError(t, err)
	require.EqualValues(t, 0, repaired)

}

func TestPgQue_ReconcileReadyRunsReemitsUnmarkedReadyRunOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-ready-repair")
	q := mustPgQueQueue(t)

	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Status:    domain.StatusQueued,
		Priority:  11,
	}
	require.NoError(t, st.CreateRun(ctx,
		run))

	claimed, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.Len(t, claimed,

		0)

	repaired, err := q.ReconcileReadyRuns(ctx, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, repaired)

	repairedAgain, err := q.ReconcileReadyRuns(ctx, 10)
	require.NoError(t, err)
	require.EqualValues(t, 0, repairedAgain)
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	claimed, err = q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.False(t, len(claimed) !=
		1 || claimed[0].ID !=
		run.
			ID)

	assertCurrentGenerationActiveClaim(t, ctx, run.ID)
}

func TestPgQue_CreatesRouteIdempotently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-route")
	q := mustPgQueQueue(t)

	for range 2 {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
		require.NoError(t, q.Enqueue(ctx,
			run))

	}

	var routeRows, queueRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM strait_pgque_routes WHERE route_key = 'http'`,
	).Scan(&routeRows),
	)
	require.EqualValues(t, 1, routeRows)
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM pgque.queue q
		JOIN strait_pgque_routes r ON r.queue_name = q.queue_name
		WHERE r.route_key = 'http'`,
	).Scan(&queueRows))
	require.EqualValues(t, 1, queueRows)

}

func TestPgQue_MaintainRotatesEventTables(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-maintenance")
	consumerName := "test-" + newID()
	q := queue.NewPgQueQueue(testDB.Pool, queue.NewPostgresRunWriter(testDB.Pool), queue.PgQueConfig{
		TickInterval:        10 * time.Millisecond,
		MaintenanceInterval: 10 * time.Millisecond,
		RotationPeriod:      time.Millisecond,
		ConsumerName:        consumerName,
		NackDelay:           10 * time.Millisecond,
		ReceiveWindow:       100,
	})

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	require.NoError(t, q.Enqueue(ctx,
		run))
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	claimed, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.Len(t, claimed,

		1)

	var queueName string
	var beforeTable int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT r.queue_name, q.queue_cur_table
		FROM strait_pgque_routes r
		JOIN pgque.queue q ON q.queue_name = r.queue_name
		WHERE r.route_key = 'http'`,
	).Scan(&queueName,
		&beforeTable,
	))

	if _, err := testDB.Pool.Exec(ctx, `
		DELETE FROM pgque.subscription s
		USING pgque.consumer c, pgque.queue q
		WHERE s.sub_consumer = c.co_id
		  AND s.sub_queue = q.queue_id
		  AND q.queue_name = $1
		  AND c.co_name <> $2`, queueName, consumerName); err != nil {
		require.Failf(t, "test failure",

			"clean stale test subscriptions: %v", err)
	}
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	var latestTick int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT max(t.tick_id)
		FROM pgque.tick t
		JOIN pgque.queue q ON q.queue_id = t.tick_queue
		WHERE q.queue_name = $1`,

		queueName).Scan(&latestTick))

	if _, err := testDB.Pool.Exec(ctx, `SELECT pgque.register_consumer_at($1, $2, $3)`, queueName, consumerName, latestTick); err != nil {
		require.Failf(t, "test failure",

			"advance consumer tick: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE pgque.queue
		SET queue_switch_time = NOW() - INTERVAL '1 hour',
		    queue_switch_step2 = (
		        SELECT pg_snapshot_xmin(t.tick_snapshot)::text::bigint - 1
		        FROM pgque.tick t
		        WHERE t.tick_queue = pgque.queue.queue_id
		          AND t.tick_id = $2
		    )
		WHERE queue_name = $1`, queueName, latestTick); err != nil {
		require.Failf(t, "test failure",

			"age pgque route queue: %v", err)
	}
	require.NoError(t, q.Maintain(ctx))

	var operations string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT coalesce(string_agg(func_name || ':' || coalesce(func_arg, ''), ','), '')
		FROM pgque.maint_operations()`,
	).Scan(&operations))

	var rotationPeriod, switchAge string
	var switchStep2, tickXmin int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT q.queue_rotation_period::text,
		       age(now(), q.queue_switch_time)::text,
		       q.queue_switch_step2,
		       pg_snapshot_xmin(t.tick_snapshot)::text::bigint
		FROM pgque.queue q
		JOIN pgque.tick t ON t.tick_queue = q.queue_id
		WHERE q.queue_name = $1
		  AND t.tick_id = $2`,

		queueName, latestTick).Scan(&rotationPeriod, &switchAge, &switchStep2,
		&tickXmin))

	var afterTable int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT queue_cur_table
		FROM pgque.queue
		WHERE queue_name = $1`,

		queueName).Scan(&afterTable))
	require.NotEqual(t, beforeTable,

		afterTable)

}

func TestPgQue_DoesNotCreateLegacyQueueEntries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-no-legacy-queue-entry")
	q := mustPgQueQueue(t)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	require.NoError(t, q.Enqueue(ctx,
		run))

	var queueEntries int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM queue_entries WHERE run_id = $1`,

		run.
			ID).Scan(&queueEntries))
	require.EqualValues(t, 0, queueEntries)
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	claimed, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.False(t, len(claimed) !=
		1 || claimed[0].ID !=
		run.
			ID)
	require.NoError(t, st.UpdateRunStatus(ctx, run.
		ID,
		domain.StatusExecuting,

		domain.
			StatusCompleted, map[string]any{"finished_at": time.
			Now()}))
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM queue_entries WHERE run_id = $1`,

		run.
			ID).Scan(&queueEntries))
	require.EqualValues(t, 0, queueEntries)

	batchRuns := []*domain.JobRun{
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID},
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID},
	}
	inserted, err := q.EnqueueBatch(ctx, batchRuns)
	require.NoError(t, err)
	require.Equal(t, int64(
		len(batchRuns)), inserted,
	)
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM queue_entries WHERE run_id = ANY($1)`,

		[]string{batchRuns[0].ID,
			batchRuns[1].ID}).Scan(&queueEntries))
	require.EqualValues(t, 0, queueEntries)

}

func TestPgQue_ReplayedDeadLetterRunBecomesClaimable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-dlq-replay")
	q := mustPgQueQueue(t)

	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
	}
	require.NoError(t, q.Enqueue(ctx,
		run))
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	claimed, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.False(t, len(claimed) !=
		1 || claimed[0].ID !=
		run.
			ID)

	assertCurrentGenerationActiveClaim(t, ctx, run.ID)

	from := claimed[0].Status
	if from != domain.StatusExecuting {
		require.NoError(t, st.UpdateRunStatus(ctx, run.
			ID,
			from, domain.
				StatusExecuting,

			nil))

	}
	require.NoError(t, st.UpdateRunStatus(ctx, run.
		ID,
		domain.StatusExecuting,

		domain.
			StatusDeadLetter, map[string]any{"error": "manual replay regression",
			"error_class": "test"}))

	replayed, err := st.ReplayDeadLetterRun(ctx, run.ID)
	require.NoError(t, err)
	require.NoError(t, q.EnqueueExisting(ctx, replayed))
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	var reclaimed []domain.JobRun
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		reclaimed, err = q.DequeueN(ctx, 1)
		require.NoError(t, err)

		if len(reclaimed) != 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.False(t, len(reclaimed) !=
		1 || reclaimed[0].ID !=
		run.ID)

	assertCurrentGenerationActiveClaim(t, ctx, run.ID)

	duplicate, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.Len(t, duplicate,

		0)

}

func TestPgQue_ActivateDueRunsAppendsReadyEventWithoutMutatingState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	q := mustPgQueQueue(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-delayed-promotion")
	past := time.Now().UTC().Add(-time.Minute)
	run := &domain.JobRun{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Status:      domain.StatusDelayed,
		Attempt:     1,
		Priority:    7,
		TriggeredBy: domain.TriggerManual,
		ScheduledAt: &past,
	}
	require.NoError(t, st.CreateRun(ctx,
		run))

	var beforeGeneration int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT ready_generation
		FROM job_run_state
		WHERE run_id = $1`,

		run.ID).Scan(&beforeGeneration))

	promoted, err := q.ActivateDueRuns(ctx, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, promoted)

	var ledgerStatus, stateStatus, readStatus domain.RunStatus
	var afterGeneration int64
	var readyEvents int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT jr.status, s.status, rs.status, s.ready_generation,
		       (SELECT COUNT(*) FROM job_run_ready_events WHERE run_id = jr.id AND reason = 'delayed_due')
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		JOIN job_run_read_state rs ON rs.run_id = jr.id
		WHERE jr.id = $1`,

		run.ID).Scan(&ledgerStatus,
		&stateStatus, &readStatus, &afterGeneration,
		&readyEvents))
	require.Equal(t, domain.
		StatusDelayed,
		ledgerStatus,
	)
	require.Equal(t, domain.
		StatusDelayed,
		stateStatus,
	)
	require.Equal(t, domain.
		StatusQueued,
		readStatus,
	)
	require.Equal(t, beforeGeneration,

		afterGeneration,
	)
	require.EqualValues(t, 1, readyEvents)

	promotedAgain, err := q.ActivateDueRuns(ctx, 10)
	require.NoError(t, err)
	require.EqualValues(t, 0, promotedAgain)

	var queueEntries int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM queue_entries WHERE run_id = $1`,

		run.
			ID).Scan(&queueEntries))
	require.EqualValues(t, 0, queueEntries)
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	claimed, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.False(t, len(claimed) !=
		1 || claimed[0].ID !=
		run.
			ID)
	require.Equal(t, domain.
		StatusExecuting,
		claimed[0].
			Status)

	assertCurrentGenerationActiveClaim(t, ctx, run.ID)
	require.NoError(t, st.UpdateRunStatus(ctx, run.
		ID,
		domain.StatusExecuting,

		domain.
			StatusCompleted, map[string]any{"finished_at": time.
			Now().UTC()}))

	var terminalStatus domain.RunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT status FROM job_run_read_state WHERE run_id = $1`,

		run.
			ID).Scan(&terminalStatus))
	require.Equal(t, domain.
		StatusCompleted,
		terminalStatus,
	)

}

func TestPgQue_WorkerRecoveredReadyEventOverridesDelayedSchedule(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	q := mustPgQueQueue(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-worker-recovered")
	future := time.Now().UTC().Add(time.Hour)
	run := &domain.JobRun{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Status:      domain.StatusDelayed,
		Attempt:     1,
		Priority:    4,
		TriggeredBy: domain.TriggerManual,
		ScheduledAt: &future,
		NextRetryAt: &future,
	}
	require.NoError(t, st.CreateRun(ctx,
		run))

	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
		SELECT run_id, ready_generation, attempt, 'worker_recovered'
		FROM job_run_state
		WHERE run_id = $1`,
		run.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert worker_recovered ready event: %v", err)
	}

	readyRun, err := st.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		readyRun.
			Status,
	)
	require.NoError(t, q.EnqueueExisting(ctx, readyRun))
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	claimed, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.False(t, len(claimed) !=
		1 || claimed[0].ID !=
		run.
			ID)
	require.Equal(t, domain.
		StatusExecuting,
		claimed[0].
			Status)

	assertCurrentGenerationActiveClaim(t, ctx, run.ID)

	var stateStatus domain.RunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT status FROM job_run_state WHERE run_id = $1`,

		run.ID,
	).Scan(&stateStatus))
	require.Equal(t, domain.
		StatusDelayed,
		stateStatus,
	)

}

func TestPgQue_RequeuePausedJobRunsAppendsReadyEventWithoutStatusFlip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	q := mustPgQueQueue(t)

	projectID := "project-pgque-paused-requeue"
	wf := testutil.MustCreateWorkflow(t, ctx, st, &testutil.WorkflowOpts{ProjectID: &projectID})
	stepJob := testutil.MustCreateJob(t, ctx, st, &testutil.JobOpts{ProjectID: &projectID})
	step := testutil.MustCreateWorkflowStep(t, ctx, st, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, st, wf.ID, &testutil.WorkflowRunOpts{ProjectID: &projectID})

	run := &domain.JobRun{
		ID:          newID(),
		JobID:       stepJob.ID,
		ProjectID:   projectID,
		Status:      domain.StatusPaused,
		Attempt:     1,
		Priority:    5,
		TriggeredBy: domain.TriggerWorkflow,
	}
	require.NoError(t, st.CreateRun(ctx,
		run))

	testutil.MustCreateWorkflowStepRun(t, ctx, st, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{JobRunID: &run.ID})

	var beforeGeneration int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT ready_generation
		FROM job_run_state
		WHERE run_id = $1`,

		run.ID).Scan(&beforeGeneration))

	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
		VALUES ($1, $2, $3, NOW())`,
		run.ID,
		beforeGeneration,
		run.Attempt,
	); err != nil {
		require.Failf(t, "test failure",

			"insert stale paused active claim: %v", err)
	}

	requeued, err := q.RequeuePausedJobRuns(ctx, wfRun.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, requeued)

	var ledgerStatus, stateStatus, readStatus domain.RunStatus
	var afterGeneration int64
	var readyEvents int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT jr.status, s.status, rs.status, s.ready_generation,
		       (SELECT COUNT(*) FROM job_run_ready_events WHERE run_id = jr.id AND reason = 'paused_resume')
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		JOIN job_run_read_state rs ON rs.run_id = jr.id
		WHERE jr.id = $1`,

		run.ID).Scan(&ledgerStatus,
		&stateStatus, &readStatus, &afterGeneration,
		&readyEvents))
	require.Equal(t, domain.
		StatusPaused,
		ledgerStatus,
	)
	require.Equal(t, domain.
		StatusPaused,
		stateStatus,
	)
	require.Equal(t, beforeGeneration+
		1, afterGeneration,
	)
	require.Equal(t, domain.
		StatusQueued,
		readStatus,
	)
	require.EqualValues(t, 1, readyEvents)

	var firstResumeLifecycleEvents, firstResumeCacheVersions int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT
		       (SELECT COUNT(*) FROM job_run_lifecycle_events WHERE run_id = s.run_id AND from_status = 'paused' AND to_status = 'queued'),
		       (SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id = s.run_id)
		FROM job_run_state s
		WHERE s.run_id = $1`,

		run.ID).Scan(&firstResumeLifecycleEvents, &firstResumeCacheVersions))
	require.EqualValues(t, 1, firstResumeLifecycleEvents)

	requeued, err = q.RequeuePausedJobRuns(ctx, wfRun.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, requeued)

	var duplicateGeneration int64
	var duplicateReadyEvents, duplicateLifecycleEvents, duplicateCacheVersions int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT s.ready_generation,
		       (SELECT COUNT(*) FROM job_run_ready_events WHERE run_id = s.run_id AND reason = 'paused_resume'),
		       (SELECT COUNT(*) FROM job_run_lifecycle_events WHERE run_id = s.run_id AND from_status = 'paused' AND to_status = 'queued'),
		       (SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id = s.run_id)
		FROM job_run_state s
		WHERE s.run_id = $1`,

		run.
			ID).Scan(&duplicateGeneration,

		&duplicateReadyEvents, &duplicateLifecycleEvents, &duplicateCacheVersions))
	require.Equal(t, afterGeneration,

		duplicateGeneration,
	)
	require.EqualValues(t, 1, duplicateReadyEvents)
	require.EqualValues(t, 1, duplicateLifecycleEvents)
	require.Equal(t, firstResumeCacheVersions,

		duplicateCacheVersions,
	)

	var queueEntries int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM queue_entries WHERE run_id = $1`,

		run.
			ID).Scan(&queueEntries))
	require.EqualValues(t, 0, queueEntries)
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	claimed, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.False(t, len(claimed) !=
		1 || claimed[0].ID !=
		run.
			ID)
	require.Equal(t, domain.
		StatusExecuting,
		claimed[0].
			Status)

	assertCurrentGenerationActiveClaim(t, ctx, run.ID)
	require.NoError(t, st.UpdateRunStatus(ctx, run.
		ID,
		domain.StatusExecuting,

		domain.
			StatusCompleted, map[string]any{"finished_at": time.
			Now().UTC()}))

	var terminalStatus domain.RunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT status FROM job_run_read_state WHERE run_id = $1`,

		run.
			ID).Scan(&terminalStatus))
	require.Equal(t, domain.
		StatusCompleted,
		terminalStatus,
	)

}

func TestPgQue_ActivateDueRunsPromotesReadyRetries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	q := mustPgQueQueue(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-ready-retry")
	run := &domain.JobRun{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Status:      domain.StatusQueued,
		Attempt:     1,
		Priority:    3,
		TriggeredBy: domain.TriggerManual,
	}
	require.NoError(t, st.CreateRun(ctx,
		run))

	future := time.Now().UTC().Add(time.Hour)
	require.NoError(t, st.ScheduleRetry(ctx, run.
		ID, future,
		2),
	)

	promoted, err := q.ActivateDueRuns(ctx, 10)
	require.NoError(t, err)
	require.EqualValues(t, 0, promoted)

	var beforeGeneration int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT ready_generation FROM job_run_state WHERE run_id = $1`,

		run.ID).Scan(&beforeGeneration))

	past := time.Now().UTC().Add(-time.Second)
	require.NoError(t, st.ScheduleRetry(ctx, run.
		ID, past,
		2))

	promoted, err = q.ActivateDueRuns(ctx, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, promoted)

	var stateStatus, readStatus domain.RunStatus
	var stateAttempt, readAttempt int
	var afterGeneration int64
	var rawRetryRows, pendingRetries, readyEvents int
	var latestRetryCleared bool
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT s.status, s.attempt, rs.status, rs.attempt, s.ready_generation,
		       (SELECT COUNT(*) FROM job_retries WHERE run_id = s.run_id),
		       COALESCE((SELECT cleared FROM job_retries WHERE run_id = s.run_id ORDER BY id DESC LIMIT 1), FALSE),
		       (SELECT COUNT(*) FROM job_retries r
		        WHERE r.run_id = s.run_id
		          AND r.cleared = FALSE
		          AND r.next_retry_at IS NOT NULL
		          AND NOT EXISTS (
		              SELECT 1 FROM job_retries newer WHERE newer.run_id = r.run_id AND newer.id > r.id
		          )),
		       (SELECT COUNT(*) FROM job_run_ready_events WHERE run_id = s.run_id AND reason = 'retry_ready')
		FROM job_run_state s
		JOIN job_run_read_state rs ON rs.run_id = s.run_id
		WHERE s.run_id = $1`,

		run.ID).Scan(&stateStatus, &stateAttempt,
		&readStatus, &readAttempt,
		&afterGeneration, &rawRetryRows,

		&latestRetryCleared, &pendingRetries, &readyEvents))
	require.Equal(t, domain.
		StatusQueued,
		stateStatus,
	)
	require.EqualValues(t, 1, stateAttempt)
	require.Equal(t, domain.
		StatusQueued,
		readStatus,
	)
	require.EqualValues(t, 2, readAttempt)
	require.Equal(t, beforeGeneration,

		afterGeneration,
	)
	require.EqualValues(t, 3, rawRetryRows)
	require.True(t, latestRetryCleared)
	require.EqualValues(t, 0, pendingRetries)
	require.EqualValues(t, 1, readyEvents)
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	claimed, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.False(t, len(claimed) !=
		1 || claimed[0].ID !=
		run.
			ID)
	require.EqualValues(t, 2, claimed[0].Attempt)

	assertCurrentGenerationActiveClaim(t, ctx, run.ID)
}

func TestPgQue_ActivateDueRunsPromotesRetriesWithDelayedBacklog(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	q := mustPgQueQueue(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-retry-fairness")

	past := time.Now().UTC().Add(-time.Minute)
	delayedIDs := make([]string, 0, 2)
	for range 2 {
		run := &domain.JobRun{
			ID:          newID(),
			JobID:       job.ID,
			ProjectID:   job.ProjectID,
			Status:      domain.StatusDelayed,
			Attempt:     1,
			Priority:    3,
			ScheduledAt: &past,
			TriggeredBy: domain.TriggerManual,
		}
		require.NoError(t, st.CreateRun(ctx,
			run))

		delayedIDs = append(delayedIDs, run.ID)
	}
	retryRun := &domain.JobRun{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Status:      domain.StatusQueued,
		Attempt:     1,
		Priority:    3,
		TriggeredBy: domain.TriggerManual,
	}
	require.NoError(t, st.CreateRun(ctx,
		retryRun,
	))
	require.NoError(t, st.ScheduleRetry(ctx, retryRun.
		ID,
		past,
		2))

	promoted, err := q.ActivateDueRuns(ctx, 2)
	require.NoError(t, err)
	require.EqualValues(t, 2, promoted)

	var delayedReady, retryReady, rawRetryRows, pendingRetries int
	var latestRetryCleared bool
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT
			(SELECT COUNT(*) FROM job_run_ready_events WHERE run_id = ANY($1) AND reason = 'delayed_due'),
			(SELECT COUNT(*) FROM job_run_ready_events WHERE run_id = $2 AND reason = 'retry_ready'),
			(SELECT COUNT(*) FROM job_retries WHERE run_id = $2),
			COALESCE((SELECT cleared FROM job_retries WHERE run_id = $2 ORDER BY id DESC LIMIT 1), FALSE),
			(SELECT COUNT(*) FROM job_retries r
			 WHERE r.run_id = $2
			   AND r.cleared = FALSE
			   AND r.next_retry_at IS NOT NULL
			   AND NOT EXISTS (
			       SELECT 1 FROM job_retries newer WHERE newer.run_id = r.run_id AND newer.id > r.id
			   ))`,

		delayedIDs,
		retryRun.ID).Scan(&delayedReady, &retryReady,
		&rawRetryRows, &latestRetryCleared,
		&pendingRetries))
	require.EqualValues(t, 1, delayedReady)
	require.EqualValues(t, 1, retryReady)
	require.EqualValues(t, 2, rawRetryRows)
	require.True(t, latestRetryCleared)
	require.EqualValues(t, 0, pendingRetries)

	promoted, err = q.ActivateDueRuns(ctx, 2)
	require.NoError(t, err)
	require.EqualValues(t, 1, promoted)
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_run_ready_events
		WHERE run_id = ANY($1)
		  AND reason = 'delayed_due'`,

		delayedIDs,
	).Scan(&delayedReady))
	require.EqualValues(t, 2, delayedReady)

}

func TestPgQue_DequeueWindowDoesNotLoseUnseenBatchMessages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-window")
	q := queue.NewPgQueQueue(testDB.Pool, queue.NewPostgresRunWriter(testDB.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "test-" + newID(),
		NackDelay:     10 * time.Millisecond,
		ReceiveWindow: 10,
	})

	want := 25
	for i := 0; i < want; i++ {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
		require.NoError(t, q.Enqueue(ctx,
			run))

	}
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	seen := make(map[string]struct{}, want)
	deadline := time.Now().Add(5 * time.Second)
	for len(seen) < want && time.Now().Before(deadline) {
		runs, err := q.DequeueN(ctx, 5)
		require.NoError(t, err)

		if len(runs) == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		for _, run := range runs {
			if _, ok := seen[run.ID]; ok {
				require.Failf(t, "test failure",

					"duplicate claim for run %s", run.ID)
			}
			seen[run.ID] = struct{}{}
		}
	}
	require.Len(t, seen, want)

}

func TestPgQue_DequeueCatchesUpAcrossEmptyTickLag(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-empty-tick-lag")
	q := queue.NewPgQueQueue(testDB.Pool, queue.NewPostgresRunWriter(testDB.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "test-" + newID(),
		NackDelay:     10 * time.Millisecond,
		ReceiveWindow: 10,
	})

	primed, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.Empty(t, primed)
	for i := 0; i < 12; i++ {
		require.NoErrorf(t, q.ForceTick(ctx, "http"), "ForceTick empty %d", i)
	}

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	require.NoError(t, q.Enqueue(ctx, run))
	require.NoError(t, q.ForceTick(ctx, "http"))

	claimed, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, run.ID, claimed[0].ID)
}

func TestPgQue_ConcurrentDequeueDrainsSingleBatchWithoutDuplicates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-concurrent-batch")
	q := queue.NewPgQueQueue(testDB.Pool, queue.NewPostgresRunWriter(testDB.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "test-" + newID(),
		NackDelay:     10 * time.Millisecond,
		ReceiveWindow: 100,
	})

	const want = 120
	runs := make([]*domain.JobRun, 0, want)
	for range want {
		runs = append(runs, &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID})
	}
	inserted, err := q.EnqueueBatch(ctx, runs)
	require.NoError(t, err)
	require.EqualValues(t, want,
		inserted,
	)
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	var mu sync.Mutex
	seen := make(map[string]struct{}, want)
	errCh := make(chan error, 1)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for {
				mu.Lock()
				done := len(seen) >= want
				mu.Unlock()
				if done {
					return
				}
				claimed, err := q.DequeueN(ctx, 4)
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
				if len(claimed) == 0 {
					time.Sleep(5 * time.Millisecond)
					continue
				}
				mu.Lock()
				for _, run := range claimed {
					if _, ok := seen[run.ID]; ok {
						mu.Unlock()
						select {
						case errCh <- fmt.Errorf("duplicate claim for run %s", run.ID):
						default:
						}
						return
					}
					seen[run.ID] = struct{}{}
				}
				mu.Unlock()
			}
		}()
	}
	close(start)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case err := <-errCh:
		require.Failf(t, "test failure", "concurrent dequeue: %v", err)
	case <-done:
	case <-ctx.Done():
		require.Failf(t, "test failure", "timed out waiting for concurrent dequeue: claimed %d of %d", len(seen), want)
	}
	require.Len(t, seen, want)

}

func TestPgQue_DequeueUsesAppendOnlyPriorityEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-priority-events")
	q := mustPgQueQueue(t)

	low := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Priority:  1,
	}
	high := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Priority:  50,
	}
	require.NoError(t, q.Enqueue(ctx,
		low))
	require.NoError(t, q.Enqueue(ctx,
		high))

	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_priority_events (run_id, priority)
		VALUES ($1, 100)`,
		low.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"append priority event: %v", err)
	}
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	claimed, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.Len(t, claimed,

		1)
	require.Equal(t, low.ID,

		claimed[0].ID)
	require.EqualValues(t, 100, claimed[0].
		Priority)

	var statePriority int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT priority
		FROM job_run_state
		WHERE run_id = $1`,

		low.ID).Scan(&statePriority))
	require.EqualValues(t, 1, statePriority)

}

func TestPgQue_ClaimUsesRunStateNotFatLedger(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-state-claim")
	q := mustPgQueQueue(t)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	require.NoError(t, q.Enqueue(ctx,
		run))
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	claimed, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.False(t, len(claimed) !=
		1 || claimed[0].Status !=
		domain.StatusExecuting,
	)

	var ledgerStatus, stateStatus string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT status FROM job_runs WHERE id = $1`,

		run.ID).Scan(&ledgerStatus))
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT status FROM job_run_state WHERE run_id = $1`,

		run.ID,
	).Scan(&stateStatus))
	require.Equal(t, string(
		domain.StatusQueued,
	),
		ledgerStatus,
	)
	require.Equal(t, string(
		domain.StatusQueued,
	),
		stateStatus,
	)

	var claimRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_run_active_claims
		WHERE run_id = $1`,

		run.ID).Scan(&claimRows))
	require.EqualValues(t, 1, claimRows)

	var readStatus string
	var readStartedAt, readUpdatedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT status, started_at, updated_at
		FROM job_run_read_state
		WHERE run_id = $1`,

		run.ID).Scan(&readStatus,

		&readStartedAt, &readUpdatedAt))
	require.Equal(t, string(
		domain.StatusExecuting,
	), readStatus,
	)
	require.False(t, readStartedAt.
		IsZero())
	require.False(t, readUpdatedAt.
		Before(readStartedAt))

	got, err := st.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusExecuting,
		got.
			Status,
	)
	require.False(t, got.StartedAt ==
		nil || got.
		StartedAt.
		IsZero())

}

func TestPgQue_TerminalTransitionCompletesActiveClaimWithoutUpdatingHotState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-active-claim-terminal")
	q := mustPgQueQueue(t)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	require.NoError(t, q.Enqueue(ctx,
		run))
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	claimed, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.False(t, len(claimed) !=
		1 || claimed[0].Status !=
		domain.StatusExecuting,
	)

	finishedAt := time.Now().UTC()
	require.NoError(t, st.UpdateRunStatus(ctx, run.
		ID,
		domain.StatusExecuting,

		domain.
			StatusCompleted, map[string]any{"finished_at": finishedAt}))

	var hotStatus, readStatus, terminalStatus domain.RunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT status FROM job_run_state WHERE run_id = $1`,

		run.ID,
	).Scan(&hotStatus))
	require.Equal(t, domain.
		StatusQueued,
		hotStatus,
	)
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT status FROM job_run_terminal_state WHERE run_id = $1`,

		run.ID).Scan(&terminalStatus))
	require.Equal(t, domain.
		StatusCompleted,
		terminalStatus,
	)
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT status FROM job_run_read_state WHERE run_id = $1`,

		run.
			ID).Scan(&readStatus))
	require.Equal(t, domain.
		StatusCompleted,
		readStatus,
	)

}

func TestPgQue_ActiveClaimEnforcesLimitedConcurrencyWithoutCounterWrites(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-active-claim-counts")
	q := mustPgQueQueue(t)

	runs := []*domain.JobRun{
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID},
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID},
	}
	for _, run := range runs {
		require.NoError(t, q.Enqueue(ctx,
			run))

	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_run_state SET job_max_concurrency = 1 WHERE job_id = $1`, job.ID); err != nil {
		require.Failf(t, "test failure",

			"set max concurrency: %v", err)
	}

	claimed, err := q.DequeueN(ctx, 2)
	require.NoError(t, err)
	require.Len(t, claimed,

		1)

	var activeClaims int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_run_active_claims claim
		JOIN job_run_state s
		  ON s.run_id = claim.run_id
		 AND s.ready_generation = claim.ready_generation
		LEFT JOIN job_run_terminal_state terminal ON terminal.run_id = s.run_id
		WHERE s.job_id = $1
		  AND s.status = 'queued'
		  AND terminal.run_id IS NULL`,

		job.ID,
	).Scan(&activeClaims))
	require.EqualValues(t, 1, activeClaims)

	var count int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COALESCE(SUM(count), 0)
		FROM job_active_counts
		WHERE job_id = $1`,

		job.
			ID).Scan(&count))
	require.EqualValues(t, 0, count)

	counterUpdatedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_active_counts (job_id, concurrency_key, count, updated_at)
		VALUES ($1, '', 0, $2)
		ON CONFLICT (job_id, concurrency_key)
		DO UPDATE SET count = 0, updated_at = EXCLUDED.updated_at`,
		job.ID, counterUpdatedAt,
	); err != nil {
		require.Failf(t, "test failure",

			"seed active count row: %v", err)
	}
	require.NoError(t, st.UpdateRunStatus(ctx, claimed[0].ID, domain.
		StatusExecuting,

		domain.StatusCompleted,
		map[string]any{"finished_at": time.Now()}))

	var afterCounterUpdatedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COALESCE(SUM(count), 0), MAX(updated_at)
		FROM job_active_counts
		WHERE job_id = $1`,

		job.ID).Scan(&count,

		&afterCounterUpdatedAt))
	require.EqualValues(t, 0, count)
	require.True(t, afterCounterUpdatedAt.
		Equal(
			counterUpdatedAt,
		))

	next, err := q.DequeueN(ctx, 2)
	require.NoError(t, err)
	require.Len(t, next, 1)
	require.NotEqual(t, claimed[0].ID,
		next[0].ID,
	)

}

func TestPgQue_ConcurrentLimitedClaimsSerializeOnActiveClaims(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-active-claim-serialized")
	q := mustPgQueQueue(t)

	for range 2 {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
		require.NoError(t, q.Enqueue(ctx,
			run))

	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_run_state SET job_max_concurrency = 1 WHERE job_id = $1`, job.ID); err != nil {
		require.Failf(t, "test failure",

			"set max concurrency: %v", err)
	}
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	start := make(chan struct{})
	errCh := make(chan error, 8)
	claimedCh := make(chan []domain.JobRun, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claimed, err := q.DequeueN(ctx, 1)
			if err != nil {
				errCh <- err
				return
			}
			claimedCh <- claimed
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	close(claimedCh)

	for err := range errCh {
		require.Failf(t, "test failure",

			"DequeueN: %v", err)
	}
	claimedIDs := make(map[string]struct{})
	for claimed := range claimedCh {
		for _, run := range claimed {
			if _, ok := claimedIDs[run.ID]; ok {
				require.Failf(t, "test failure",

					"duplicate claim for run %s", run.ID)
			}
			claimedIDs[run.ID] = struct{}{}
		}
	}
	require.Len(t, claimedIDs,

		1)

	var count int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COALESCE(SUM(count), 0)
		FROM job_active_counts
		WHERE job_id = $1`,

		job.
			ID).Scan(&count))
	require.EqualValues(t, 0, count)

}

func TestPgQue_StaleGenerationEventIsIgnored(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-stale-generation")
	q := mustPgQueQueue(t)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	require.NoError(t, q.Enqueue(ctx,
		run))

	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_run_state SET ready_generation = ready_generation + 1 WHERE run_id = $1`, run.ID); err != nil {
		require.Failf(t, "test failure",

			"bump ready generation: %v", err)
	}

	claimed, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.Len(t, claimed,

		0)

}

func TestPgQue_DequeueNForWorkerQueuesFiltersByEnvironment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	projectID := "project-pgque-worker-env"
	prodEnvID := mustCreateEnvironment(t, ctx, st, projectID, "production")
	stagingEnvID := mustCreateEnvironment(t, ctx, st, projectID, "staging")
	prodJob := mustCreateJob(t, ctx, st, projectID)
	markWorkerJobQueueEnvironment(t, ctx, prodJob, "priority", prodEnvID)
	stagingJob := mustCreateJob(t, ctx, st, projectID)
	markWorkerJobQueueEnvironment(t, ctx, stagingJob, "priority", stagingEnvID)
	q := mustPgQueQueue(t)

	prodRun := &domain.JobRun{ID: newID(), JobID: prodJob.ID, ProjectID: projectID, ExecutionMode: domain.ExecutionModeWorker, QueueName: "priority"}
	require.NoError(t, q.Enqueue(ctx,
		prodRun))

	stagingRun := &domain.JobRun{ID: newID(), JobID: stagingJob.ID, ProjectID: projectID, ExecutionMode: domain.ExecutionModeWorker, QueueName: "priority"}
	require.NoError(t, q.Enqueue(ctx,
		stagingRun,
	))

	stagingBatch, err := q.DequeueNForWorkerQueues(ctx, 1, []domain.WorkerQueueRef{{ProjectID: projectID, QueueName: "priority", EnvironmentID: stagingEnvID}})
	require.NoError(t, err)
	require.False(t, len(stagingBatch) != 1 || stagingBatch[0].
		ID != stagingRun.
		ID,
	)

	prodBatch, err := q.DequeueNForWorkerQueues(ctx, 1, []domain.WorkerQueueRef{{ProjectID: projectID, QueueName: "priority", EnvironmentID: prodEnvID}})
	require.NoError(t, err)
	require.False(t, len(prodBatch) !=
		1 || prodBatch[0].ID !=
		prodRun.ID)

}

func TestPgQue_ReconcileReadyRunsPreservesWorkerEnvironmentRoutes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	projectID := "project-pgque-worker-repair-env"
	prodEnvID := mustCreateEnvironment(t, ctx, st, projectID, "production")
	stagingEnvID := mustCreateEnvironment(t, ctx, st, projectID, "staging")
	prodJob := mustCreateJob(t, ctx, st, projectID)
	markWorkerJobQueueEnvironment(t, ctx, prodJob, "priority", prodEnvID)
	stagingJob := mustCreateJob(t, ctx, st, projectID)
	markWorkerJobQueueEnvironment(t, ctx, stagingJob, "priority", stagingEnvID)
	q := mustPgQueQueue(t)

	prodRun := &domain.JobRun{
		ID:            newID(),
		JobID:         prodJob.ID,
		ProjectID:     projectID,
		Status:        domain.StatusQueued,
		ExecutionMode: domain.ExecutionModeWorker,
		QueueName:     "priority",
		Priority:      8,
	}
	require.NoError(t, st.CreateRun(ctx,
		prodRun,
	))

	stagingRun := &domain.JobRun{
		ID:            newID(),
		JobID:         stagingJob.ID,
		ProjectID:     projectID,
		Status:        domain.StatusQueued,
		ExecutionMode: domain.ExecutionModeWorker,
		QueueName:     "priority",
		Priority:      9,
	}
	require.NoError(t, st.CreateRun(ctx,
		stagingRun,
	))

	beforeRepair, err := q.DequeueNForWorkerQueues(ctx, 1, []domain.WorkerQueueRef{{
		ProjectID:     projectID,
		QueueName:     "priority",
		EnvironmentID: stagingEnvID,
	}})
	require.NoError(t, err)
	require.Len(t, beforeRepair,

		0)

	repaired, err := q.ReconcileReadyRuns(ctx, 10)
	require.NoError(t, err)
	require.EqualValues(t, 2, repaired)

	repairedAgain, err := q.ReconcileReadyRuns(ctx, 10)
	require.NoError(t, err)
	require.EqualValues(t, 0, repairedAgain)

	stagingBatch, err := q.DequeueNForWorkerQueues(ctx, 1, []domain.WorkerQueueRef{{
		ProjectID:     projectID,
		QueueName:     "priority",
		EnvironmentID: stagingEnvID,
	}})
	require.NoError(t, err)
	require.False(t, len(stagingBatch) != 1 || stagingBatch[0].
		ID != stagingRun.
		ID,
	)

	prodBatch, err := q.DequeueNForWorkerQueues(ctx, 1, []domain.WorkerQueueRef{{
		ProjectID:     projectID,
		QueueName:     "priority",
		EnvironmentID: prodEnvID,
	}})
	require.NoError(t, err)
	require.False(t, len(prodBatch) !=
		1 || prodBatch[0].ID !=
		prodRun.ID)

	assertCurrentGenerationActiveClaim(t, ctx, stagingRun.ID)
	assertCurrentGenerationActiveClaim(t, ctx, prodRun.ID)
}

func TestPgQue_WorkerEnvironmentClaimsAreUniqueAndComplete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	projectID := "project-pgque-worker-env-invariants"
	prodEnvID := mustCreateEnvironment(t, ctx, st, projectID, "production")
	stagingEnvID := mustCreateEnvironment(t, ctx, st, projectID, "staging")
	prodJob := mustCreateJob(t, ctx, st, projectID)
	markWorkerJobQueueEnvironment(t, ctx, prodJob, "priority", prodEnvID)
	stagingJob := mustCreateJob(t, ctx, st, projectID)
	markWorkerJobQueueEnvironment(t, ctx, stagingJob, "priority", stagingEnvID)
	q := mustPgQueQueue(t)

	const runsPerEnvironment = 12
	prodWant := make(map[string]struct{}, runsPerEnvironment)
	stagingWant := make(map[string]struct{}, runsPerEnvironment)
	for range runsPerEnvironment {
		prodRun := &domain.JobRun{ID: newID(), JobID: prodJob.ID, ProjectID: projectID, ExecutionMode: domain.ExecutionModeWorker, QueueName: "priority"}
		require.NoError(t, q.Enqueue(ctx,
			prodRun))

		prodWant[prodRun.ID] = struct{}{}

		stagingRun := &domain.JobRun{ID: newID(), JobID: stagingJob.ID, ProjectID: projectID, ExecutionMode: domain.ExecutionModeWorker, QueueName: "priority"}
		require.NoError(t, q.Enqueue(ctx,
			stagingRun,
		))

		stagingWant[stagingRun.ID] = struct{}{}
	}

	type claimResult struct {
		name string
		runs []domain.JobRun
		err  error
	}
	results := make(chan claimResult, 2)
	var wg sync.WaitGroup
	claimWorkerRuns := func(name, envID string) {
		defer wg.Done()
		claimed := make([]domain.JobRun, 0, runsPerEnvironment)
		for len(claimed) < runsPerEnvironment {
			if err := ctx.Err(); err != nil {
				results <- claimResult{name: name, runs: claimed, err: err}
				return
			}
			batch, err := q.DequeueNForWorkerQueues(ctx, 3, []domain.WorkerQueueRef{{
				ProjectID:     projectID,
				QueueName:     "priority",
				EnvironmentID: envID,
			}})
			if err != nil {
				results <- claimResult{name: name, runs: claimed, err: err}
				return
			}
			if len(batch) == 0 {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			claimed = append(claimed, batch...)
		}
		results <- claimResult{name: name, runs: claimed}
	}

	wg.Add(2)
	go claimWorkerRuns("production", prodEnvID)
	go claimWorkerRuns("staging", stagingEnvID)
	wg.Wait()
	close(results)

	claimedByRun := make(map[string]string, runsPerEnvironment*2)
	for result := range results {
		require.Nil(t, result.
			err)
		require.Len(t, result.runs,

			runsPerEnvironment,
		)

		for _, run := range result.runs {
			if previous, ok := claimedByRun[run.ID]; ok {
				require.Failf(t, "test failure",

					"run %s claimed by both %s and %s", run.ID, previous, result.name)
			}
			claimedByRun[run.ID] = result.name
			switch result.name {
			case "production":
				if _, ok := prodWant[run.ID]; !ok {
					require.Failf(t, "test failure",

						"production worker claimed run %s outside production environment", run.ID)
				}
			case "staging":
				if _, ok := stagingWant[run.ID]; !ok {
					require.Failf(t, "test failure",

						"staging worker claimed run %s outside staging environment", run.ID)
				}
			default:
				require.Failf(t, "test failure", "unexpected worker result %q", result.name)
			}
		}
	}
	require.Len(t, claimedByRun,

		runsPerEnvironment*
			2,
	)

	allRunIDs := make([]string, 0, runsPerEnvironment*2)
	for runID := range prodWant {
		if _, ok := claimedByRun[runID]; !ok {
			require.Failf(t, "test failure",

				"production run %s was never claimed", runID)
		}
		allRunIDs = append(allRunIDs, runID)
	}
	for runID := range stagingWant {
		if _, ok := claimedByRun[runID]; !ok {
			require.Failf(t, "test failure",

				"staging run %s was never claimed", runID)
		}
		allRunIDs = append(allRunIDs, runID)
	}

	var claimRows, distinctClaimedRuns int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*), COUNT(DISTINCT run_id)
		FROM job_run_active_claims
		WHERE run_id = ANY($1::text[])`,

		allRunIDs,
	).Scan(&claimRows, &distinctClaimedRuns))
	require.False(t, claimRows !=
		runsPerEnvironment*
			2 ||
		distinctClaimedRuns !=
			runsPerEnvironment*
				2)

}

func TestPgQue_MaxConcurrencyEnforcedFromRunState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-concurrency")
	q := mustPgQueQueue(t)

	runs := []*domain.JobRun{
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID},
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID},
	}
	for _, run := range runs {
		require.NoError(t, q.Enqueue(ctx,
			run))

	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_run_state SET job_max_concurrency = 1 WHERE job_id = $1`, job.ID); err != nil {
		require.Failf(t, "test failure",

			"set max concurrency: %v", err)
	}

	claimed, err := q.DequeueN(ctx, 2)
	require.NoError(t, err)
	require.Len(t, claimed,

		1)

}
