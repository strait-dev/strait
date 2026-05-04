//go:build integration

package grpc

import (
	"context"
	"testing"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

// TestIntegration_Reconcile_CompletedUpdatesWorkerTask asserts that
// reconcileInFlightTasks transitions the worker_tasks row to "completed"
// in addition to the run row — regression for the same-class bug as the
// fallback path.
func TestIntegration_Reconcile_CompletedUpdatesWorkerTask(t *testing.T) {
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
	_, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	tasks := []*workerv1.InFlightTask{{RunId: runID, Status: "completed"}}
	svc.reconcileInFlightTasks(ctx, workerID, "", tasks)

	got, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if got.Status != domain.WorkerTaskStatusCompleted {
		t.Fatalf("worker_tasks not transitioned: got %q, want completed", got.Status)
	}
	run, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != domain.StatusCompleted {
		t.Fatalf("run not transitioned: got %q, want completed", run.Status)
	}
}

// TestIntegration_Reconcile_FailedUpdatesWorkerTask asserts the failed/abandoned
// branch transitions the worker_tasks row to "failed" so it doesn't linger in
// "assigned" after the run is requeued or dead-lettered.
func TestIntegration_Reconcile_FailedUpdatesWorkerTask(t *testing.T) {
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
	_, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	tasks := []*workerv1.InFlightTask{{RunId: runID, Status: "failed", ErrorMessage: "boom"}}
	svc.reconcileInFlightTasks(ctx, workerID, "", tasks)

	got, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if got.Status != domain.WorkerTaskStatusFailed {
		t.Fatalf("worker_tasks not transitioned: got %q, want failed", got.Status)
	}
}

// TestIntegration_Reconcile_OwnershipMismatchSkips guards the adversarial path:
// a worker reporting an in-flight run it doesn't own (no matching worker_tasks
// row) must NOT touch either the run or any worker_tasks row.
func TestIntegration_Reconcile_OwnershipMismatchSkips(t *testing.T) {
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
	_, _, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	tasks := []*workerv1.InFlightTask{{RunId: runID, Status: "completed"}}
	// Use a worker ID that has no worker_tasks row for runID.
	svc.reconcileInFlightTasks(ctx, "impostor-worker", "", tasks)

	got, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if got.Status != domain.WorkerTaskStatusAssigned {
		t.Fatalf("ownership mismatch should not touch worker_task: got %q", got.Status)
	}
	run, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != domain.StatusExecuting {
		t.Fatalf("ownership mismatch should not touch run: got %q", run.Status)
	}
}

// TestIntegration_Reconcile_UnknownStatusIgnored asserts that a malformed
// status string ("zombie") is logged and skipped — never producing a panic
// or a partial state transition.
func TestIntegration_Reconcile_UnknownStatusIgnored(t *testing.T) {
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
	_, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	tasks := []*workerv1.InFlightTask{{RunId: runID, Status: "zombie"}}
	svc.reconcileInFlightTasks(ctx, workerID, "", tasks)

	got, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if got.Status != domain.WorkerTaskStatusAssigned {
		t.Fatalf("unknown status should not transition worker_task: got %q", got.Status)
	}
}

// TestIntegration_Reconcile_NilAndEmptyEntries asserts the loop tolerates
// nil entries and empty RunId values without panicking or affecting state.
func TestIntegration_Reconcile_NilAndEmptyEntries(t *testing.T) {
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
	_, workerID, _, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	tasks := []*workerv1.InFlightTask{
		nil,
		{RunId: "", Status: "completed"},
	}
	svc.reconcileInFlightTasks(ctx, workerID, "", tasks)

	got, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if got.Status != domain.WorkerTaskStatusAssigned {
		t.Fatalf("nil/empty entries should not affect state: got %q", got.Status)
	}
}
