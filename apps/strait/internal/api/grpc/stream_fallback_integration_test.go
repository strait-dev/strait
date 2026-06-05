//go:build integration

package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

// seedRunWithTask inserts a project, job, executing run, and an assigned
// worker_tasks row so handleTaskResult fallback has a real graph to operate
// on. Returns the IDs needed by the assertion.
func seedRunWithTask(t *testing.T, ctx context.Context, q *store.Queries, env *testutil.TestEnv) (projectID, workerID, runID, taskID string) {
	t.Helper()

	projectID = "proj-" + uuid.Must(uuid.NewV7()).String()
	workerID = "worker-" + uuid.Must(uuid.NewV7()).String()
	runID = uuid.Must(uuid.NewV7()).String()
	taskID = uuid.Must(uuid.NewV7()).String()
	require.NoError(t,

		q.CreateProject(ctx,
			&domain.Project{ID: projectID,
				OrgID: "org-1",
				Name:  "fallback-test",
			}))

	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{
		ProjectID: &projectID,
	})

	// Insert a run in StatusExecuting (matches the precondition for
	// UpdateRunStatus from executing -> terminal).
	executing := domain.StatusExecuting
	run := testutil.BuildRun(job, &testutil.RunOpts{
		ID:     &runID,
		Status: &executing,
	})
	run.ExecutionMode = domain.ExecutionModeWorker
	require.NoError(t,

		q.CreateRun(ctx, run))

	// CreateRun may set its own initial status — force executing for the test.
	if err := q.UpdateRunStatus(ctx, runID, run.Status, domain.StatusExecuting, map[string]any{
		"started_at": time.Now(),
	}); err != nil {
		// Already-executing or matching state is fine.
		t.Logf("UpdateRunStatus to executing returned: %v (continuing)", err)
	}
	require.NoError(t,

		q.RegisterWorker(ctx,
			&domain.Worker{ID: workerID,
				ProjectID: projectID,
				QueueName: "default", Hostname: "h",

				Version: "1.0", Status: domain.WorkerStatusActive},
		))
	require.NoError(t,

		q.CreateWorkerTask(
			ctx, &domain.WorkerTask{ID: taskID, WorkerID: workerID,
				RunID: runID, ProjectID: projectID,

				Status: domain.WorkerTaskStatusAssigned}))

	// Insert worker row so worker_tasks FK (if any) is satisfied.

	return projectID, workerID, runID, taskID
}

func assignedTaskResult(runID, taskID, status string) *workerv1.TaskResult {
	return &workerv1.TaskResult{
		RunId:        runID,
		Status:       status,
		AssignmentId: taskID,
		Attempt:      1,
	}
}

// fallbackService builds a workerService wired to a real DB / no-op pub for
// fallback-path tests. resultChannels is intentionally nil so every
// TaskResult lands on the fallback branch.
func fallbackService(q *store.Queries) *workerService {
	return &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(), // never registered for the runID
	}
}

type finalizerCall struct {
	runID        string
	status       string
	errorMessage string
	output       json.RawMessage
}

type recordingRunFinalizer struct {
	calls      []finalizerCall
	taskStatus domain.WorkerTaskStatus
	err        error
}

func (f *recordingRunFinalizer) FinalizeWorkerRunResult(_ context.Context, runID, status, errorMessage string, output json.RawMessage) (domain.WorkerTaskStatus, error) {
	copied := json.RawMessage(nil)
	if len(output) > 0 {
		copied = append(json.RawMessage(nil), output...)
	}
	f.calls = append(f.calls, finalizerCall{
		runID:        runID,
		status:       status,
		errorMessage: errorMessage,
		output:       copied,
	})
	if f.err != nil {
		return "", f.err
	}
	if f.taskStatus != "" {
		return f.taskStatus, nil
	}
	if status == "success" {
		return domain.WorkerTaskStatusCompleted, nil
	}
	return domain.WorkerTaskStatusFailed, nil
}

func fallbackServiceWithFinalizer(q *store.Queries, finalizer WorkerRunResultFinalizer) *workerService {
	var value atomic.Value
	value.Store(finalizer)
	svc := fallbackService(q)
	svc.runFinalizer = &value
	return svc
}

// TestIntegration_HandleTaskResult_Fallback_SuccessUpdatesWorkerTask asserts
// that a late TaskResult arriving when no dispatcher is waiting transitions
// the worker_tasks row to "completed", not just the run row. Regression for
// the bug where the fallback only updated runs and left tasks stuck in
// "assigned".
func TestIntegration_HandleTaskResult_Fallback_SuccessUpdatesWorkerTask(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)

	svc := fallbackService(q)

	tr := assignedTaskResult(runID, taskID, "success")
	tr.OutputJson = []byte(`{"worker":"result"}`)
	require.NoError(t,

		svc.handleTaskResult(ctx, workerID,
			projectID,
			tr))

	// Worker task must now be completed.
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

	var result map[string]string
	require.NoError(t,

		json.Unmarshal(run.
			Result, &result))
	require.Equal(t,
		"result",

		result["worker"])

}

func TestIntegration_HandleTaskResult_Fallback_DoesNotCompleteTaskWhenRunUpdateFails(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	if _, err := env.DB.Pool.Exec(ctx, `
		UPDATE job_runs
		SET status = $1, finished_at = NOW(), result = $2::jsonb
		WHERE id = $3
	`, domain.StatusCanceled, json.RawMessage(`{"already":true}`), runID); err != nil {
		require.Failf(t, "test failure",

			"force run terminal: %v", err)
	}

	svc := fallbackService(q)
	tr := assignedTaskResult(runID, taskID, "success")
	tr.OutputJson = []byte(`{"worker":"late"}`)
	require.NoError(t,

		svc.handleTaskResult(ctx, workerID,
			projectID,
			tr))

	got, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusAssigned,

		got.Status,
	)

}

func TestIntegration_HandleTaskResult_Fallback_UsesRunFinalizerForSuccess(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	finalizer := &recordingRunFinalizer{}
	svc := fallbackServiceWithFinalizer(q, finalizer)

	tr := assignedTaskResult(runID, taskID, "success")
	tr.OutputJson = []byte(`{"worker":"result"}`)
	require.NoError(t,

		svc.handleTaskResult(ctx, workerID,
			projectID,
			tr))
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
			string(call.
				output) !=
				`{"worker":"result"}`,
	)

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

func TestIntegration_HandleTaskResult_Fallback_InvalidSuccessOutputRoutesFailure(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	finalizer := &recordingRunFinalizer{}
	svc := fallbackServiceWithFinalizer(q, finalizer)

	tr := assignedTaskResult(runID, taskID, "success")
	tr.OutputJson = []byte(`{"worker":`)
	require.NoError(t,

		svc.handleTaskResult(ctx, workerID,
			projectID,
			tr))
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
			call.errorMessage !=
				invalidWorkerOutputError ||
			call.output !=
				nil)

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusFailed,

		task.Status,
	)

}

func TestIntegration_HandleTaskResult_Fallback_FinalizerErrorLeavesTaskOpen(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackServiceWithFinalizer(q, &recordingRunFinalizer{err: errors.New("finalizer failed")})

	tr := assignedTaskResult(runID, taskID, "success")
	require.NoError(t,

		svc.handleTaskResult(ctx, workerID,
			projectID,
			tr))

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusAssigned,

		task.
			Status)

}

func TestIntegration_HandleAck_MarksOpenWorkerTaskAccepted(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	msg := &workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_Ack{
			Ack: &workerv1.Acknowledged{Id: runID},
		},
	}
	require.NoError(t,

		svc.handleWorkerMessage(ctx, workerID,
			projectID,
			"", "", msg,
		))

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusAccepted,

		task.
			Status)
	require.NotNil(t,

		task.AcceptedAt,
	)

}

// TestIntegration_HandleTaskResult_Fallback_FailedUpdatesWorkerTask asserts the
// failure variant of the fallback path transitions the worker_tasks row to
// "failed".
func TestIntegration_HandleTaskResult_Fallback_FailedUpdatesWorkerTask(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	tr := assignedTaskResult(runID, taskID, "failed")
	tr.ErrorMessage = "boom"
	require.NoError(t,

		svc.handleTaskResult(ctx, workerID,
			projectID,
			tr))

	got, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusFailed,

		got.Status,
	)

}

// TestIntegration_HandleTaskResult_Fallback_ProjectMismatchRejects guards the
// adversarial path: a worker authenticated to a different project must NOT
// touch the run or worker_tasks row.
func TestIntegration_HandleTaskResult_Fallback_ProjectMismatchRejects(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	_, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	// Use the WRONG project ID — simulates a stream authenticated to project B
	// trying to mark a run that belongs to project A.
	tr := assignedTaskResult(runID, taskID, "success")
	require.NoError(t,

		svc.handleTaskResult(ctx, workerID,
			"proj-impostor",
			tr))

	got, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusAssigned,

		got.Status,
	)

}

// TestIntegration_HandleTaskResult_Fallback_OwnershipMismatchRejects guards
// against a worker reporting a result for a run it was never assigned. With
// no worker_tasks row matching (workerID, runID), the fallback must not
// touch the run row.
func TestIntegration_HandleTaskResult_Fallback_OwnershipMismatchRejects(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, _, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	// A different worker, in the same project, reports a result it doesn't own.
	otherWorker := "other-worker-" + uuid.Must(uuid.NewV7()).String()
	require.NoError(t,

		q.RegisterWorker(ctx,
			&domain.Worker{ID: otherWorker,
				ProjectID: projectID,
				QueueName: "default", Hostname: "h", Version: "1.0", Status: domain.WorkerStatusActive,
			}))

	tr := assignedTaskResult(runID, taskID, "success")
	require.NoError(t,

		svc.handleTaskResult(ctx, otherWorker,
			projectID,
			tr))

	// Original worker_task must remain assigned (not touched by the impostor).
	got, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusAssigned,

		got.Status,
	)

}

// TestIntegration_HandleTaskResult_Fallback_ClosedAssignmentRejects verifies
// that historical worker_tasks rows do not grant ownership after a disconnect
// requeue. A stale worker result must not complete a run now owned by another
// live assignment.
func TestIntegration_HandleTaskResult_Fallback_ClosedAssignmentRejects(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)
	require.NoError(t,

		q.UpdateWorkerTaskStatus(ctx, taskID,
			domain.WorkerTaskStatusFailed,
		))

	tr := assignedTaskResult(runID, taskID, "success")
	require.NoError(t,

		svc.handleTaskResult(ctx, workerID,
			projectID,
			tr))

	run, err := q.GetRun(ctx, runID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			StatusExecuting,

		run.Status)

	got, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusFailed,

		got.Status,
	)

}

// TestIntegration_HandleTaskResult_Fallback_IdempotentOnRepeat asserts that
// repeating the fallback path (e.g. a duplicate late result) does not produce
// inconsistent state.
func TestIntegration_HandleTaskResult_Fallback_IdempotentOnRepeat(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	tr := assignedTaskResult(runID, taskID, "success")
	require.NoError(t,

		svc.handleTaskResult(ctx, workerID,
			projectID,
			tr))
	require.NoError(t,

		svc.handleTaskResult(ctx, workerID,
			projectID,
			tr))

	// Second time — must remain "completed", no panic, no error.

	got, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusCompleted,

		got.
			Status)

}
