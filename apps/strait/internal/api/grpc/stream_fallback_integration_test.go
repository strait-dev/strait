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

	if err := q.CreateProject(ctx, &domain.Project{
		ID:    projectID,
		OrgID: "org-1",
		Name:  "fallback-test",
	}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

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
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	// CreateRun may set its own initial status — force executing for the test.
	if err := q.UpdateRunStatus(ctx, runID, run.Status, domain.StatusExecuting, map[string]any{
		"started_at": time.Now(),
	}); err != nil {
		// Already-executing or matching state is fine.
		t.Logf("UpdateRunStatus to executing returned: %v (continuing)", err)
	}

	// Insert worker row so worker_tasks FK (if any) is satisfied.
	if err := q.RegisterWorker(ctx, &domain.Worker{
		ID:        workerID,
		ProjectID: projectID,
		QueueName: "default",
		Hostname:  "h",
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
		Status:    domain.WorkerTaskStatusAssigned,
	}); err != nil {
		t.Fatalf("CreateWorkerTask: %v", err)
	}

	return projectID, workerID, runID, taskID
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

	svc := fallbackService(q)

	tr := &workerv1.TaskResult{RunId: runID, Status: "success", OutputJson: []byte(`{"worker":"result"}`)}
	if err := svc.handleTaskResult(ctx, workerID, projectID, tr); err != nil {
		t.Fatalf("handleTaskResult: %v", err)
	}

	// Worker task must now be completed.
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
	var result map[string]string
	if err := json.Unmarshal(run.Result, &result); err != nil {
		t.Fatalf("unmarshal run result: %v", err)
	}
	if result["worker"] != "result" {
		t.Fatalf("run result = %s, want worker output_json", string(run.Result))
	}
}

func TestIntegration_HandleTaskResult_Fallback_DoesNotCompleteTaskWhenRunUpdateFails(t *testing.T) {
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
	if _, err := env.DB.Pool.Exec(ctx, `
		UPDATE job_runs
		SET status = $1, finished_at = NOW(), result = $2::jsonb
		WHERE id = $3
	`, domain.StatusCanceled, json.RawMessage(`{"already":true}`), runID); err != nil {
		t.Fatalf("force run terminal: %v", err)
	}

	svc := fallbackService(q)
	tr := &workerv1.TaskResult{RunId: runID, Status: "success", OutputJson: []byte(`{"worker":"late"}`)}
	if err := svc.handleTaskResult(ctx, workerID, projectID, tr); err != nil {
		t.Fatalf("handleTaskResult: %v", err)
	}

	got, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if got.Status != domain.WorkerTaskStatusAssigned {
		t.Fatalf("worker task status = %q, want assigned when run update fails", got.Status)
	}
}

func TestIntegration_HandleTaskResult_Fallback_UsesRunFinalizerForSuccess(t *testing.T) {
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
	finalizer := &recordingRunFinalizer{}
	svc := fallbackServiceWithFinalizer(q, finalizer)

	tr := &workerv1.TaskResult{RunId: runID, Status: "success", OutputJson: []byte(`{"worker":"result"}`)}
	if err := svc.handleTaskResult(ctx, workerID, projectID, tr); err != nil {
		t.Fatalf("handleTaskResult: %v", err)
	}

	if len(finalizer.calls) != 1 {
		t.Fatalf("finalizer calls = %d, want 1", len(finalizer.calls))
	}
	call := finalizer.calls[0]
	if call.runID != runID || call.status != "success" || string(call.output) != `{"worker":"result"}` {
		t.Fatalf("unexpected finalizer call: %+v", call)
	}
	task, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if task.Status != domain.WorkerTaskStatusCompleted {
		t.Fatalf("worker task status = %q, want completed", task.Status)
	}
	run, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != domain.StatusExecuting {
		t.Fatalf("fallback should let finalizer own run transition, got run status %q", run.Status)
	}
}

func TestIntegration_HandleTaskResult_Fallback_FinalizerErrorLeavesTaskOpen(t *testing.T) {
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
	svc := fallbackServiceWithFinalizer(q, &recordingRunFinalizer{err: errors.New("finalizer failed")})

	tr := &workerv1.TaskResult{RunId: runID, Status: "success"}
	if err := svc.handleTaskResult(ctx, workerID, projectID, tr); err != nil {
		t.Fatalf("handleTaskResult: %v", err)
	}

	task, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if task.Status != domain.WorkerTaskStatusAssigned {
		t.Fatalf("worker task status = %q, want assigned when finalizer fails", task.Status)
	}
}

func TestIntegration_HandleAck_MarksOpenWorkerTaskAccepted(t *testing.T) {
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
	svc := fallbackService(q)

	msg := &workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_Ack{
			Ack: &workerv1.Acknowledged{Id: runID},
		},
	}
	if err := svc.handleWorkerMessage(ctx, workerID, projectID, msg); err != nil {
		t.Fatalf("handleWorkerMessage ack: %v", err)
	}

	task, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if task.Status != domain.WorkerTaskStatusAccepted {
		t.Fatalf("task status = %q, want accepted", task.Status)
	}
	if task.AcceptedAt == nil {
		t.Fatal("accepted_at is nil, want timestamp")
	}
}

// TestIntegration_HandleTaskResult_Fallback_FailedUpdatesWorkerTask asserts the
// failure variant of the fallback path transitions the worker_tasks row to
// "failed".
func TestIntegration_HandleTaskResult_Fallback_FailedUpdatesWorkerTask(t *testing.T) {
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
	svc := fallbackService(q)

	tr := &workerv1.TaskResult{RunId: runID, Status: "failed", ErrorMessage: "boom"}
	if err := svc.handleTaskResult(ctx, workerID, projectID, tr); err != nil {
		t.Fatalf("handleTaskResult: %v", err)
	}

	got, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if got.Status != domain.WorkerTaskStatusFailed {
		t.Fatalf("worker_tasks not transitioned: got %q, want failed", got.Status)
	}
}

// TestIntegration_HandleTaskResult_Fallback_ProjectMismatchRejects guards the
// adversarial path: a worker authenticated to a different project must NOT
// touch the run or worker_tasks row.
func TestIntegration_HandleTaskResult_Fallback_ProjectMismatchRejects(t *testing.T) {
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

	// Use the WRONG project ID — simulates a stream authenticated to project B
	// trying to mark a run that belongs to project A.
	tr := &workerv1.TaskResult{RunId: runID, Status: "success"}
	if err := svc.handleTaskResult(ctx, workerID, "proj-impostor", tr); err != nil {
		t.Fatalf("handleTaskResult unexpectedly errored: %v", err)
	}

	got, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if got.Status != domain.WorkerTaskStatusAssigned {
		t.Fatalf("project mismatch should not transition worker_task: got %q", got.Status)
	}
}

// TestIntegration_HandleTaskResult_Fallback_OwnershipMismatchRejects guards
// against a worker reporting a result for a run it was never assigned. With
// no worker_tasks row matching (workerID, runID), the fallback must not
// touch the run row.
func TestIntegration_HandleTaskResult_Fallback_OwnershipMismatchRejects(t *testing.T) {
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
	projectID, _, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	// A different worker, in the same project, reports a result it doesn't own.
	otherWorker := "other-worker-" + uuid.Must(uuid.NewV7()).String()
	if err := q.RegisterWorker(ctx, &domain.Worker{
		ID:        otherWorker,
		ProjectID: projectID,
		QueueName: "default",
		Hostname:  "h",
		Version:   "1.0",
		Status:    domain.WorkerStatusActive,
	}); err != nil {
		t.Fatalf("RegisterWorker other: %v", err)
	}

	tr := &workerv1.TaskResult{RunId: runID, Status: "success"}
	if err := svc.handleTaskResult(ctx, otherWorker, projectID, tr); err != nil {
		t.Fatalf("handleTaskResult: %v", err)
	}

	// Original worker_task must remain assigned (not touched by the impostor).
	got, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if got.Status != domain.WorkerTaskStatusAssigned {
		t.Fatalf("ownership mismatch should not touch original task: got %q", got.Status)
	}
}

// TestIntegration_HandleTaskResult_Fallback_ClosedAssignmentRejects verifies
// that historical worker_tasks rows do not grant ownership after a disconnect
// requeue. A stale worker result must not complete a run now owned by another
// live assignment.
func TestIntegration_HandleTaskResult_Fallback_ClosedAssignmentRejects(t *testing.T) {
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
	svc := fallbackService(q)

	if err := q.UpdateWorkerTaskStatus(ctx, taskID, domain.WorkerTaskStatusFailed); err != nil {
		t.Fatalf("UpdateWorkerTaskStatus: %v", err)
	}

	tr := &workerv1.TaskResult{RunId: runID, Status: "success"}
	if err := svc.handleTaskResult(ctx, workerID, projectID, tr); err != nil {
		t.Fatalf("handleTaskResult: %v", err)
	}

	run, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != domain.StatusExecuting {
		t.Fatalf("closed assignment result changed run status to %q, want executing", run.Status)
	}

	got, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if got.Status != domain.WorkerTaskStatusFailed {
		t.Fatalf("closed assignment result changed task status to %q, want failed", got.Status)
	}
}

// TestIntegration_HandleTaskResult_Fallback_IdempotentOnRepeat asserts that
// repeating the fallback path (e.g. a duplicate late result) does not produce
// inconsistent state.
func TestIntegration_HandleTaskResult_Fallback_IdempotentOnRepeat(t *testing.T) {
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
	svc := fallbackService(q)

	tr := &workerv1.TaskResult{RunId: runID, Status: "success"}
	if err := svc.handleTaskResult(ctx, workerID, projectID, tr); err != nil {
		t.Fatalf("handleTaskResult #1: %v", err)
	}
	// Second time — must remain "completed", no panic, no error.
	if err := svc.handleTaskResult(ctx, workerID, projectID, tr); err != nil {
		t.Fatalf("handleTaskResult #2: %v", err)
	}

	got, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if got.Status != domain.WorkerTaskStatusCompleted {
		t.Fatalf("expected completed after idempotent retry, got %q", got.Status)
	}
}
