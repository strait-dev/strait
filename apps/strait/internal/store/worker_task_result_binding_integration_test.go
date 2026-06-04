//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

func TestIntegration_MarkWorkerTaskResultReceivedByAssignment_BindsAndPersistsResult(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

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

func TestIntegration_ClaimRecoverableWorkerTaskResults_ClaimsOnlyOldExecutingHandoffs(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedWorkerTaskForBinding(t, ctx, q, 1)
	marked, err := q.MarkWorkerTaskResultReceivedByAssignment(ctx, taskID, workerID, projectID, runID, 1, "success", "", []byte(`{"ok":true}`), 7)
	if err != nil {
		t.Fatalf("MarkWorkerTaskResultReceivedByAssignment: %v", err)
	}
	if !marked {
		t.Fatal("expected exact handoff to be marked")
	}
	if _, err := env.DB.Pool.Exec(ctx, `UPDATE worker_tasks SET result_received_at = NOW() - INTERVAL '10 minutes' WHERE id = $1`, taskID); err != nil {
		t.Fatalf("age result handoff: %v", err)
	}

	claimed, err := q.ClaimRecoverableWorkerTaskResults(ctx, time.Now().Add(-5*time.Minute), 10)
	if err != nil {
		t.Fatalf("ClaimRecoverableWorkerTaskResults: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed = %d, want 1", len(claimed))
	}
	if claimed[0].ID != taskID || claimed[0].Status != domain.WorkerTaskStatusFinalizing {
		t.Fatalf("claimed task = %+v, want finalizing %s", claimed[0], taskID)
	}
	if claimed[0].Result == nil || claimed[0].Result.Status != "success" || claimed[0].Result.DurationMS != 7 {
		t.Fatalf("claimed durable result = %+v, want persisted success result", claimed[0].Result)
	}

	claimedAgain, err := q.ClaimRecoverableWorkerTaskResults(ctx, time.Now().Add(-5*time.Minute), 10)
	if err != nil {
		t.Fatalf("ClaimRecoverableWorkerTaskResults again: %v", err)
	}
	if len(claimedAgain) != 0 {
		t.Fatalf("claimedAgain = %d, want 0 while finalizing claim is held", len(claimedAgain))
	}

	if err := q.ResetWorkerTaskFinalizingToResultReceived(ctx, taskID); err != nil {
		t.Fatalf("ResetWorkerTaskFinalizingToResultReceived: %v", err)
	}
	task, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if task.Status != domain.WorkerTaskStatusResultReceived {
		t.Fatalf("task status after reset = %q, want result_received", task.Status)
	}
}

func TestIntegration_MarkWorkerTaskResultReceived_SkipsDuplicateHandoffWrites(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	_, _, _, taskID := seedWorkerTaskForBinding(t, ctx, q, 1)
	marked, err := q.MarkWorkerTaskResultReceived(ctx, taskID)
	if err != nil {
		t.Fatalf("MarkWorkerTaskResultReceived: %v", err)
	}
	if !marked {
		t.Fatal("MarkWorkerTaskResultReceived marked = false, want true")
	}

	var receivedXmin string
	if err := env.DB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM worker_tasks
		WHERE id = $1`,
		taskID,
	).Scan(&receivedXmin); err != nil {
		t.Fatalf("query result_received worker task version: %v", err)
	}

	markedAgain, err := q.MarkWorkerTaskResultReceived(ctx, taskID)
	if err != nil {
		t.Fatalf("MarkWorkerTaskResultReceived duplicate: %v", err)
	}
	if !markedAgain {
		t.Fatal("duplicate MarkWorkerTaskResultReceived marked = false, want true")
	}

	var duplicateReceivedXmin string
	if err := env.DB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM worker_tasks
		WHERE id = $1`,
		taskID,
	).Scan(&duplicateReceivedXmin); err != nil {
		t.Fatalf("query duplicate result_received worker task version: %v", err)
	}
	if duplicateReceivedXmin != receivedXmin {
		t.Fatalf("duplicate result_received handoff changed xmin from %s to %s", receivedXmin, duplicateReceivedXmin)
	}
}

func TestIntegration_UpdateWorkerTaskStatus_SkipsDuplicateStatusWrites(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	_, _, _, taskID := seedWorkerTaskForBinding(t, ctx, q, 1)
	if err := q.UpdateWorkerTaskStatus(ctx, taskID, domain.WorkerTaskStatusAccepted); err != nil {
		t.Fatalf("UpdateWorkerTaskStatus accepted: %v", err)
	}

	var acceptedXmin string
	var acceptedAt time.Time
	if err := env.DB.Pool.QueryRow(ctx, `
		SELECT xmin::text, accepted_at
		FROM worker_tasks
		WHERE id = $1`,
		taskID,
	).Scan(&acceptedXmin, &acceptedAt); err != nil {
		t.Fatalf("query accepted worker task version: %v", err)
	}

	if err := q.UpdateWorkerTaskStatus(ctx, taskID, domain.WorkerTaskStatusAccepted); err != nil {
		t.Fatalf("UpdateWorkerTaskStatus duplicate accepted: %v", err)
	}

	var duplicateAcceptedXmin string
	var duplicateAcceptedAt time.Time
	if err := env.DB.Pool.QueryRow(ctx, `
		SELECT xmin::text, accepted_at
		FROM worker_tasks
		WHERE id = $1`,
		taskID,
	).Scan(&duplicateAcceptedXmin, &duplicateAcceptedAt); err != nil {
		t.Fatalf("query duplicate accepted worker task version: %v", err)
	}
	if duplicateAcceptedXmin != acceptedXmin {
		t.Fatalf("duplicate accepted update changed xmin from %s to %s", acceptedXmin, duplicateAcceptedXmin)
	}
	if !duplicateAcceptedAt.Equal(acceptedAt) {
		t.Fatalf("duplicate accepted update changed accepted_at from %s to %s", acceptedAt, duplicateAcceptedAt)
	}

	if err := q.UpdateWorkerTaskStatus(ctx, taskID, domain.WorkerTaskStatusCompleted); err != nil {
		t.Fatalf("UpdateWorkerTaskStatus completed: %v", err)
	}

	var completedXmin string
	var finishedAt time.Time
	if err := env.DB.Pool.QueryRow(ctx, `
		SELECT xmin::text, finished_at
		FROM worker_tasks
		WHERE id = $1`,
		taskID,
	).Scan(&completedXmin, &finishedAt); err != nil {
		t.Fatalf("query completed worker task version: %v", err)
	}
	if completedXmin == duplicateAcceptedXmin {
		t.Fatalf("completed transition kept xmin %s, want a real update", completedXmin)
	}

	if err := q.UpdateWorkerTaskStatus(ctx, taskID, domain.WorkerTaskStatusCompleted); err != nil {
		t.Fatalf("UpdateWorkerTaskStatus duplicate completed: %v", err)
	}

	var duplicateCompletedXmin string
	var duplicateFinishedAt time.Time
	if err := env.DB.Pool.QueryRow(ctx, `
		SELECT xmin::text, finished_at
		FROM worker_tasks
		WHERE id = $1`,
		taskID,
	).Scan(&duplicateCompletedXmin, &duplicateFinishedAt); err != nil {
		t.Fatalf("query duplicate completed worker task version: %v", err)
	}
	if duplicateCompletedXmin != completedXmin {
		t.Fatalf("duplicate completed update changed xmin from %s to %s", completedXmin, duplicateCompletedXmin)
	}
	if !duplicateFinishedAt.Equal(finishedAt) {
		t.Fatalf("duplicate completed update changed finished_at from %s to %s", finishedAt, duplicateFinishedAt)
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
	executing := domain.StatusExecuting
	run := testutil.BuildRun(job, &testutil.RunOpts{ID: &runID, Status: &executing})
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
