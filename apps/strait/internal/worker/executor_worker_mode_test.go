package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	workergrpc "strait/internal/api/grpc"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
)

// fakeWorkerDispatcher is a test double for WorkerRunDispatcher. It returns
// a pre-configured opaque value plus an optional error from WorkerDispatch,
// and looks up status / error message by direct map lookup on the opaque so
// tests can model arbitrary TaskResult-shaped values without importing the
// grpc proto types.
type fakeWorkerDispatcher struct {
	opaque any
	err    error
	calls  atomic.Int32

	statusOf map[any]string
	errorOf  map[any]string
	outputOf map[any]json.RawMessage

	completeCalls  atomic.Int32
	completeStatus domain.WorkerTaskStatus
}

func (f *fakeWorkerDispatcher) WorkerDispatch(_ context.Context, _ *domain.JobRun, _ *domain.Job) (any, error) {
	f.calls.Add(1)
	return f.opaque, f.err
}

func (f *fakeWorkerDispatcher) ResultStatus(opaque any) string {
	if f.statusOf == nil {
		return ""
	}
	return f.statusOf[opaque]
}

func (f *fakeWorkerDispatcher) ResultError(opaque any) string {
	if f.errorOf == nil {
		return ""
	}
	return f.errorOf[opaque]
}

func (f *fakeWorkerDispatcher) ResultOutput(opaque any) json.RawMessage {
	if f.outputOf == nil {
		return nil
	}
	return f.outputOf[opaque]
}

func (f *fakeWorkerDispatcher) CompleteWorkerTask(_ context.Context, _ any, status domain.WorkerTaskStatus) error {
	f.completeStatus = status
	f.completeCalls.Add(1)
	return nil
}

type blockingWorkerDispatcher struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingWorkerDispatcher) WorkerDispatch(_ context.Context, _ *domain.JobRun, _ *domain.Job) (any, error) {
	close(b.started)
	<-b.release
	return struct{}{}, nil
}

func (b *blockingWorkerDispatcher) ResultStatus(any) string { return "success" }

func (b *blockingWorkerDispatcher) ResultError(any) string { return "" }

func (b *blockingWorkerDispatcher) ResultOutput(any) json.RawMessage { return nil }

func (b *blockingWorkerDispatcher) CompleteWorkerTask(context.Context, any, domain.WorkerTaskStatus) error {
	return nil
}

type contextDeadlineWorkerDispatcher struct {
	started     chan struct{}
	hasDeadline atomic.Bool
	calls       atomic.Int32
}

func (d *contextDeadlineWorkerDispatcher) WorkerDispatch(ctx context.Context, _ *domain.JobRun, _ *domain.Job) (any, error) {
	d.calls.Add(1)
	if _, ok := ctx.Deadline(); ok {
		d.hasDeadline.Store(true)
	}
	close(d.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (d *contextDeadlineWorkerDispatcher) ResultStatus(any) string { return "" }

func (d *contextDeadlineWorkerDispatcher) ResultError(any) string { return "" }

func (d *contextDeadlineWorkerDispatcher) ResultOutput(any) json.RawMessage { return nil }

func (d *contextDeadlineWorkerDispatcher) CompleteWorkerTask(context.Context, any, domain.WorkerTaskStatus) error {
	return nil
}

func newWorkerModeExecutor(t *testing.T, store *mockExecutorStore, dispatcher WorkerRunDispatcher) (*Executor, *mockWorkerPublisher, *mockWorkflowCallback) {
	t.Helper()
	pub := &mockWorkerPublisher{}
	cb := &mockWorkflowCallback{}

	pool := NewPool(2)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             &mockExecQueue{},
		Store:             store,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
		HTTPClient:        http.DefaultClient,
		Publisher:         pub,
		WorkflowCallback:  cb,
		WorkerDispatcher:  dispatcher,
	})
	return exec, pub, cb
}

func workerModeJob(maxAttempts int) *domain.Job {
	return &domain.Job{
		ID:            "job-1",
		ProjectID:     "proj-1",
		ExecutionMode: domain.ExecutionModeWorker,
		Queue:         "default",
		MaxAttempts:   maxAttempts,
		TimeoutSecs:   30,
	}
}

func TestWorkerRunResultFromDispatch(t *testing.T) {
	t.Parallel()

	opaque := struct{ id string }{id: "result-1"}
	wantOutput := json.RawMessage(`{"ok":true}`)
	dispatcher := &fakeWorkerDispatcher{
		statusOf: map[any]string{opaque: "success"},
		errorOf:  map[any]string{opaque: "ignored warning"},
		outputOf: map[any]json.RawMessage{opaque: wantOutput},
	}
	exec, _, _ := newWorkerModeExecutor(t, &mockExecutorStore{}, dispatcher)

	result := exec.workerRunResultFromDispatch(opaque)
	if result.status != "success" {
		t.Fatalf("status = %q, want success", result.status)
	}
	if result.errorMessage != "ignored warning" {
		t.Fatalf("errorMessage = %q, want ignored warning", result.errorMessage)
	}
	if string(result.output) != string(wantOutput) {
		t.Fatalf("output = %s, want %s", result.output, wantOutput)
	}
	if !result.succeeded() {
		t.Fatal("success status should be marked succeeded")
	}
}

func TestWorkerRunResultFailureMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   workerRunResult
		want string
	}{
		{
			name: "explicit error wins",
			in: workerRunResult{
				status:       "failed",
				errorMessage: "boom",
			},
			want: "boom",
		},
		{
			name: "empty status means malformed result",
			in:   workerRunResult{},
			want: "worker returned malformed or empty result",
		},
		{
			name: "terminal status without error is named",
			in: workerRunResult{
				status: "cancelled",
			},
			want: `worker reported terminal status "cancelled" without error message`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.in.failureMessage(); got != tt.want {
				t.Fatalf("failureMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFinalizeWorkerRunResult_SuccessUsesExecutorCompletionPath(t *testing.T) {
	t.Parallel()
	startedAt := time.Now().Add(-time.Second)
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
		Attempt:   1,
		StartedAt: &startedAt,
	}
	job := workerModeJob(3)
	ms := &mockExecutorStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			if id != run.ID {
				t.Fatalf("GetRun id = %q, want %q", id, run.ID)
			}
			return run, nil
		},
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			if id != job.ID {
				t.Fatalf("GetJob id = %q, want %q", id, job.ID)
			}
			return job, nil
		},
	}
	exec, _, _ := newWorkerModeExecutor(t, ms, &fakeWorkerDispatcher{})

	taskStatus, err := exec.FinalizeWorkerRunResult(context.Background(), run.ID, "success", "", json.RawMessage(`{"ok":true}`))
	if err != nil {
		t.Fatalf("FinalizeWorkerRunResult: %v", err)
	}
	if taskStatus != domain.WorkerTaskStatusCompleted {
		t.Fatalf("task status = %q, want completed", taskStatus)
	}
	call := requireOnlyStatusTransition(t, ms.statusUpdates(), domain.StatusExecuting, domain.StatusCompleted)
	if string(call.fields["result"].(json.RawMessage)) != `{"ok":true}` {
		t.Fatalf("result field = %v, want worker output", call.fields["result"])
	}
}

func TestFinalizeWorkerRunResult_FailureUsesExecutorRetryPath(t *testing.T) {
	t.Parallel()
	startedAt := time.Now().Add(-time.Second)
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
		Attempt:   1,
		StartedAt: &startedAt,
	}
	job := workerModeJob(3)
	ms := &mockExecutorStore{
		getRunFn: func(context.Context, string) (*domain.JobRun, error) {
			return run, nil
		},
		getJobFn: func(context.Context, string) (*domain.Job, error) {
			return job, nil
		},
	}
	exec, _, _ := newWorkerModeExecutor(t, ms, &fakeWorkerDispatcher{})

	taskStatus, err := exec.FinalizeWorkerRunResult(context.Background(), run.ID, "failed", "boom", nil)
	if err != nil {
		t.Fatalf("FinalizeWorkerRunResult: %v", err)
	}
	if taskStatus != domain.WorkerTaskStatusFailed {
		t.Fatalf("task status = %q, want failed", taskStatus)
	}
	call := requireOnlyStatusTransition(t, ms.statusUpdates(), domain.StatusExecuting, domain.StatusQueued)
	if call.fields["attempt"] != 2 {
		t.Fatalf("attempt field = %v, want 2", call.fields["attempt"])
	}
	if call.fields["error"] != "boom" {
		t.Fatalf("error field = %v, want boom", call.fields["error"])
	}
}

// TestExecuteWorkerMode_SuccessRoutesToHandleSuccess verifies that a worker
// returning Status="success" transitions the run to completed.
func TestExecuteWorkerMode_SuccessRoutesToHandleSuccess(t *testing.T) {
	t.Parallel()
	successOpaque := struct{ tag string }{tag: "ok"}
	dispatcher := &fakeWorkerDispatcher{
		opaque:   successOpaque,
		statusOf: map[any]string{successOpaque: "success"},
	}

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return workerModeJob(3), nil
		},
	}
	exec, _, cb := newWorkerModeExecutor(t, ms, dispatcher)

	run := testRun(1)
	exec.executeWorkerMode(context.Background(), run, workerModeJob(3))

	waitForCondition(t, 2*time.Second, func() bool { return cb.calls.Load() >= 1 }, "callback")

	requireStatusUpdateTo(t, ms.statusUpdates(), domain.StatusCompleted)
}

func TestExecuteWorkerMode_TimesOutWorkerDispatchUsingExecutionPolicy(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	dispatcher := &contextDeadlineWorkerDispatcher{started: make(chan struct{})}
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return workerModeJob(3), nil
		},
	}
	exec, _, _ := newWorkerModeExecutor(t, ms, dispatcher)

	run := testRun(1)
	job := workerModeJob(3)
	policy := defaultExecutionPolicy(job)
	policy.timeoutSecs = 1

	done := make(chan struct{})
	start := time.Now()
	concWG.Go(func() {
		exec.executeWorkerMode(context.Background(), run, job, policy)
		close(done)
	})

	select {
	case <-dispatcher.started:
	case <-time.After(time.Second):
		t.Fatal("worker dispatch did not start")
	}
	if !dispatcher.hasDeadline.Load() {
		t.Fatal("worker dispatch context did not include a deadline")
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker-mode dispatch did not return after policy timeout")
	}
	if elapsed := time.Since(start); elapsed < time.Second || elapsed > 2500*time.Millisecond {
		t.Fatalf("worker-mode timeout elapsed = %s, want about 1s", elapsed)
	}

	timeoutUpdate := requireRetryTransition(t, ms.statusUpdates())
	if timeoutUpdate.fields["attempt"] != 2 {
		t.Fatalf("attempt field = %v, want 2", timeoutUpdate.fields["attempt"])
	}
	if timeoutUpdate.fields["error"] != "execution timed out" {
		t.Fatalf("error field = %v, want execution timed out", timeoutUpdate.fields["error"])
	}
}

func TestExecuteWorkerMode_ParentCancellationRequeuesWithoutTimeout(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	dispatcher := &contextDeadlineWorkerDispatcher{started: make(chan struct{})}
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return workerModeJob(3), nil
		},
		updateRunStatusFn: func(ctx context.Context, _ string, _, to domain.RunStatus, _ map[string]any) error {
			if to == domain.StatusQueued && ctx.Err() != nil {
				t.Fatalf("requeue used cancelled context: %v", ctx.Err())
			}
			return nil
		},
	}
	exec, _, _ := newWorkerModeExecutor(t, ms, dispatcher)

	run := testRun(1)
	job := workerModeJob(3)
	policy := defaultExecutionPolicy(job)
	policy.timeoutSecs = 30
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	concWG.Go(func() {
		exec.executeWorkerMode(ctx, run, job, policy)
		close(done)
	})

	select {
	case <-dispatcher.started:
	case <-time.After(time.Second):
		t.Fatal("worker dispatch did not start")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker-mode dispatch did not return after parent cancellation")
	}

	requeueUpdate := requireRetryTransition(t, ms.statusUpdates())
	if _, ok := requeueUpdate.fields["attempt"]; ok {
		t.Fatalf("cancellation requeue should not increment attempt: %+v", requeueUpdate.fields)
	}
	if requeueUpdate.fields["error"] != nil {
		t.Fatalf("cancellation requeue should clear error, got: %+v", requeueUpdate.fields)
	}
}

func TestExecuteWorkerMode_SuccessPersistsWorkerOutput(t *testing.T) {
	t.Parallel()
	successOpaque := struct{ tag string }{tag: "output"}
	wantOutput := json.RawMessage(`{"answer":42}`)
	dispatcher := &fakeWorkerDispatcher{
		opaque:   successOpaque,
		statusOf: map[any]string{successOpaque: "success"},
		outputOf: map[any]json.RawMessage{successOpaque: wantOutput},
	}

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return workerModeJob(3), nil
		},
	}
	exec, _, _ := newWorkerModeExecutor(t, ms, dispatcher)

	run := testRun(1)
	exec.executeWorkerMode(context.Background(), run, workerModeJob(3))

	completed := waitForStatusUpdate(t, ms, domain.StatusCompleted)
	got, ok := completed.fields["result"].(json.RawMessage)
	if !ok {
		t.Fatalf("completed fields missing json result: %+v", completed.fields)
	}
	if string(got) != string(wantOutput) {
		t.Fatalf("result = %s, want %s", got, wantOutput)
	}
}

func TestExecuteWorkerMode_CompletesWorkerTaskAfterRunResultPersists(t *testing.T) {
	t.Parallel()
	successOpaque := struct{ tag string }{tag: "complete-after-persist"}
	dispatcher := &fakeWorkerDispatcher{
		opaque:   successOpaque,
		statusOf: map[any]string{successOpaque: "success"},
		outputOf: map[any]json.RawMessage{successOpaque: json.RawMessage(`{"ok":true}`)},
	}

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return workerModeJob(3), nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, to domain.RunStatus, fields map[string]any) error {
			if to == domain.StatusCompleted {
				if dispatcher.completeCalls.Load() != 0 {
					t.Fatal("worker task completed before run result persistence")
				}
				if got, ok := fields["result"].(json.RawMessage); !ok || string(got) != `{"ok":true}` {
					t.Fatalf("completed run fields result = %v, want worker output", fields["result"])
				}
			}
			return nil
		},
	}
	exec, _, _ := newWorkerModeExecutor(t, ms, dispatcher)

	run := testRun(1)
	exec.executeWorkerMode(context.Background(), run, workerModeJob(3))

	waitForCondition(t, 2*time.Second, func() bool {
		return dispatcher.completeCalls.Load() == 1
	}, "worker task completion")
	if dispatcher.completeStatus != domain.WorkerTaskStatusCompleted {
		t.Fatalf("worker task status = %q, want completed", dispatcher.completeStatus)
	}
}

func TestExecuteWorkerMode_DoesNotCompleteWorkerTaskWhenRunPersistenceFails(t *testing.T) {
	t.Parallel()
	successOpaque := struct{ tag string }{tag: "persist-fails"}
	dispatcher := &fakeWorkerDispatcher{
		opaque:   successOpaque,
		statusOf: map[any]string{successOpaque: "success"},
		outputOf: map[any]json.RawMessage{successOpaque: json.RawMessage(`{"ok":true}`)},
	}

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return workerModeJob(3), nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, to domain.RunStatus, _ map[string]any) error {
			if to == domain.StatusCompleted {
				return fmt.Errorf("persist failed")
			}
			return nil
		},
	}
	exec, _, _ := newWorkerModeExecutor(t, ms, dispatcher)

	run := testRun(1)
	exec.executeWorkerMode(context.Background(), run, workerModeJob(3))

	time.Sleep(50 * time.Millisecond)
	if dispatcher.completeCalls.Load() != 0 {
		t.Fatalf("worker task completed %d times despite failed run persistence", dispatcher.completeCalls.Load())
	}
}

// TestExecuteWorkerMode_FailedStatusRoutesToHandleFailure verifies that a
// worker reporting Status="failed" with maxAttempts=1 transitions the run to
// dead_letter — NOT completed. This is the regression for the bug at
// executor_dispatch.go where handleSuccess was called unconditionally.
func TestExecuteWorkerMode_FailedStatusRoutesToHandleFailure(t *testing.T) {
	t.Parallel()
	failOpaque := struct{ tag string }{tag: "fail"}
	dispatcher := &fakeWorkerDispatcher{
		opaque:   failOpaque,
		statusOf: map[any]string{failOpaque: "failed"},
		errorOf:  map[any]string{failOpaque: "boom: divide by zero"},
	}

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return workerModeJob(1), nil
		},
	}
	exec, _, cb := newWorkerModeExecutor(t, ms, dispatcher)

	run := testRun(1)
	exec.executeWorkerMode(context.Background(), run, workerModeJob(1))

	waitForCondition(t, 2*time.Second, func() bool { return cb.calls.Load() >= 1 }, "callback")

	updates := ms.statusUpdates()
	requireNoStatusUpdateTo(t, updates, domain.StatusCompleted)
	deadLetter := requireStatusUpdateTo(t, updates, domain.StatusDeadLetter)
	if msg, ok := deadLetter.fields["error"].(string); !ok || msg == "" {
		t.Fatalf("expected error message in failure fields, got: %+v", deadLetter.fields)
	}
}

// TestExecuteWorkerMode_FailedWithEmptyErrorUsesDefault asserts that a worker
// reporting "failed" without an error_message gets a synthesized default,
// so the run is still recorded with a non-empty error string.
func TestExecuteWorkerMode_FailedWithEmptyErrorUsesDefault(t *testing.T) {
	t.Parallel()
	op := struct{ tag string }{tag: "fail-empty"}
	dispatcher := &fakeWorkerDispatcher{
		opaque:   op,
		statusOf: map[any]string{op: "failed"},
		// errorOf intentionally empty
	}

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return workerModeJob(1), nil
		},
	}
	exec, _, _ := newWorkerModeExecutor(t, ms, dispatcher)

	run := testRun(1)
	exec.executeWorkerMode(context.Background(), run, workerModeJob(1))

	deadLetter := waitForStatusUpdate(t, ms, domain.StatusDeadLetter)
	msg, _ := deadLetter.fields["error"].(string)
	if msg == "" {
		t.Fatalf("expected default error message, got empty")
	}
}

// TestExecuteWorkerMode_NilResultTreatedAsFailure asserts the defensive path
// when WorkerDispatch returns (nil, nil): we don't silently mark the run as
// completed; we route to handleFailure with a malformed-result error.
func TestExecuteWorkerMode_NilResultTreatedAsFailure(t *testing.T) {
	t.Parallel()
	dispatcher := &fakeWorkerDispatcher{
		opaque: nil, // nil opaque + nil err — defensive case
	}

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return workerModeJob(1), nil
		},
	}
	exec, _, _ := newWorkerModeExecutor(t, ms, dispatcher)

	run := testRun(1)
	exec.executeWorkerMode(context.Background(), run, workerModeJob(1))

	waitForStatusUpdate(t, ms, domain.StatusDeadLetter)

	requireNoStatusUpdateTo(t, ms.statusUpdates(), domain.StatusCompleted)
}

// TestExecuteWorkerMode_UnknownStatusTreatedAsFailure asserts an unrecognized
// status string (e.g. "in_progress" leaked from a future protocol version) is
// treated as a failure rather than silently completed.
func TestExecuteWorkerMode_UnknownStatusTreatedAsFailure(t *testing.T) {
	t.Parallel()
	op := struct{ tag string }{tag: "unknown"}
	dispatcher := &fakeWorkerDispatcher{
		opaque:   op,
		statusOf: map[any]string{op: "weird-future-status"},
		errorOf:  map[any]string{op: ""},
	}

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return workerModeJob(1), nil
		},
	}
	exec, _, _ := newWorkerModeExecutor(t, ms, dispatcher)

	run := testRun(1)
	exec.executeWorkerMode(context.Background(), run, workerModeJob(1))

	waitForStatusUpdate(t, ms, domain.StatusDeadLetter)

	requireNoStatusUpdateTo(t, ms.statusUpdates(), domain.StatusCompleted)
}

// TestExecuteWorkerMode_DispatchErrorRequeuesOnNoWorker asserts that the
// existing "no worker available" path requeues the run unchanged.
func TestExecuteWorkerMode_DispatchErrorRequeuesOnNoWorker(t *testing.T) {
	t.Parallel()
	dispatcher := &fakeWorkerDispatcher{
		err: fmt.Errorf("dispatcher busy: %w", workergrpc.ErrNoWorkerAvailable),
	}

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return workerModeJob(3), nil
		},
	}
	exec, _, _ := newWorkerModeExecutor(t, ms, dispatcher)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	exec.executeWorkerMode(context.Background(), run, workerModeJob(3))

	requeue := requireRetryTransition(t, ms.statusUpdates())
	assertQueuedResetFields(t, requeue.fields)
}

// TestExecuteWorkerMode_RegistersHeartbeatWhileDispatchInFlight verifies that
// worker-mode runs participate in the executor heartbeat loop for the full
// duration of the remote task. Without this, long-running worker-mode runs
// appear stale to the reaper and get crashed mid-execution.
func TestExecuteWorkerMode_RegistersHeartbeatWhileDispatchInFlight(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	dispatcher := &blockingWorkerDispatcher{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	ms := &mockExecutorStore{}
	exec, _, _ := newWorkerModeExecutor(t, ms, dispatcher)
	run := testRun(1)

	done := make(chan struct{})
	concWG.Go(func() {
		defer close(done)
		exec.executeWorkerMode(context.Background(), run, workerModeJob(3))
	})

	select {
	case <-dispatcher.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for worker dispatch to start")
	}

	waitForCondition(t, 2*time.Second, func() bool {
		return exec.heartbeat.ActiveCount() == 1
	}, "worker heartbeat registration")

	close(dispatcher.release)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for worker dispatch to finish")
	}

	if got := exec.heartbeat.ActiveCount(); got != 0 {
		t.Fatalf("heartbeat active count = %d, want 0 after worker completion", got)
	}
}

func TestExecuteWorkerMode_NilDispatcherRequeuesWithCleanQueuedFields(t *testing.T) {
	t.Parallel()

	ms := &mockExecutorStore{}
	exec, _, _ := newWorkerModeExecutor(t, ms, nil)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	exec.executeWorkerMode(context.Background(), run, workerModeJob(3))

	requeue := requireRetryTransition(t, ms.statusUpdates())
	assertQueuedResetFields(t, requeue.fields)
}

func TestExecuteWorkerMode_NilDispatcherRequeuesDequeuedRun(t *testing.T) {
	t.Parallel()

	ms := &mockExecutorStore{}
	exec, _, _ := newWorkerModeExecutor(t, ms, nil)

	run := testRun(1)
	run.Status = domain.StatusDequeued
	exec.executeWorkerMode(context.Background(), run, workerModeJob(3))

	requeue := requireStatusTransition(t, ms.statusUpdates(), domain.StatusDequeued, domain.StatusQueued)
	assertQueuedResetFields(t, requeue.fields)
}

// TestExecuteWorkerMode_TrustsExplicitFailureOverErrorField is an adversarial
// guard: a worker reporting status="success" but with a non-empty error
// message should be trusted as success (the explicit status wins). This
// documents the precedence order — the alternative (treating a non-empty
// error as failure regardless of status) would let a misbehaving worker
// downgrade its own success outcomes by always sending an error string.
func TestExecuteWorkerMode_TrustsExplicitFailureOverErrorField(t *testing.T) {
	t.Parallel()
	op := struct{ tag string }{tag: "success+err"}
	dispatcher := &fakeWorkerDispatcher{
		opaque:   op,
		statusOf: map[any]string{op: "success"},
		errorOf:  map[any]string{op: "warning: ignored"},
	}

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return workerModeJob(3), nil
		},
	}
	exec, _, _ := newWorkerModeExecutor(t, ms, dispatcher)

	run := testRun(1)
	exec.executeWorkerMode(context.Background(), run, workerModeJob(3))

	waitForStatusUpdate(t, ms, domain.StatusCompleted)
}

func waitForStatusUpdate(t *testing.T, store *mockExecutorStore, status domain.RunStatus) statusUpdateCall {
	t.Helper()

	waitForCondition(t, 2*time.Second, func() bool {
		for _, update := range store.statusUpdates() {
			if update.to == status {
				return true
			}
		}
		return false
	}, string(status)+" transition")

	return requireStatusUpdateTo(t, store.statusUpdates(), status)
}

func assertQueuedResetFields(t *testing.T, fields map[string]any) {
	t.Helper()

	if fields == nil {
		t.Fatal("expected queued reset fields, got nil")
		return
	}

	wantNil := []string{
		"error",
		"error_class",
		"finished_at",
		"heartbeat_at",
		"next_retry_at",
		"started_at",
	}
	for _, key := range wantNil {
		value, ok := fields[key]
		if !ok {
			t.Fatalf("expected field %q in queued reset fields: %+v", key, fields)
		}
		if value != nil {
			t.Fatalf("expected field %q to be cleared, got %#v", key, value)
		}
	}
}
