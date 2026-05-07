//go:build integration

package store_test

import (
	"context"
	"testing"

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
