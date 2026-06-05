//go:build integration

package grpc

import (
	"context"
	"encoding/json"
	"testing"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// TestIntegration_Reconcile_CompletedUpdatesWorkerTask asserts that
// reconcileInFlightTasks transitions the worker_tasks row to "completed"
// in addition to the run row — regression for the same-class bug as the
// fallback path.
func TestIntegration_Reconcile_CompletedUpdatesWorkerTask(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	tasks := []*workerv1.InFlightTask{{RunId: runID, AssignmentId: taskID, Attempt: 1, Status: "completed", OutputJson: []byte(`{"recovered":true}`)}}
	svc.reconcileInFlightTasks(ctx, workerID, projectID, tasks)

	got, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusCompleted,

		got.
			Status)

	run, err := q.GetRun(ctx, runID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			StatusCompleted,

		run.Status)

	var result map[string]bool
	require.NoError(t,

		json.Unmarshal(run.
			Result, &result))
	require.True(t, result["recovered"])

}

// TestIntegration_Reconcile_FailedUpdatesWorkerTask asserts the failed/abandoned
// branch transitions the worker_tasks row to "failed" so it doesn't linger in
// "assigned" after the run is requeued or dead-lettered.
func TestIntegration_Reconcile_FailedUpdatesWorkerTask(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	tasks := []*workerv1.InFlightTask{{RunId: runID, AssignmentId: taskID, Attempt: 1, Status: "failed", ErrorMessage: "boom"}}
	svc.reconcileInFlightTasks(ctx, workerID, projectID, tasks)

	got, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusFailed,

		got.Status,
	)

}

func TestIntegration_Reconcile_CompletedUsesRunFinalizer(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	finalizer := &recordingRunFinalizer{}
	svc := fallbackServiceWithFinalizer(q, finalizer)

	tasks := []*workerv1.InFlightTask{{RunId: runID, AssignmentId: taskID, Attempt: 1, Status: "completed", OutputJson: []byte(`{"recovered":true}`)}}
	svc.reconcileInFlightTasks(ctx, workerID, projectID, tasks)
	require.Len(t, finalizer.
		calls,
		1)

	call := finalizer.calls[0]
	require.False(t,
		call.
			runID !=
			runID ||
			call.status !=
				"success" ||

			string(call.output) != `{"recovered":true}`)

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusCompleted,

		task.
			Status)

	run, err := q.GetRun(ctx, runID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			StatusExecuting,

		run.Status)

}

func TestIntegration_Reconcile_InvalidCompletedOutputRoutesFailure(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	finalizer := &recordingRunFinalizer{}
	svc := fallbackServiceWithFinalizer(q, finalizer)

	tasks := []*workerv1.InFlightTask{{RunId: runID, AssignmentId: taskID, Attempt: 1, Status: "completed", OutputJson: []byte(`{"recovered":`)}}
	svc.reconcileInFlightTasks(ctx, workerID, projectID, tasks)
	require.Len(t, finalizer.
		calls,
		1)

	call := finalizer.calls[0]
	require.False(t,
		call.
			runID !=
			runID ||
			call.status !=
				"failed" ||

			call.errorMessage != invalidWorkerOutputError || call.
			output != nil)

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusFailed,

		task.Status,
	)

}

func TestIntegration_Reconcile_FailedUsesRunFinalizer(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	finalizer := &recordingRunFinalizer{}
	svc := fallbackServiceWithFinalizer(q, finalizer)

	tasks := []*workerv1.InFlightTask{{RunId: runID, AssignmentId: taskID, Attempt: 1, Status: "failed", ErrorMessage: "boom"}}
	svc.reconcileInFlightTasks(ctx, workerID, projectID, tasks)
	require.Len(t, finalizer.
		calls,
		1)

	call := finalizer.calls[0]
	require.False(t,
		call.
			runID !=
			runID ||
			call.status !=
				"failed" ||

			call.errorMessage != "boom")

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusFailed,

		task.Status,
	)

	run, err := q.GetRun(ctx, runID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			StatusExecuting,

		run.Status)

}

// TestIntegration_Reconcile_OwnershipMismatchSkips guards the adversarial path:
// a worker reporting an in-flight run it doesn't own (no matching worker_tasks
// row) must NOT touch either the run or any worker_tasks row.
func TestIntegration_Reconcile_OwnershipMismatchSkips(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, _, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	tasks := []*workerv1.InFlightTask{{RunId: runID, AssignmentId: taskID, Attempt: 1, Status: "completed"}}
	// Use a worker ID that has no worker_tasks row for runID.
	svc.reconcileInFlightTasks(ctx, "impostor-worker", projectID, tasks)

	got, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusAssigned,

		got.Status,
	)

	run, err := q.GetRun(ctx, runID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			StatusExecuting,

		run.Status)

}

func TestIntegration_Reconcile_StaleAssignmentReplaySkipsCurrentTask(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, staleTaskID := seedRunWithTask(t, ctx, q, env)
	require.NoError(t,

		q.UpdateWorkerTaskStatus(ctx, staleTaskID,
			domain.
				WorkerTaskStatusFailed))

	const currentTaskID = "task-current-assignment"
	require.NoError(t,

		q.CreateWorkerTask(
			ctx, &domain.WorkerTask{ID: currentTaskID,
				WorkerID: workerID, ProjectID: projectID,
				RunID: runID, Attempt: 2, Status: domain.WorkerTaskStatusAssigned,
			}))

	svc := fallbackService(q)

	tasks := []*workerv1.InFlightTask{{
		RunId:        runID,
		AssignmentId: staleTaskID,
		Attempt:      1,
		Status:       "completed",
		OutputJson:   []byte(`{"stale":true}`),
	}}
	svc.reconcileInFlightTasks(ctx, workerID, projectID, tasks)

	current, err := q.GetWorkerTask(ctx, currentTaskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusAssigned,

		current.
			Status)

	run, err := q.GetRun(ctx, runID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			StatusExecuting,

		run.Status)

}

// TestIntegration_Reconcile_UnknownStatusIgnored asserts that a malformed
// status string ("zombie") is logged and skipped — never producing a panic
// or a partial state transition.
func TestIntegration_Reconcile_UnknownStatusIgnored(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	tasks := []*workerv1.InFlightTask{{RunId: runID, AssignmentId: taskID, Attempt: 1, Status: "zombie"}}
	svc.reconcileInFlightTasks(ctx, workerID, projectID, tasks)

	got, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusAssigned,

		got.Status,
	)

}

// TestIntegration_Reconcile_NilAndEmptyEntries asserts the loop tolerates
// nil entries and empty RunId values without panicking or affecting state.
func TestIntegration_Reconcile_NilAndEmptyEntries(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, _, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	tasks := []*workerv1.InFlightTask{
		nil,
		{RunId: "", Status: "completed"},
	}
	svc.reconcileInFlightTasks(ctx, workerID, projectID, tasks)

	got, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusAssigned,

		got.Status,
	)

}
