//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

// TestIntegration_ListWorkerTasksByWorker_ProjectFilter pins the Phase H
// contract: ListWorkerTasksByWorker scopes by project_id, so a wrong-project
// query returns no rows even when the worker_id matches existing task rows.
//
// Pre-fix the query joined only on worker_id, relying entirely on the
// handler-layer GetWorker(project) check. Layered defense at the SQL boundary
// prevents any future caller — or any future cross-project worker_id artifact
// — from leaking tasks across projects.
func TestIntegration_ListWorkerTasksByWorker_ProjectFilter(t *testing.T) {
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

	const workerID = "shared-id"
	const projectA = "proj-A"
	const projectB = "proj-B"

	if err := q.RegisterWorker(ctx, &domain.Worker{
		ID:        workerID,
		ProjectID: projectA,
		QueueName: "q",
		Hostname:  "host-a",
		Version:   "1.0",
		Status:    domain.WorkerStatusActive,
	}); err != nil {
		t.Fatalf("register worker: %v", err)
	}

	// Two tasks for project A under this worker.
	for _, taskID := range []string{"task-a1", "task-a2"} {
		if err := q.CreateWorkerTask(ctx, &domain.WorkerTask{
			ID:        taskID,
			WorkerID:  workerID,
			RunID:     "run-" + taskID,
			ProjectID: projectA,
			Status:    domain.WorkerTaskStatusAssigned,
		}); err != nil {
			t.Fatalf("create task %s: %v", taskID, err)
		}
	}

	// Project A query returns both.
	gotA, err := q.ListWorkerTasksByWorker(ctx, workerID, projectA, "", 100, 0)
	if err != nil {
		t.Fatalf("list project A: %v", err)
	}
	if len(gotA) != 2 {
		t.Errorf("project A: got %d tasks, want 2", len(gotA))
	}

	// Project B query returns zero, even though worker_id matches.
	gotB, err := q.ListWorkerTasksByWorker(ctx, workerID, projectB, "", 100, 0)
	if err != nil {
		t.Fatalf("list project B: %v", err)
	}
	if len(gotB) != 0 {
		t.Errorf("project B: got %d tasks, want 0 (cross-project leak)", len(gotB))
	}
}
