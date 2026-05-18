//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

func TestIntegration_MarkWorkerTaskResultReceivedByAssignment_BindsAndPersistsResult(t *testing.T) {
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
	projectID, workerID, runID, taskID := seedWorkerTaskForBinding(t, ctx, q, 3)

	staleChecks := []struct {
		name      string
		taskID    string
		workerID  string
		projectID string
		runID     string
		attempt   int
	}{
		{name: "wrong task", taskID: uuid.Must(uuid.NewV7()).String(), workerID: workerID, projectID: projectID, runID: runID, attempt: 3},
		{name: "wrong worker", taskID: taskID, workerID: "worker-other", projectID: projectID, runID: runID, attempt: 3},
		{name: "wrong project", taskID: taskID, workerID: workerID, projectID: "project-other", runID: runID, attempt: 3},
		{name: "wrong run", taskID: taskID, workerID: workerID, projectID: projectID, runID: uuid.Must(uuid.NewV7()).String(), attempt: 3},
		{name: "wrong attempt", taskID: taskID, workerID: workerID, projectID: projectID, runID: runID, attempt: 2},
		{name: "missing attempt", taskID: taskID, workerID: workerID, projectID: projectID, runID: runID, attempt: 0},
	}
	for _, tc := range staleChecks {
		t.Run(tc.name, func(t *testing.T) {
			marked, err := q.MarkWorkerTaskResultReceivedByAssignment(
				ctx,
				tc.taskID,
				tc.workerID,
				tc.projectID,
				tc.runID,
				tc.attempt,
				"success",
				"",
				[]byte(`{"stale":true}`),
				11,
			)
			if err != nil {
				t.Fatalf("MarkWorkerTaskResultReceivedByAssignment: %v", err)
			}
			if marked {
				t.Fatal("stale assignment identity must not mark the task")
			}
		})
	}

	marked, err := q.MarkWorkerTaskResultReceivedByAssignment(
		ctx,
		taskID,
		workerID,
		projectID,
		runID,
		3,
		"success",
		"",
		[]byte(`{"ok":true}`),
		42,
	)
	if err != nil {
		t.Fatalf("MarkWorkerTaskResultReceivedByAssignment exact: %v", err)
	}
	if !marked {
		t.Fatal("exact assignment identity should mark the task")
	}

	task, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if task.Status != domain.WorkerTaskStatusResultReceived {
		t.Fatalf("status = %q, want result_received", task.Status)
	}
	if task.Attempt != 3 {
		t.Fatalf("attempt = %d, want 3", task.Attempt)
	}
	if task.Result == nil {
		t.Fatal("expected durable worker result")
	}
	if task.Result.Status != "success" || task.Result.DurationMS != 42 || task.Result.ReceivedAt == nil {
		t.Fatalf("unexpected durable worker result metadata: %+v", task.Result)
	}
	var payload map[string]bool
	if err := json.Unmarshal(task.Result.Output, &payload); err != nil {
		t.Fatalf("unmarshal durable output: %v", err)
	}
	if !payload["ok"] {
		t.Fatalf("durable output = %s, want ok=true", string(task.Result.Output))
	}
}

func seedWorkerTaskForBinding(t *testing.T, ctx context.Context, q *store.Queries, attempt int) (projectID, workerID, runID, taskID string) {
	t.Helper()

	projectID = "project-" + uuid.Must(uuid.NewV7()).String()
	workerID = "worker-" + uuid.Must(uuid.NewV7()).String()
	runID = uuid.Must(uuid.NewV7()).String()
	taskID = uuid.Must(uuid.NewV7()).String()

	if err := q.CreateProject(ctx, &domain.Project{
		ID:    projectID,
		OrgID: "org-worker-binding",
		Name:  "worker-binding",
	}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	run := testutil.BuildRun(job, &testutil.RunOpts{ID: &runID})
	run.Attempt = attempt
	run.ExecutionMode = domain.ExecutionModeWorker
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
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
	if err := q.CreateWorkerTask(ctx, &domain.WorkerTask{
		ID:        taskID,
		WorkerID:  workerID,
		RunID:     runID,
		ProjectID: projectID,
		Attempt:   attempt,
		Status:    domain.WorkerTaskStatusAssigned,
	}); err != nil {
		t.Fatalf("CreateWorkerTask: %v", err)
	}
	return projectID, workerID, runID, taskID
}
