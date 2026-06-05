//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestIntegration_RequeueOpenWorkerTasks_RequeuesExecutingRuns(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID := "proj-disconnect-recovery"
	workerID := "worker-disconnect-recovery"
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: "org-1", Name: "disconnect-recovery"},
	))

	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{
		ProjectID: &projectID,
	})
	require.NoError(t, q.RegisterWorker(ctx, &domain.
		Worker{ID: workerID,

		ProjectID: projectID,
		QueueName: "default", Hostname: "host", Version: "1.0",

		Status: domain.WorkerStatusActive}))

	executing := domain.StatusExecuting
	run := testutil.BuildRun(job, &testutil.RunOpts{
		ID:     new("run-disconnect-recovery"),
		Status: &executing,
	})
	run.ExecutionMode = domain.ExecutionModeWorker
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.CreateWorkerTask(ctx, &domain.WorkerTask{ID: "task-disconnect-recovery",
		WorkerID: workerID, RunID: run.ID, ProjectID: projectID,

		Status: domain.WorkerTaskStatusAssigned}))

	count, err := q.RequeueOpenWorkerTasks(ctx, workerID, projectID, "worker disconnected before reporting result")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	gotRun, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		gotRun.
			Status)
	require.Equal(t, "worker disconnected before reporting result",

		gotRun.Error)
	require.False(t, gotRun.
		StartedAt !=
		nil ||
		gotRun.FinishedAt !=

			nil || gotRun.HeartbeatAt !=
		nil)

	var ledgerStatus, stateStatus domain.RunStatus
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`
		SELECT jr.status, s.status
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,

		run.ID).Scan(&ledgerStatus, &stateStatus))
	require.Equal(t, domain.
		StatusExecuting,
		ledgerStatus,
	)
	require.Equal(t, domain.
		StatusQueued,
		stateStatus,
	)

	gotTask, err := q.GetWorkerTask(ctx, "task-disconnect-recovery")
	require.NoError(t, err)
	require.Equal(t, domain.
		WorkerTaskStatusFailed,

		gotTask.
			Status)
	require.NotNil(t, gotTask.
		FinishedAt,
	)

}

func TestIntegration_RequeueOpenWorkerTasks_PgQueActiveClaimDoesNotTouchActiveCounter(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)
	fixture := seedPgQueClaimedWorkerTask(t, ctx, env, "disconnect")

	count, err := fixture.q.RequeueOpenWorkerTasks(ctx, fixture.workerID, fixture.projectID, "worker disconnected before reporting result")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	assertPgQueWorkerRecoveryReleasedClaimOnly(t, ctx, env, fixture)
}

func TestIntegration_RequeueOpenWorkerTasks_PgQueDelayedActiveClaimBumpsGeneration(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)
	fixture := seedPgQueClaimedWorkerTask(t, ctx, env, "disconnect-delayed")
	beforeGeneration := markPgQueClaimedWorkerRunDelayed(t, ctx, env, fixture.runID)

	count, err := fixture.q.RequeueOpenWorkerTasks(ctx, fixture.workerID, fixture.projectID, "worker disconnected before reporting result")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	assertPgQueWorkerRecoveryReleasedClaimOnly(t, ctx, env, fixture)
	assertPgQueWorkerRecoveryBumpedGeneration(t, ctx, env, fixture.runID, beforeGeneration)
	assertPgQueWorkerRecoveryPreservedDelayedState(t, ctx, env, fixture.runID)
	assertLatestWorkerRecoveryLifecycleEvent(t, ctx, env, fixture.runID, domain.StatusDelayed, domain.StatusQueued)
}

func TestIntegration_RecoverStaleWorkerTasks_PgQueActiveClaimDoesNotTouchActiveCounter(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)
	fixture := seedPgQueClaimedWorkerTask(t, ctx, env, "stale")
	if _, err := env.DB.Pool.Exec(ctx, `UPDATE workers SET last_seen_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, fixture.workerID); err != nil {
		require.Failf(t, "test failure",

			"age worker: %v", err)
	}

	count, err := fixture.q.RecoverStaleWorkerTasks(ctx, time.Now().Add(-5*time.Minute), "stale worker heartbeat")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	assertPgQueWorkerRecoveryReleasedClaimOnly(t, ctx, env, fixture)
}

func TestIntegration_RecoverStaleWorkerTasks_PgQueDelayedActiveClaimIsListedForReadyEvent(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)
	fixture := seedPgQueClaimedWorkerTask(t, ctx, env, "stale-delayed")
	beforeGeneration := markPgQueClaimedWorkerRunDelayed(t, ctx, env, fixture.runID)
	if _, err := env.DB.Pool.Exec(ctx, `UPDATE workers SET last_seen_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, fixture.workerID); err != nil {
		require.Failf(t, "test failure",

			"age worker: %v", err)
	}

	recoverableRunIDs, err := fixture.q.ListRecoverableStaleWorkerTaskRunIDs(ctx, time.Now().Add(-5*time.Minute), nil)
	require.NoError(t, err)
	require.False(t, len(recoverableRunIDs) != 1 ||
		recoverableRunIDs[0] != fixture.runID)

	count, err := fixture.q.RecoverStaleWorkerTasks(ctx, time.Now().Add(-5*time.Minute), "stale worker heartbeat")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	assertPgQueWorkerRecoveryReleasedClaimOnly(t, ctx, env, fixture)
	assertPgQueWorkerRecoveryBumpedGeneration(t, ctx, env, fixture.runID, beforeGeneration)
	assertPgQueWorkerRecoveryPreservedDelayedState(t, ctx, env, fixture.runID)
	assertLatestWorkerRecoveryLifecycleEvent(t, ctx, env, fixture.runID, domain.StatusDelayed, domain.StatusQueued)
}

func TestIntegration_RequeueOpenWorkerTasks_SkipsResultReceivedRuns(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID := "proj-disconnect-result-received"
	workerID := "worker-disconnect-result-received"
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: "org-1", Name: "disconnect-result-received",
	}))

	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{
		ProjectID: &projectID,
	})
	require.NoError(t, q.RegisterWorker(ctx, &domain.
		Worker{ID: workerID,

		ProjectID: projectID,
		QueueName: "default", Hostname: "host", Version: "1.0",

		Status: domain.WorkerStatusActive}))

	executing := domain.StatusExecuting
	run := testutil.BuildRun(job, &testutil.RunOpts{
		ID:     new("run-disconnect-result-received"),
		Status: &executing,
	})
	run.ExecutionMode = domain.ExecutionModeWorker
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.CreateWorkerTask(ctx, &domain.WorkerTask{ID: "task-disconnect-result-received",
		WorkerID: workerID,
		RunID:    run.ID, ProjectID: projectID, Status: domain.WorkerTaskStatusAssigned}))

	marked, err := q.MarkWorkerTaskResultReceived(ctx, "task-disconnect-result-received")
	require.NoError(t, err)
	require.True(t, marked)

	count, err := q.RequeueOpenWorkerTasks(ctx, workerID, projectID, "worker disconnected before reporting result")
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

	gotRun, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusExecuting,
		gotRun.
			Status)
	require.Equal(t, "", gotRun.
		Error,
	)

	gotTask, err := q.GetWorkerTask(ctx, "task-disconnect-result-received")
	require.NoError(t, err)
	require.Equal(t, domain.
		WorkerTaskStatusResultReceived,

		gotTask.
			Status)
	require.Nil(t, gotTask.
		FinishedAt,
	)

}

func TestIntegration_DeepSecRecoverStaleWorkerTasks_RequeuesExecutingRuns(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID := "proj-stale-worker-recovery"
	workerID := "worker-stale-recovery"
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: "org-stale",
		Name:  "stale recovery"}),
	)

	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	require.NoError(t, q.RegisterWorker(ctx, &domain.
		Worker{ID: workerID,

		ProjectID: projectID,
		QueueName: "default", Hostname: "host", Version: "1.0",

		Status: domain.WorkerStatusActive}))

	if _, err := env.DB.Pool.Exec(ctx, `UPDATE workers SET last_seen_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, workerID); err != nil {
		require.Failf(t, "test failure",

			"age worker: %v", err)
	}

	executing := domain.StatusExecuting
	run := testutil.BuildRun(job, &testutil.RunOpts{ID: new("run-stale-recovery"), Status: &executing})
	run.ExecutionMode = domain.ExecutionModeWorker
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.CreateWorkerTask(ctx, &domain.WorkerTask{ID: "task-stale-recovery", WorkerID: workerID, RunID: run.ID, ProjectID: projectID,

		Status: domain.WorkerTaskStatusAssigned}))

	recoverableRunIDs, err := q.ListRecoverableStaleWorkerTaskRunIDs(ctx, time.Now().Add(-5*time.Minute), nil)
	require.NoError(t, err)
	require.False(t, len(recoverableRunIDs) != 1 ||
		recoverableRunIDs[0] != run.ID)

	count, err := q.RecoverStaleWorkerTasks(ctx, time.Now().Add(-5*time.Minute), "stale worker heartbeat")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	gotRun, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		gotRun.
			Status)

	var ledgerStatus, stateStatus domain.RunStatus
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`
		SELECT jr.status, s.status
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,

		run.ID).Scan(&ledgerStatus, &stateStatus))
	require.Equal(t, domain.
		StatusExecuting,
		ledgerStatus,
	)
	require.Equal(t, domain.
		StatusQueued,
		stateStatus,
	)

	gotTask, err := q.GetWorkerTask(ctx, "task-stale-recovery")
	require.NoError(t, err)
	require.Equal(t, domain.
		WorkerTaskStatusFailed,

		gotTask.
			Status)

}

type pgQueClaimedWorkerFixture struct {
	q                *store.Queries
	projectID        string
	workerID         string
	jobID            string
	runID            string
	taskID           string
	counterUpdatedAt time.Time
}

func seedPgQueClaimedWorkerTask(
	t *testing.T,
	ctx context.Context,
	env *testutil.TestEnv,
	suffix string,
) pgQueClaimedWorkerFixture {
	t.Helper()

	q := store.New(env.DB.Pool)
	projectID := "proj-pgque-worker-recovery-" + suffix
	workerID := "worker-pgque-recovery-" + suffix
	taskID := "task-pgque-recovery-" + suffix
	runID := "run-pgque-recovery-" + suffix
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: "org-1", Name: "pgque-worker-recovery-" +
			suffix}))

	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	require.NoError(t, q.RegisterWorker(ctx, &domain.
		Worker{ID: workerID,

		ProjectID: projectID,
		QueueName: "default", Hostname: "host", Version: "1.0",

		Status: domain.WorkerStatusActive}))

	queued := domain.StatusQueued
	run := testutil.BuildRun(job, &testutil.RunOpts{
		ID:     &runID,
		Status: &queued,
	})
	run.ExecutionMode = domain.ExecutionModeWorker
	require.NoError(t, q.CreateRun(ctx,
		run))

	if _, err := env.DB.Pool.Exec(ctx, `UPDATE job_run_state SET job_max_concurrency = 1 WHERE run_id = $1`, runID); err != nil {
		require.Failf(t, "test failure",

			"mark limited worker run: %v", err)
	}
	if _, err := env.DB.Pool.Exec(ctx, `
		INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
		SELECT run_id, ready_generation, attempt, NOW()
		FROM job_run_state
		WHERE run_id = $1`,
		runID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert active claim: %v", err)
	}
	require.NoError(t, q.CreateWorkerTask(ctx, &domain.WorkerTask{ID: taskID, WorkerID: workerID,
		RunID: runID, ProjectID: projectID, Status: domain.
			WorkerTaskStatusAssigned,
	}))

	counterUpdatedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if _, err := env.DB.Pool.Exec(ctx, `
		INSERT INTO job_active_counts (job_id, concurrency_key, count, updated_at)
		VALUES ($1, '', 0, $2)
		ON CONFLICT (job_id, concurrency_key)
		DO UPDATE SET count = 0, updated_at = EXCLUDED.updated_at`,
		job.ID, counterUpdatedAt,
	); err != nil {
		require.Failf(t, "test failure",

			"seed active count row: %v", err)
	}

	return pgQueClaimedWorkerFixture{
		q:                q,
		projectID:        projectID,
		workerID:         workerID,
		jobID:            job.ID,
		runID:            runID,
		taskID:           taskID,
		counterUpdatedAt: counterUpdatedAt,
	}
}

func markPgQueClaimedWorkerRunDelayed(
	t *testing.T,
	ctx context.Context,
	env *testutil.TestEnv,
	runID string,
) int64 {
	t.Helper()

	var readyGeneration int64
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`
		UPDATE job_run_state
		SET status = 'delayed',
		    scheduled_at = NOW() + INTERVAL '5 minutes',
		    next_retry_at = NOW() + INTERVAL '5 minutes'
		WHERE run_id = $1
		RETURNING ready_generation`,

		runID).Scan(&readyGeneration))

	return readyGeneration
}

func assertPgQueWorkerRecoveryReleasedClaimOnly(
	t *testing.T,
	ctx context.Context,
	env *testutil.TestEnv,
	fixture pgQueClaimedWorkerFixture,
) {
	t.Helper()

	run, err := fixture.q.GetRun(ctx, fixture.runID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		run.Status,
	)

	var activeClaims int
	var counterUpdatedAt time.Time
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`
		SELECT
			(SELECT COUNT(*) FROM job_run_active_claims WHERE run_id = $1),
			(SELECT updated_at FROM job_active_counts WHERE job_id = $2 AND concurrency_key = '')`,

		fixture.runID,
		fixture.jobID).Scan(&activeClaims,
		&counterUpdatedAt,
	),
	)
	require.EqualValues(t, 1, activeClaims)
	require.True(t, counterUpdatedAt.
		Equal(fixture.
			counterUpdatedAt,
		),
	)

	deleted, err := fixture.q.DeleteInactiveActiveClaims(ctx, 100)
	require.NoError(t, err)
	require.GreaterOrEqual(
		t,
		deleted,
		int64(1))
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_run_active_claims
		WHERE run_id = $1`,

		fixture.runID).Scan(&activeClaims))
	require.EqualValues(t, 0, activeClaims)

	task, err := fixture.q.GetWorkerTask(ctx, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WorkerTaskStatusFailed,

		task.Status,
	)

}

func assertPgQueWorkerRecoveryBumpedGeneration(
	t *testing.T,
	ctx context.Context,
	env *testutil.TestEnv,
	runID string,
	beforeGeneration int64,
) {
	t.Helper()

	var afterGeneration int64
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`
		SELECT ready_generation
		FROM job_run_state
		WHERE run_id = $1`,

		runID).Scan(&afterGeneration),
	)
	require.Equal(t, beforeGeneration+
		1, afterGeneration,
	)

}

func assertPgQueWorkerRecoveryPreservedDelayedState(
	t *testing.T,
	ctx context.Context,
	env *testutil.TestEnv,
	runID string,
) {
	t.Helper()

	var stateStatus, readStatus domain.RunStatus
	var readyEvents int
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`
		SELECT s.status, rs.status,
		       (SELECT COUNT(*)
		        FROM job_run_ready_events e
		        WHERE e.run_id = s.run_id
		          AND e.ready_generation = s.ready_generation
		          AND e.reason = 'worker_recovered')
		FROM job_run_state s
		JOIN job_run_read_state rs ON rs.run_id = s.run_id
		WHERE s.run_id = $1`,

		runID).Scan(&stateStatus,

		&readStatus, &readyEvents))
	require.Equal(t, domain.
		StatusDelayed,
		stateStatus,
	)
	require.Equal(t, domain.
		StatusQueued,
		readStatus,
	)
	require.EqualValues(t, 1, readyEvents)

}

func assertLatestWorkerRecoveryLifecycleEvent(
	t *testing.T,
	ctx context.Context,
	env *testutil.TestEnv,
	runID string,
	wantFrom domain.RunStatus,
	wantTo domain.RunStatus,
) {
	t.Helper()

	var fromStatus, toStatus domain.RunStatus
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`
		SELECT from_status, to_status
		FROM job_run_lifecycle_events
		WHERE run_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1`,

		runID).Scan(&fromStatus, &toStatus))
	require.False(t, fromStatus !=
		wantFrom ||
		toStatus !=
			wantTo)

}

func TestIntegration_DeepSecRecoverStaleWorkerTasksExcept_SkipsConnectedWorker(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID := "proj-stale-connected-recovery"
	workerID := "worker-stale-but-connected"
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: "org-stale-connected",
		Name:  "stale connected recovery",
	}))

	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	require.NoError(t, q.RegisterWorker(ctx, &domain.
		Worker{ID: workerID,

		ProjectID: projectID,
		QueueName: "default", Hostname: "host", Version: "1.0",

		Status: domain.WorkerStatusActive}))

	if _, err := env.DB.Pool.Exec(ctx, `UPDATE workers SET last_seen_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, workerID); err != nil {
		require.Failf(t, "test failure",

			"age worker: %v", err)
	}

	executing := domain.StatusExecuting
	run := testutil.BuildRun(job, &testutil.RunOpts{ID: new("run-stale-connected-recovery"), Status: &executing})
	run.ExecutionMode = domain.ExecutionModeWorker
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.CreateWorkerTask(ctx, &domain.WorkerTask{ID: "task-stale-connected-recovery",
		WorkerID: workerID,
		RunID:    run.ID, ProjectID: projectID,
		Status: domain.WorkerTaskStatusAssigned}))

	activeWorkers := []store.ActiveWorkerRef{{WorkerID: workerID, ProjectID: projectID}}
	recoverableRunIDs, err := q.ListRecoverableStaleWorkerTaskRunIDs(ctx, time.Now().Add(-5*time.Minute), activeWorkers)
	require.NoError(t, err)
	require.Len(t, recoverableRunIDs,

		0)

	count, err := q.RecoverStaleWorkerTasksExceptRefs(ctx, time.Now().Add(-5*time.Minute), "stale worker heartbeat", activeWorkers)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

	gotRun, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusExecuting,
		gotRun.
			Status)

	gotTask, err := q.GetWorkerTask(ctx, "task-stale-connected-recovery")
	require.NoError(t, err)
	require.Equal(t, domain.
		WorkerTaskStatusAssigned,

		gotTask.
			Status,
	)

	evicted, err := q.EvictStaleWorkersExceptRefs(ctx, time.Now().Add(-5*time.Minute), activeWorkers)
	require.NoError(t, err)
	require.EqualValues(t, 0, evicted)

	worker, err := q.GetWorker(ctx, workerID, projectID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WorkerStatusActive,

		worker.Status,
	)

}

func TestIntegration_RecoverStaleWorkerTasksExceptRefs_DoesNotCrossTenantByWorkerID(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	workerID := "shared-worker-id"
	projectA := "proj-worker-active"
	projectB := "proj-worker-stale"
	for _, projectID := range []string{projectA, projectB} {
		require.NoError(t, q.CreateProject(ctx, &domain.
			Project{ID: projectID,

			OrgID: "org-" + projectID,
			Name:  projectID}),
		)
		require.NoError(t, q.RegisterWorker(ctx, &domain.
			Worker{ID: workerID,

			ProjectID: projectID,
			QueueName: "default", Hostname: "host", Version: "1.0",

			Status: domain.WorkerStatusActive}))

	}
	if _, err := env.DB.Pool.Exec(ctx, `UPDATE workers SET last_seen_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, workerID); err != nil {
		require.Failf(t, "test failure",

			"age workers: %v", err)
	}

	jobB := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectB})
	executing := domain.StatusExecuting
	runB := testutil.BuildRun(jobB, &testutil.RunOpts{ID: new("run-cross-tenant-stale-worker"), Status: &executing})
	runB.ExecutionMode = domain.ExecutionModeWorker
	require.NoError(t, q.CreateRun(ctx,
		runB))
	require.NoError(t, q.CreateWorkerTask(ctx, &domain.WorkerTask{ID: "task-cross-tenant-stale-worker",
		WorkerID: workerID,
		RunID:    runB.ID, ProjectID: projectB, Status: domain.WorkerTaskStatusAssigned}))

	activeProjectA := []store.ActiveWorkerRef{{WorkerID: workerID, ProjectID: projectA}}
	count, err := q.RecoverStaleWorkerTasksExceptRefs(ctx, time.Now().Add(-5*time.Minute), "stale worker heartbeat", activeProjectA)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	gotRun, err := q.GetRun(ctx, runB.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		gotRun.
			Status)

	evicted, err := q.EvictStaleWorkersExceptRefs(ctx, time.Now().Add(-5*time.Minute), activeProjectA)
	require.NoError(t, err)
	require.EqualValues(t, 1, evicted)

	workerA, err := q.GetWorker(ctx, workerID, projectA)
	require.NoError(t, err)
	require.Equal(t, domain.
		WorkerStatusActive,

		workerA.Status,
	)

	workerB, err := q.GetWorker(ctx, workerID, projectB)
	require.NoError(t, err)
	require.Equal(t, domain.
		WorkerStatusOffline,

		workerB.Status,
	)

}

func TestIntegration_DeepSecRecoverStaleWorkerTasks_SkipsFutureStreamLease(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID := "proj-stale-leased-recovery"
	workerID := "worker-stale-but-leased"
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: "org-stale-leased",
		Name:  "stale leased recovery",
	}))

	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	require.NoError(t, q.RegisterWorker(ctx, &domain.
		Worker{ID: workerID,

		ProjectID: projectID,
		QueueName: "default", Hostname: "host", Version: "1.0",

		Status: domain.WorkerStatusActive}))

	if _, err := env.DB.Pool.Exec(ctx, `UPDATE workers SET last_seen_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, workerID); err != nil {
		require.Failf(t, "test failure",

			"age worker: %v", err)
	}
	require.NoError(t, q.RenewWorkerStreamLease(
		ctx, workerID,
		projectID,

		time.Now().Add(5*time.
			Minute)))

	executing := domain.StatusExecuting
	run := testutil.BuildRun(job, &testutil.RunOpts{ID: new("run-stale-leased-recovery"), Status: &executing})
	run.ExecutionMode = domain.ExecutionModeWorker
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.CreateWorkerTask(ctx, &domain.WorkerTask{ID: "task-stale-leased-recovery",
		WorkerID: workerID,
		RunID:    run.ID, ProjectID: projectID,

		Status: domain.WorkerTaskStatusAssigned}))

	count, err := q.RecoverStaleWorkerTasks(ctx, time.Now().Add(-5*time.Minute), "stale worker heartbeat")
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

	evicted, err := q.EvictStaleWorkers(ctx, time.Now().Add(-5*time.Minute))
	require.NoError(t, err)
	require.EqualValues(t, 0, evicted)

	task, err := q.GetWorkerTask(ctx, "task-stale-leased-recovery")
	require.NoError(t, err)
	require.Equal(t, domain.
		WorkerTaskStatusAssigned,

		task.
			Status)

}

func TestIntegration_DeepSecDeleteStaleOfflineWorkers_DoesNotReserveIDsForever(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	_, err := env.DB.Pool.Exec(ctx, `
		INSERT INTO workers (id, project_id, queue_name, hostname, version, status, last_seen_at, registered_at)
		VALUES
			('offline-old', 'proj-a', 'default', 'host', '1.0', 'offline', NOW() - INTERVAL '48 hours', NOW() - INTERVAL '48 hours'),
			('offline-open', 'proj-a', 'default', 'host', '1.0', 'offline', NOW() - INTERVAL '48 hours', NOW() - INTERVAL '48 hours'),
			('offline-fresh', 'proj-a', 'default', 'host', '1.0', 'offline', NOW(), NOW())
	`)
	require.NoError(t, err)

	if _, err := env.DB.Pool.Exec(ctx, `
		INSERT INTO worker_tasks (id, worker_id, run_id, project_id, status, assigned_at)
		VALUES ('task-open-delete-guard', 'offline-open', 'missing-run', 'proj-a', 'assigned', NOW() - INTERVAL '48 hours')
	`); err != nil {
		require.Failf(t, "test failure",

			"insert open task: %v", err)
	}

	deleted, err := q.DeleteStaleOfflineWorkers(ctx, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

	var remaining int
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM workers WHERE id IN ('offline-old', 'offline-open', 'offline-fresh')`,
	).Scan(
		&remaining,
	))
	require.EqualValues(t, 2, remaining)

}
