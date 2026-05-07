//go:build integration

package grpc

import (
	"context"
	"testing"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

// TestIntegration_FinalizeDisconnect_MarksOfflineAndAudits pins the Phase G
// contract: the disconnect cleanup path must (a) flip the workers row to
// `offline` and (b) write a worker.disconnected audit event, even though the
// stream's request context has been cancelled.
//
// Pre-fix the deferred block reused the cancelled stream ctx, so neither the
// audit insert nor any status transition reached Postgres. The fix uses a
// detached context with a 5s timeout.
func TestIntegration_FinalizeDisconnect_MarksOfflineAndAudits(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

	q := store.New(env.DB.Pool)

	const workerID = "disco-worker"
	const projectID = "proj-disco"

	if err := q.RegisterWorker(ctx, &domain.Worker{
		ID:        workerID,
		ProjectID: projectID,
		QueueName: "q",
		Hostname:  "host",
		Version:   "1.0",
		Status:    domain.WorkerStatusActive,
	}); err != nil {
		t.Fatalf("seed worker: %v", err)
	}

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
	}

	// finalizeDisconnect deliberately takes no ctx — it must allocate its own
	// detached context internally so it remains effective when the stream
	// ctx is already cancelled at the time the deferred cleanup fires.
	svc.finalizeDisconnect(projectID, workerID)

	// Workers row must now be offline.
	var status string
	if err := env.DB.Pool.QueryRow(ctx,
		`SELECT status FROM workers WHERE id = $1`, workerID,
	).Scan(&status); err != nil {
		t.Fatalf("read worker status: %v", err)
	}
	if status != string(domain.WorkerStatusOffline) {
		t.Errorf("worker status = %q, want %q", status, domain.WorkerStatusOffline)
	}

	// Audit event must have landed.
	var auditCount int
	if err := env.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events
		 WHERE resource_type = 'worker' AND resource_id = $1 AND action = $2`,
		workerID, domain.AuditActionWorkerDisconnected,
	).Scan(&auditCount); err != nil {
		t.Fatalf("read audit events: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("expected 1 worker.disconnected audit event, got %d", auditCount)
	}
}

// TestIntegration_FinalizeDisconnect_RequeuesOpenWorkerRuns verifies that
// disconnect cleanup requeues in-flight worker-mode runs and closes out their
// worker_tasks rows instead of waiting for the generic stale-run reaper.
func TestIntegration_FinalizeDisconnect_RequeuesOpenWorkerRuns(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
	}

	svc.finalizeDisconnect(projectID, workerID)

	run, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != domain.StatusQueued {
		t.Fatalf("run status = %q, want queued", run.Status)
	}
	if run.StartedAt != nil {
		t.Fatalf("run.StartedAt = %v, want nil after requeue", run.StartedAt)
	}
	if run.FinishedAt != nil {
		t.Fatalf("run.FinishedAt = %v, want nil after requeue", run.FinishedAt)
	}
	if run.HeartbeatAt != nil {
		t.Fatalf("run.HeartbeatAt = %v, want nil after requeue", run.HeartbeatAt)
	}
	if run.Error != "worker disconnected before reporting result" {
		t.Fatalf("run.Error = %q, want disconnect reason", run.Error)
	}

	task, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if task.Status != domain.WorkerTaskStatusFailed {
		t.Fatalf("worker task status = %q, want failed", task.Status)
	}
	if task.FinishedAt == nil || task.FinishedAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatalf("worker task FinishedAt = %v, want recent timestamp", task.FinishedAt)
	}
}

// TestIntegration_FinalizeDisconnect_SkipsResultReceivedWorkerRuns verifies
// that disconnect cleanup cannot requeue a run after the API has already
// received the worker result but before executor finalization has completed.
func TestIntegration_FinalizeDisconnect_SkipsResultReceivedWorkerRuns(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	if marked, err := q.MarkWorkerTaskResultReceived(ctx, taskID); err != nil {
		t.Fatalf("MarkWorkerTaskResultReceived: %v", err)
	} else if !marked {
		t.Fatal("MarkWorkerTaskResultReceived marked = false, want true")
	}

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
	}

	svc.finalizeDisconnect(projectID, workerID)

	run, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != domain.StatusExecuting {
		t.Fatalf("run status = %q, want executing", run.Status)
	}
	if run.Error != "" {
		t.Fatalf("run.Error = %q, want empty", run.Error)
	}

	task, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if task.Status != domain.WorkerTaskStatusResultReceived {
		t.Fatalf("worker task status = %q, want result_received", task.Status)
	}
	if task.FinishedAt != nil {
		t.Fatalf("worker task FinishedAt = %v, want nil before finalization", task.FinishedAt)
	}
}

// TestIntegration_TaskResultHandoffPrecedesDisconnectRequeue verifies the
// stream recv path marks the worker_task non-requeueable before delivering the
// buffered TaskResult to WorkerDispatch. This pins the race where a worker
// disconnect immediately after sending a result could requeue completed work.
func TestIntegration_TaskResultHandoffPrecedesDisconnectRequeue(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	resultChannels := NewResultChannelRegistry()
	resultCh := resultChannels.Register(runID, projectID, workerID)
	t.Cleanup(func() { resultChannels.Deregister(runID) })

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       NewConnectionRegistry(),
		resultChannels: resultChannels,
	}

	if err := svc.handleTaskResult(ctx, workerID, projectID, &workerv1.TaskResult{
		RunId:      runID,
		Status:     "success",
		OutputJson: []byte(`{"ok":true}`),
	}); err != nil {
		t.Fatalf("handleTaskResult: %v", err)
	}

	task, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if task.Status != domain.WorkerTaskStatusResultReceived {
		t.Fatalf("worker task status after stream handoff = %q, want result_received", task.Status)
	}

	svc.finalizeDisconnect(projectID, workerID)

	run, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != domain.StatusExecuting {
		t.Fatalf("run status after disconnect = %q, want executing", run.Status)
	}

	select {
	case got := <-resultCh:
		if got == nil || got.RunId != runID || got.Status != "success" {
			t.Fatalf("delivered result = %#v, want success for run %s", got, runID)
		}
	default:
		t.Fatal("expected buffered result to be delivered to dispatcher channel")
	}
}

// TestIntegration_CleanupRegistration_StaleReconnectDoesNotRequeue verifies
// that a stale stream from a same-ID reconnect cannot run disconnect cleanup
// for the replacement connection.
func TestIntegration_CleanupRegistration_StaleReconnectDoesNotRequeue(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)

	reg := NewConnectionRegistry()
	oldWorker := registerWorkerInRegistry(t, reg, workerID, projectID, 1)
	oldToken := oldWorker.regToken
	newWorker := &ConnectedWorker{
		WorkerID:       workerID,
		ProjectID:      projectID,
		APIKeyID:       oldWorker.APIKeyID,
		Queues:         []string{"default"},
		SlotsTotal:     1,
		SlotsAvailable: 1,
		Status:         "active",
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}
	if err := reg.Register(newWorker); err != nil {
		t.Fatalf("reconnect register: %v", err)
	}

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       reg,
		resultChannels: NewResultChannelRegistry(),
	}

	svc.cleanupRegistration(projectID, workerID, oldToken)

	run, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != domain.StatusExecuting {
		t.Fatalf("stale cleanup changed run status to %q, want executing", run.Status)
	}

	task, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if task.Status != domain.WorkerTaskStatusAssigned {
		t.Fatalf("stale cleanup changed task status to %q, want assigned", task.Status)
	}

	worker, err := q.GetWorker(ctx, workerID, projectID)
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if worker.Status != domain.WorkerStatusActive {
		t.Fatalf("stale cleanup changed worker status to %q, want active", worker.Status)
	}
}
