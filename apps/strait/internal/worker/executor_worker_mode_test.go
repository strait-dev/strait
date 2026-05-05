package worker

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	workergrpc "strait/internal/api/grpc"
	"strait/internal/domain"
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

	updates := ms.statusUpdates()
	gotCompleted := false
	for _, u := range updates {
		if u.to == domain.StatusCompleted {
			gotCompleted = true
		}
	}
	if !gotCompleted {
		t.Fatalf("expected transition to completed, got updates: %+v", updates)
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
	gotCompleted := false
	gotDeadLetter := false
	var failureFields map[string]any
	for _, u := range updates {
		if u.to == domain.StatusCompleted {
			gotCompleted = true
		}
		if u.to == domain.StatusDeadLetter {
			gotDeadLetter = true
			failureFields = u.fields
		}
	}
	if gotCompleted {
		t.Fatal("worker reported failed but run was marked completed — regression")
	}
	if !gotDeadLetter {
		t.Fatalf("expected dead_letter transition, got updates: %+v", updates)
	}
	if msg, ok := failureFields["error"].(string); !ok || msg == "" {
		t.Fatalf("expected error message in failure fields, got: %+v", failureFields)
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

	waitForCondition(t, 2*time.Second, func() bool {
		for _, u := range ms.statusUpdates() {
			if u.to == domain.StatusDeadLetter {
				return true
			}
		}
		return false
	}, "dead_letter transition")

	for _, u := range ms.statusUpdates() {
		if u.to == domain.StatusDeadLetter {
			msg, _ := u.fields["error"].(string)
			if msg == "" {
				t.Fatalf("expected default error message, got empty")
			}
			return
		}
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

	waitForCondition(t, 2*time.Second, func() bool {
		for _, u := range ms.statusUpdates() {
			if u.to == domain.StatusDeadLetter {
				return true
			}
		}
		return false
	}, "dead_letter transition")

	for _, u := range ms.statusUpdates() {
		if u.to == domain.StatusCompleted {
			t.Fatal("nil result silently routed to completed — regression")
		}
	}
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

	waitForCondition(t, 2*time.Second, func() bool {
		for _, u := range ms.statusUpdates() {
			if u.to == domain.StatusDeadLetter {
				return true
			}
		}
		return false
	}, "dead_letter transition")

	for _, u := range ms.statusUpdates() {
		if u.to == domain.StatusCompleted {
			t.Fatal("unknown status silently routed to completed — regression")
		}
	}
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

	gotRequeued := false
	for _, u := range ms.statusUpdates() {
		if u.from == domain.StatusExecuting && u.to == domain.StatusQueued {
			gotRequeued = true
		}
	}
	if !gotRequeued {
		t.Fatalf("expected requeue from executing to queued, got: %+v", ms.statusUpdates())
	}
}

// TestExecuteWorkerMode_RegistersHeartbeatWhileDispatchInFlight verifies that
// worker-mode runs participate in the executor heartbeat loop for the full
// duration of the remote task. Without this, long-running worker-mode runs
// appear stale to the reaper and get crashed mid-execution.
func TestExecuteWorkerMode_RegistersHeartbeatWhileDispatchInFlight(t *testing.T) {
	t.Parallel()

	dispatcher := &blockingWorkerDispatcher{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	ms := &mockExecutorStore{}
	exec, _, _ := newWorkerModeExecutor(t, ms, dispatcher)
	run := testRun(1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		exec.executeWorkerMode(context.Background(), run, workerModeJob(3))
	}()

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

	waitForCondition(t, 2*time.Second, func() bool {
		for _, u := range ms.statusUpdates() {
			if u.to == domain.StatusCompleted {
				return true
			}
		}
		return false
	}, "completed transition")
}
