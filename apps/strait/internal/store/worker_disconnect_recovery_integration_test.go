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
	env, err := testutil.SetupTestEnv(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

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

func TestIntegration_RequeueOpenWorkerTasks_SkipsResultReceivedRuns(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

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
	env, err := testutil.SetupTestEnv(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

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
	gotTask, err := q.GetWorkerTask(ctx, "task-stale-recovery")
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if gotTask.Status != domain.WorkerTaskStatusFailed {
		t.Fatalf("worker task status = %q, want failed", gotTask.Status)
	}
}

func TestIntegration_DeepSecRecoverStaleWorkerTasksExcept_SkipsConnectedWorker(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

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

	count, err := q.RecoverStaleWorkerTasksExcept(ctx, time.Now().Add(-5*time.Minute), "stale worker heartbeat", []string{workerID})
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

	evicted, err := q.EvictStaleWorkersExcept(ctx, time.Now().Add(-5*time.Minute), []string{workerID})
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

func TestIntegration_DeepSecRecoverStaleWorkerTasks_SkipsFutureStreamLease(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

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
	env, err := testutil.SetupTestEnv(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

	q := store.New(env.DB.Pool)
	_, err = env.DB.Pool.Exec(ctx, `
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
