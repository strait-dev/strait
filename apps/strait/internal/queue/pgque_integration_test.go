//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
)

func mustPgQueQueue(t *testing.T) *queue.PgQueQueue {
	t.Helper()
	return queue.NewPgQueQueue(testDB.Pool, queue.NewPostgresQueue(testDB.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "test-" + newID(),
		NackDelay:     10 * time.Millisecond,
		ReceiveWindow: 100,
	})
}

func TestQueueEngine_PgQueSelectable(t *testing.T) {
	q, err := queue.NewQueueEngine(testDB.Pool, queue.EnginePgQue, queue.BatchlogConfig{TickInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewQueueEngine(pgque): %v", err)
	}
	if _, ok := q.(*queue.PgQueQueue); !ok {
		t.Fatalf("queue = %T, want *PgQueQueue", q)
	}
}

func TestPgQue_EnqueueInTxRollbackLeavesNoClaimableEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-rollback")
	q := mustPgQueQueue(t)

	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.EnqueueInTx(ctx, tx, run); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("EnqueueInTx: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	claimed, err := q.DequeueN(ctx, 10)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("claimed len = %d, want 0 after rollback", len(claimed))
	}
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
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	var routeRows, queueRows int
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM strait_pgque_routes WHERE route_key = 'http'`).Scan(&routeRows); err != nil {
		t.Fatalf("route count: %v", err)
	}
	if routeRows != 1 {
		t.Fatalf("route rows = %d, want 1", routeRows)
	}
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pgque.queue q
		JOIN strait_pgque_routes r ON r.queue_name = q.queue_name
		WHERE r.route_key = 'http'`).Scan(&queueRows); err != nil {
		t.Fatalf("pgque queue count: %v", err)
	}
	if queueRows != 1 {
		t.Fatalf("pgque queue rows = %d, want 1", queueRows)
	}
}

func TestPgQue_ClaimUsesRunStateNotFatLedger(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-state-claim")
	q := mustPgQueQueue(t)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := q.ForceTick(ctx, "http"); err != nil {
		t.Fatalf("ForceTick: %v", err)
	}
	claimed, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Status != domain.StatusDequeued {
		t.Fatalf("claimed = %+v, want one dequeued run", claimed)
	}

	var ledgerStatus, stateStatus string
	if err := testDB.Pool.QueryRow(ctx, `SELECT status FROM job_runs WHERE id = $1`, run.ID).Scan(&ledgerStatus); err != nil {
		t.Fatalf("ledger status: %v", err)
	}
	if err := testDB.Pool.QueryRow(ctx, `SELECT status FROM job_run_state WHERE run_id = $1`, run.ID).Scan(&stateStatus); err != nil {
		t.Fatalf("state status: %v", err)
	}
	if ledgerStatus != string(domain.StatusQueued) {
		t.Fatalf("ledger status = %q, want queued to avoid fat-row claim churn", ledgerStatus)
	}
	if stateStatus != string(domain.StatusDequeued) {
		t.Fatalf("state status = %q, want dequeued", stateStatus)
	}

	if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
		"started_at": time.Now(),
	}); err != nil {
		t.Fatalf("UpdateRunStatus via state: %v", err)
	}
	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != domain.StatusExecuting {
		t.Fatalf("GetRun status = %q, want executing from job_run_state", got.Status)
	}
}

func TestPgQue_StaleGenerationEventIsIgnored(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-stale-generation")
	q := mustPgQueQueue(t)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_run_state SET ready_generation = ready_generation + 1 WHERE run_id = $1`, run.ID); err != nil {
		t.Fatalf("bump ready generation: %v", err)
	}

	claimed, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("claimed stale generation event = %+v, want none", claimed)
	}
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
	if err := q.Enqueue(ctx, prodRun); err != nil {
		t.Fatalf("enqueue prod: %v", err)
	}
	stagingRun := &domain.JobRun{ID: newID(), JobID: stagingJob.ID, ProjectID: projectID, ExecutionMode: domain.ExecutionModeWorker, QueueName: "priority"}
	if err := q.Enqueue(ctx, stagingRun); err != nil {
		t.Fatalf("enqueue staging: %v", err)
	}

	stagingBatch, err := q.DequeueNForWorkerQueues(ctx, 1, []domain.WorkerQueueRef{{ProjectID: projectID, QueueName: "priority", EnvironmentID: stagingEnvID}})
	if err != nil {
		t.Fatalf("DequeueNForWorkerQueues(staging): %v", err)
	}
	if len(stagingBatch) != 1 || stagingBatch[0].ID != stagingRun.ID {
		t.Fatalf("staging batch = %+v, want staging run %s", stagingBatch, stagingRun.ID)
	}
	prodBatch, err := q.DequeueNForWorkerQueues(ctx, 1, []domain.WorkerQueueRef{{ProjectID: projectID, QueueName: "priority", EnvironmentID: prodEnvID}})
	if err != nil {
		t.Fatalf("DequeueNForWorkerQueues(prod): %v", err)
	}
	if len(prodBatch) != 1 || prodBatch[0].ID != prodRun.ID {
		t.Fatalf("prod batch = %+v, want prod run %s", prodBatch, prodRun.ID)
	}
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
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_run_state SET job_max_concurrency = 1 WHERE job_id = $1`, job.ID); err != nil {
		t.Fatalf("set max concurrency: %v", err)
	}

	claimed, err := q.DequeueN(ctx, 2)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d runs, want 1 due to max concurrency: %+v", len(claimed), claimed)
	}
}
