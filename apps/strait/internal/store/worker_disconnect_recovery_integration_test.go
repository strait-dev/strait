//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

func TestIntegration_RequeueOpenWorkerTasks_RequeuesExecutingRuns(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID := "proj-disconnect-recovery"
	workerID := "worker-disconnect-recovery"

	if err := q.CreateProject(ctx, &domain.Project{
		ID:    projectID,
		OrgID: "org-1",
		Name:  "disconnect-recovery",
	}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{
		ProjectID: &projectID,
	})

	if err := q.RegisterWorker(ctx, &domain.Worker{
		ID:        workerID,
		ProjectID: projectID,
		QueueName: "default",
		Hostname:  "host",
		Version:   "1.0",
		Status:    domain.WorkerStatusActive,
	}); err != nil {
		t.Fatalf("RegisterWorker: %v", err)
	}

	executing := domain.StatusExecuting
	run := testutil.BuildRun(job, &testutil.RunOpts{
		ID:     new("run-disconnect-recovery"),
		Status: &executing,
	})
	run.ExecutionMode = domain.ExecutionModeWorker
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := q.CreateWorkerTask(ctx, &domain.WorkerTask{
		ID:        "task-disconnect-recovery",
		WorkerID:  workerID,
		RunID:     run.ID,
		ProjectID: projectID,
		Status:    domain.WorkerTaskStatusAssigned,
	}); err != nil {
		t.Fatalf("CreateWorkerTask: %v", err)
	}

	count, err := q.RequeueOpenWorkerTasks(ctx, workerID, projectID, "worker disconnected before reporting result")
	if err != nil {
		t.Fatalf("RequeueOpenWorkerTasks: %v", err)
	}
	if count != 1 {
		t.Fatalf("RequeueOpenWorkerTasks count = %d, want 1", count)
	}

	gotRun, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if gotRun.Status != domain.StatusQueued {
		t.Fatalf("run status = %q, want queued", gotRun.Status)
	}
	if gotRun.Error != "worker disconnected before reporting result" {
		t.Fatalf("run error = %q, want disconnect reason", gotRun.Error)
	}
	if gotRun.StartedAt != nil || gotRun.FinishedAt != nil || gotRun.HeartbeatAt != nil {
		t.Fatalf("run timestamps not cleared after requeue: started=%v finished=%v heartbeat=%v", gotRun.StartedAt, gotRun.FinishedAt, gotRun.HeartbeatAt)
	}
	var ledgerStatus, stateStatus domain.RunStatus
	if err := env.DB.Pool.QueryRow(ctx, `
		SELECT jr.status, s.status
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,
		run.ID,
	).Scan(&ledgerStatus, &stateStatus); err != nil {
		t.Fatalf("query split worker requeue state: %v", err)
	}
	if ledgerStatus != domain.StatusExecuting {
		t.Fatalf("job_runs status = %q, want immutable executing ledger status", ledgerStatus)
	}
	if stateStatus != domain.StatusQueued {
		t.Fatalf("job_run_state status = %q, want queued", stateStatus)
	}

	gotTask, err := q.GetWorkerTask(ctx, "task-disconnect-recovery")
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if gotTask.Status != domain.WorkerTaskStatusFailed {
		t.Fatalf("worker task status = %q, want failed", gotTask.Status)
	}
	if gotTask.FinishedAt == nil {
		t.Fatal("worker task finished_at = nil, want timestamp")
	}
}

func TestIntegration_RequeueOpenWorkerTasks_PgQueActiveClaimDoesNotTouchActiveCounter(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)
	fixture := seedPgQueClaimedWorkerTask(t, ctx, env, "disconnect")

	count, err := fixture.q.RequeueOpenWorkerTasks(ctx, fixture.workerID, fixture.projectID, "worker disconnected before reporting result")
	if err != nil {
		t.Fatalf("RequeueOpenWorkerTasks: %v", err)
	}
	if count != 1 {
		t.Fatalf("RequeueOpenWorkerTasks count = %d, want 1", count)
	}
	assertPgQueWorkerRecoveryReleasedClaimOnly(t, ctx, env, fixture)
}

func TestIntegration_RecoverStaleWorkerTasks_PgQueActiveClaimDoesNotTouchActiveCounter(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)
	fixture := seedPgQueClaimedWorkerTask(t, ctx, env, "stale")
	if _, err := env.DB.Pool.Exec(ctx, `UPDATE workers SET last_seen_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, fixture.workerID); err != nil {
		t.Fatalf("age worker: %v", err)
	}

	count, err := fixture.q.RecoverStaleWorkerTasks(ctx, time.Now().Add(-5*time.Minute), "stale worker heartbeat")
	if err != nil {
		t.Fatalf("RecoverStaleWorkerTasks: %v", err)
	}
	if count != 1 {
		t.Fatalf("RecoverStaleWorkerTasks count = %d, want 1", count)
	}
	assertPgQueWorkerRecoveryReleasedClaimOnly(t, ctx, env, fixture)
}

func TestIntegration_RequeueOpenWorkerTasks_SkipsResultReceivedRuns(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID := "proj-disconnect-result-received"
	workerID := "worker-disconnect-result-received"

	if err := q.CreateProject(ctx, &domain.Project{
		ID:    projectID,
		OrgID: "org-1",
		Name:  "disconnect-result-received",
	}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{
		ProjectID: &projectID,
	})

	if err := q.RegisterWorker(ctx, &domain.Worker{
		ID:        workerID,
		ProjectID: projectID,
		QueueName: "default",
		Hostname:  "host",
		Version:   "1.0",
		Status:    domain.WorkerStatusActive,
	}); err != nil {
		t.Fatalf("RegisterWorker: %v", err)
	}

	executing := domain.StatusExecuting
	run := testutil.BuildRun(job, &testutil.RunOpts{
		ID:     new("run-disconnect-result-received"),
		Status: &executing,
	})
	run.ExecutionMode = domain.ExecutionModeWorker
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := q.CreateWorkerTask(ctx, &domain.WorkerTask{
		ID:        "task-disconnect-result-received",
		WorkerID:  workerID,
		RunID:     run.ID,
		ProjectID: projectID,
		Status:    domain.WorkerTaskStatusAssigned,
	}); err != nil {
		t.Fatalf("CreateWorkerTask: %v", err)
	}

	marked, err := q.MarkWorkerTaskResultReceived(ctx, "task-disconnect-result-received")
	if err != nil {
		t.Fatalf("MarkWorkerTaskResultReceived: %v", err)
	}
	if !marked {
		t.Fatal("MarkWorkerTaskResultReceived marked = false, want true")
	}

	count, err := q.RequeueOpenWorkerTasks(ctx, workerID, projectID, "worker disconnected before reporting result")
	if err != nil {
		t.Fatalf("RequeueOpenWorkerTasks: %v", err)
	}
	if count != 0 {
		t.Fatalf("RequeueOpenWorkerTasks count = %d, want 0", count)
	}

	gotRun, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if gotRun.Status != domain.StatusExecuting {
		t.Fatalf("run status = %q, want executing", gotRun.Status)
	}
	if gotRun.Error != "" {
		t.Fatalf("run error = %q, want empty", gotRun.Error)
	}

	gotTask, err := q.GetWorkerTask(ctx, "task-disconnect-result-received")
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if gotTask.Status != domain.WorkerTaskStatusResultReceived {
		t.Fatalf("worker task status = %q, want result_received", gotTask.Status)
	}
	if gotTask.FinishedAt != nil {
		t.Fatalf("worker task finished_at = %v, want nil until finalization", gotTask.FinishedAt)
	}
}

func TestIntegration_DeepSecRecoverStaleWorkerTasks_RequeuesExecutingRuns(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID := "proj-stale-worker-recovery"
	workerID := "worker-stale-recovery"
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org-stale", Name: "stale recovery"}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	if err := q.RegisterWorker(ctx, &domain.Worker{
		ID:        workerID,
		ProjectID: projectID,
		QueueName: "default",
		Hostname:  "host",
		Version:   "1.0",
		Status:    domain.WorkerStatusActive,
	}); err != nil {
		t.Fatalf("RegisterWorker: %v", err)
	}
	if _, err := env.DB.Pool.Exec(ctx, `UPDATE workers SET last_seen_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, workerID); err != nil {
		t.Fatalf("age worker: %v", err)
	}

	executing := domain.StatusExecuting
	run := testutil.BuildRun(job, &testutil.RunOpts{ID: new("run-stale-recovery"), Status: &executing})
	run.ExecutionMode = domain.ExecutionModeWorker
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := q.CreateWorkerTask(ctx, &domain.WorkerTask{
		ID:        "task-stale-recovery",
		WorkerID:  workerID,
		RunID:     run.ID,
		ProjectID: projectID,
		Status:    domain.WorkerTaskStatusAssigned,
	}); err != nil {
		t.Fatalf("CreateWorkerTask: %v", err)
	}

	recoverableRunIDs, err := q.ListRecoverableStaleWorkerTaskRunIDs(ctx, time.Now().Add(-5*time.Minute), nil)
	if err != nil {
		t.Fatalf("ListRecoverableStaleWorkerTaskRunIDs: %v", err)
	}
	if len(recoverableRunIDs) != 1 || recoverableRunIDs[0] != run.ID {
		t.Fatalf("recoverable run IDs = %v, want [%s]", recoverableRunIDs, run.ID)
	}

	count, err := q.RecoverStaleWorkerTasks(ctx, time.Now().Add(-5*time.Minute), "stale worker heartbeat")
	if err != nil {
		t.Fatalf("RecoverStaleWorkerTasks: %v", err)
	}
	if count != 1 {
		t.Fatalf("RecoverStaleWorkerTasks count = %d, want 1", count)
	}
	gotRun, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if gotRun.Status != domain.StatusQueued {
		t.Fatalf("run status = %q, want queued", gotRun.Status)
	}
	var ledgerStatus, stateStatus domain.RunStatus
	if err := env.DB.Pool.QueryRow(ctx, `
		SELECT jr.status, s.status
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,
		run.ID,
	).Scan(&ledgerStatus, &stateStatus); err != nil {
		t.Fatalf("query split stale worker requeue state: %v", err)
	}
	if ledgerStatus != domain.StatusExecuting {
		t.Fatalf("job_runs status = %q, want immutable executing ledger status", ledgerStatus)
	}
	if stateStatus != domain.StatusQueued {
		t.Fatalf("job_run_state status = %q, want queued", stateStatus)
	}
	gotTask, err := q.GetWorkerTask(ctx, "task-stale-recovery")
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if gotTask.Status != domain.WorkerTaskStatusFailed {
		t.Fatalf("worker task status = %q, want failed", gotTask.Status)
	}
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

	if err := q.CreateProject(ctx, &domain.Project{
		ID:    projectID,
		OrgID: "org-1",
		Name:  "pgque-worker-recovery-" + suffix,
	}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	if err := q.RegisterWorker(ctx, &domain.Worker{
		ID:        workerID,
		ProjectID: projectID,
		QueueName: "default",
		Hostname:  "host",
		Version:   "1.0",
		Status:    domain.WorkerStatusActive,
	}); err != nil {
		t.Fatalf("RegisterWorker: %v", err)
	}

	queued := domain.StatusQueued
	run := testutil.BuildRun(job, &testutil.RunOpts{
		ID:     &runID,
		Status: &queued,
	})
	run.ExecutionMode = domain.ExecutionModeWorker
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := env.DB.Pool.Exec(ctx, `UPDATE job_run_state SET job_max_concurrency = 1 WHERE run_id = $1`, runID); err != nil {
		t.Fatalf("mark limited worker run: %v", err)
	}
	if _, err := env.DB.Pool.Exec(ctx, `
		INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
		SELECT run_id, ready_generation, attempt, NOW()
		FROM job_run_state
		WHERE run_id = $1`,
		runID,
	); err != nil {
		t.Fatalf("insert active claim: %v", err)
	}
	if err := q.CreateWorkerTask(ctx, &domain.WorkerTask{
		ID:        taskID,
		WorkerID:  workerID,
		RunID:     runID,
		ProjectID: projectID,
		Status:    domain.WorkerTaskStatusAssigned,
	}); err != nil {
		t.Fatalf("CreateWorkerTask: %v", err)
	}

	counterUpdatedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if _, err := env.DB.Pool.Exec(ctx, `
		INSERT INTO job_active_counts (job_id, concurrency_key, count, updated_at)
		VALUES ($1, '', 0, $2)
		ON CONFLICT (job_id, concurrency_key)
		DO UPDATE SET count = 0, updated_at = EXCLUDED.updated_at`,
		job.ID, counterUpdatedAt,
	); err != nil {
		t.Fatalf("seed active count row: %v", err)
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

func assertPgQueWorkerRecoveryReleasedClaimOnly(
	t *testing.T,
	ctx context.Context,
	env *testutil.TestEnv,
	fixture pgQueClaimedWorkerFixture,
) {
	t.Helper()

	run, err := fixture.q.GetRun(ctx, fixture.runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != domain.StatusQueued {
		t.Fatalf("run status = %q, want queued", run.Status)
	}
	var activeClaims int
	var counterUpdatedAt time.Time
	if err := env.DB.Pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM job_run_active_claims WHERE run_id = $1),
			(SELECT updated_at FROM job_active_counts WHERE job_id = $2 AND concurrency_key = '')`,
		fixture.runID, fixture.jobID,
	).Scan(&activeClaims, &counterUpdatedAt); err != nil {
		t.Fatalf("query active claim/counter state: %v", err)
	}
	if activeClaims != 0 {
		t.Fatalf("active claims = %d, want 0", activeClaims)
	}
	if !counterUpdatedAt.Equal(fixture.counterUpdatedAt) {
		t.Fatalf("active count updated_at changed on PgQue worker recovery: got %s want %s", counterUpdatedAt, fixture.counterUpdatedAt)
	}

	task, err := fixture.q.GetWorkerTask(ctx, fixture.taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if task.Status != domain.WorkerTaskStatusFailed {
		t.Fatalf("worker task status = %q, want failed", task.Status)
	}
}

func TestIntegration_DeepSecRecoverStaleWorkerTasksExcept_SkipsConnectedWorker(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID := "proj-stale-connected-recovery"
	workerID := "worker-stale-but-connected"
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org-stale-connected", Name: "stale connected recovery"}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	if err := q.RegisterWorker(ctx, &domain.Worker{
		ID:        workerID,
		ProjectID: projectID,
		QueueName: "default",
		Hostname:  "host",
		Version:   "1.0",
		Status:    domain.WorkerStatusActive,
	}); err != nil {
		t.Fatalf("RegisterWorker: %v", err)
	}
	if _, err := env.DB.Pool.Exec(ctx, `UPDATE workers SET last_seen_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, workerID); err != nil {
		t.Fatalf("age worker: %v", err)
	}

	executing := domain.StatusExecuting
	run := testutil.BuildRun(job, &testutil.RunOpts{ID: new("run-stale-connected-recovery"), Status: &executing})
	run.ExecutionMode = domain.ExecutionModeWorker
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := q.CreateWorkerTask(ctx, &domain.WorkerTask{
		ID:        "task-stale-connected-recovery",
		WorkerID:  workerID,
		RunID:     run.ID,
		ProjectID: projectID,
		Status:    domain.WorkerTaskStatusAssigned,
	}); err != nil {
		t.Fatalf("CreateWorkerTask: %v", err)
	}

	activeWorkers := []store.ActiveWorkerRef{{WorkerID: workerID, ProjectID: projectID}}
	recoverableRunIDs, err := q.ListRecoverableStaleWorkerTaskRunIDs(ctx, time.Now().Add(-5*time.Minute), activeWorkers)
	if err != nil {
		t.Fatalf("ListRecoverableStaleWorkerTaskRunIDs: %v", err)
	}
	if len(recoverableRunIDs) != 0 {
		t.Fatalf("recoverable run IDs = %v, want none for connected worker", recoverableRunIDs)
	}

	count, err := q.RecoverStaleWorkerTasksExceptRefs(ctx, time.Now().Add(-5*time.Minute), "stale worker heartbeat", activeWorkers)
	if err != nil {
		t.Fatalf("RecoverStaleWorkerTasksExcept: %v", err)
	}
	if count != 0 {
		t.Fatalf("RecoverStaleWorkerTasksExcept count = %d, want 0", count)
	}
	gotRun, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if gotRun.Status != domain.StatusExecuting {
		t.Fatalf("run status = %q, want executing for connected worker", gotRun.Status)
	}
	gotTask, err := q.GetWorkerTask(ctx, "task-stale-connected-recovery")
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if gotTask.Status != domain.WorkerTaskStatusAssigned {
		t.Fatalf("worker task status = %q, want assigned for connected worker", gotTask.Status)
	}

	evicted, err := q.EvictStaleWorkersExceptRefs(ctx, time.Now().Add(-5*time.Minute), activeWorkers)
	if err != nil {
		t.Fatalf("EvictStaleWorkersExcept: %v", err)
	}
	if evicted != 0 {
		t.Fatalf("EvictStaleWorkersExcept evicted = %d, want 0", evicted)
	}
	worker, err := q.GetWorker(ctx, workerID, projectID)
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if worker.Status != domain.WorkerStatusActive {
		t.Fatalf("worker status = %q, want active", worker.Status)
	}
}

func TestIntegration_RecoverStaleWorkerTasksExceptRefs_DoesNotCrossTenantByWorkerID(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	workerID := "shared-worker-id"
	projectA := "proj-worker-active"
	projectB := "proj-worker-stale"
	for _, projectID := range []string{projectA, projectB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org-" + projectID, Name: projectID}); err != nil {
			t.Fatalf("CreateProject(%s): %v", projectID, err)
		}
		if err := q.RegisterWorker(ctx, &domain.Worker{
			ID:        workerID,
			ProjectID: projectID,
			QueueName: "default",
			Hostname:  "host",
			Version:   "1.0",
			Status:    domain.WorkerStatusActive,
		}); err != nil {
			t.Fatalf("RegisterWorker(%s): %v", projectID, err)
		}
	}
	if _, err := env.DB.Pool.Exec(ctx, `UPDATE workers SET last_seen_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, workerID); err != nil {
		t.Fatalf("age workers: %v", err)
	}

	jobB := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectB})
	executing := domain.StatusExecuting
	runB := testutil.BuildRun(jobB, &testutil.RunOpts{ID: new("run-cross-tenant-stale-worker"), Status: &executing})
	runB.ExecutionMode = domain.ExecutionModeWorker
	if err := q.CreateRun(ctx, runB); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := q.CreateWorkerTask(ctx, &domain.WorkerTask{
		ID:        "task-cross-tenant-stale-worker",
		WorkerID:  workerID,
		RunID:     runB.ID,
		ProjectID: projectB,
		Status:    domain.WorkerTaskStatusAssigned,
	}); err != nil {
		t.Fatalf("CreateWorkerTask: %v", err)
	}

	activeProjectA := []store.ActiveWorkerRef{{WorkerID: workerID, ProjectID: projectA}}
	count, err := q.RecoverStaleWorkerTasksExceptRefs(ctx, time.Now().Add(-5*time.Minute), "stale worker heartbeat", activeProjectA)
	if err != nil {
		t.Fatalf("RecoverStaleWorkerTasksExceptRefs: %v", err)
	}
	if count != 1 {
		t.Fatalf("recovered count = %d, want 1 for stale same-id worker in another project", count)
	}
	gotRun, err := q.GetRun(ctx, runB.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if gotRun.Status != domain.StatusQueued {
		t.Fatalf("run status = %q, want queued", gotRun.Status)
	}

	evicted, err := q.EvictStaleWorkersExceptRefs(ctx, time.Now().Add(-5*time.Minute), activeProjectA)
	if err != nil {
		t.Fatalf("EvictStaleWorkersExceptRefs: %v", err)
	}
	if evicted != 1 {
		t.Fatalf("evicted = %d, want only stale worker from project B", evicted)
	}
	workerA, err := q.GetWorker(ctx, workerID, projectA)
	if err != nil {
		t.Fatalf("GetWorker(projectA): %v", err)
	}
	if workerA.Status != domain.WorkerStatusActive {
		t.Fatalf("project A worker status = %q, want active", workerA.Status)
	}
	workerB, err := q.GetWorker(ctx, workerID, projectB)
	if err != nil {
		t.Fatalf("GetWorker(projectB): %v", err)
	}
	if workerB.Status != domain.WorkerStatusOffline {
		t.Fatalf("project B worker status = %q, want offline", workerB.Status)
	}
}

func TestIntegration_DeepSecRecoverStaleWorkerTasks_SkipsFutureStreamLease(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID := "proj-stale-leased-recovery"
	workerID := "worker-stale-but-leased"
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org-stale-leased", Name: "stale leased recovery"}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	if err := q.RegisterWorker(ctx, &domain.Worker{
		ID:        workerID,
		ProjectID: projectID,
		QueueName: "default",
		Hostname:  "host",
		Version:   "1.0",
		Status:    domain.WorkerStatusActive,
	}); err != nil {
		t.Fatalf("RegisterWorker: %v", err)
	}
	if _, err := env.DB.Pool.Exec(ctx, `UPDATE workers SET last_seen_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, workerID); err != nil {
		t.Fatalf("age worker: %v", err)
	}
	if err := q.RenewWorkerStreamLease(ctx, workerID, projectID, time.Now().Add(5*time.Minute)); err != nil {
		t.Fatalf("RenewWorkerStreamLease: %v", err)
	}

	executing := domain.StatusExecuting
	run := testutil.BuildRun(job, &testutil.RunOpts{ID: new("run-stale-leased-recovery"), Status: &executing})
	run.ExecutionMode = domain.ExecutionModeWorker
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := q.CreateWorkerTask(ctx, &domain.WorkerTask{
		ID:        "task-stale-leased-recovery",
		WorkerID:  workerID,
		RunID:     run.ID,
		ProjectID: projectID,
		Status:    domain.WorkerTaskStatusAssigned,
	}); err != nil {
		t.Fatalf("CreateWorkerTask: %v", err)
	}

	count, err := q.RecoverStaleWorkerTasks(ctx, time.Now().Add(-5*time.Minute), "stale worker heartbeat")
	if err != nil {
		t.Fatalf("RecoverStaleWorkerTasks: %v", err)
	}
	if count != 0 {
		t.Fatalf("RecoverStaleWorkerTasks count = %d, want 0 while stream lease is valid", count)
	}
	evicted, err := q.EvictStaleWorkers(ctx, time.Now().Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("EvictStaleWorkers: %v", err)
	}
	if evicted != 0 {
		t.Fatalf("EvictStaleWorkers evicted = %d, want 0 while stream lease is valid", evicted)
	}
	task, err := q.GetWorkerTask(ctx, "task-stale-leased-recovery")
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if task.Status != domain.WorkerTaskStatusAssigned {
		t.Fatalf("worker task status = %q, want assigned", task.Status)
	}
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
	if err != nil {
		t.Fatalf("insert workers: %v", err)
	}
	if _, err := env.DB.Pool.Exec(ctx, `
		INSERT INTO worker_tasks (id, worker_id, run_id, project_id, status, assigned_at)
		VALUES ('task-open-delete-guard', 'offline-open', 'missing-run', 'proj-a', 'assigned', NOW() - INTERVAL '48 hours')
	`); err != nil {
		t.Fatalf("insert open task: %v", err)
	}

	deleted, err := q.DeleteStaleOfflineWorkers(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("DeleteStaleOfflineWorkers: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("DeleteStaleOfflineWorkers deleted = %d, want 1", deleted)
	}
	var remaining int
	if err := env.DB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM workers WHERE id IN ('offline-old', 'offline-open', 'offline-fresh')`).Scan(&remaining); err != nil {
		t.Fatalf("count remaining workers: %v", err)
	}
	if remaining != 2 {
		t.Fatalf("remaining workers = %d, want 2", remaining)
	}
}
