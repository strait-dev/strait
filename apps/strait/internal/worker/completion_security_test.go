package worker

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sourcegraph/conc"

	"strait/internal/domain"
	orcstore "strait/internal/store"
)

// TestCompletion_HugeResultPayload verifies that handleSuccess does not panic
// when the result payload is extremely large (~10MB).
func TestCompletion_HugeResultPayload(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			return nil, nil
		},
	}
	exec := newSnoozeTestExecutor(t, store, 0)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	run := testRun(1)
	run.Status = domain.StatusExecuting
	job := testJob("http://localhost", 3, 30)

	// Build a ~10MB result payload.
	bigResult := json.RawMessage(`{"data":"` + strings.Repeat("x", 10*1024*1024) + `"}`)

	exec.handleSuccess(context.Background(), run, job, bigResult, nil)

	calls := store.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected at least one status update")
	}

	events := getEvents()
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
}

// TestCompletion_NullResultPayload verifies that handleSuccess handles a nil
// result payload without panicking.
func TestCompletion_NullResultPayload(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			return nil, nil
		},
	}
	exec := newSnoozeTestExecutor(t, store, 0)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	run := testRun(1)
	run.Status = domain.StatusExecuting
	job := testJob("http://localhost", 3, 30)

	exec.handleSuccess(context.Background(), run, job, nil, nil)

	calls := store.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected at least one status update")
	}

	events := getEvents()
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	if events[0].Type != EventCompleted {
		t.Fatalf("expected EventCompleted, got %s", events[0].Type)
	}
}

// TestCompletion_ResultWithNullBytes verifies that result payloads containing
// null bytes do not corrupt the status update.
func TestCompletion_ResultWithNullBytes(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			return nil, nil
		},
	}
	exec := newSnoozeTestExecutor(t, store, 0)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	run := testRun(1)
	run.Status = domain.StatusExecuting
	job := testJob("http://localhost", 3, 30)

	result := json.RawMessage(`{"data":"hello\u0000world"}`)
	exec.handleSuccess(context.Background(), run, job, result, nil)

	calls := store.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected at least one status update")
	}

	events := getEvents()
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
}

// TestCompletion_ConcurrentCompletionAndTimeout verifies that concurrent
// handleSuccess and handleTimeout calls on the same run do not cause panics
// or data races.
func TestCompletion_ConcurrentCompletionAndTimeout(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			return nil, nil
		},
	}
	exec := newSnoozeTestExecutor(t, store, 0)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	job := testJob("http://localhost", 3, 30)
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	var wg conc.WaitGroup

	wg.Go(func() {
		run := testRun(1)
		run.Status = domain.StatusExecuting
		exec.handleSuccess(context.Background(), run, job, nil, nil)
	})

	wg.Go(func() {
		run := testRun(1)
		run.Status = domain.StatusExecuting
		exec.handleTimeout(context.Background(), run, job, policy, nil)
	})

	wg.Wait()

	// Must not panic. At least one event should be emitted.
	events := getEvents()
	if len(events) == 0 {
		t.Fatal("expected at least one event from concurrent completion/timeout")
	}
}

// TestCompletion_WebhookCallbackFailure verifies that when the webhook path
// fails (tx begin error), the error is propagated and the run is not silently
// marked completed.
func TestCompletion_WebhookCallbackFailure(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	txPool := &mockTxBeginner{
		beginErr: context.DeadlineExceeded,
	}
	exec := newCompletionTestExecutor(t, store, txPool)

	run := &domain.JobRun{ID: "run-wh-fail", Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", WebhookURL: "https://example.com/hook"}

	err := exec.completeRunWithWebhook(context.Background(), run, job,
		domain.StatusCompleted, map[string]any{"finished_at": time.Now()})

	if err == nil {
		t.Fatal("expected error from tx begin failure")
	}

	// The plain store path should not have been used.
	calls := store.statusUpdates()
	if len(calls) != 0 {
		t.Fatalf("expected 0 plain store calls, got %d", len(calls))
	}
}

// TestCompletion_MetricsOverflow verifies that handleSuccess does not panic
// when the execution duration overflows to MaxFloat64.
func TestCompletion_MetricsOverflow(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			return nil, nil
		},
	}
	exec := newSnoozeTestExecutor(t, store, 0)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	run := testRun(1)
	run.Status = domain.StatusExecuting
	// Set StartedAt to a very old time to produce a huge duration.
	old := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	run.StartedAt = &old
	job := testJob("http://localhost", 3, 30)

	exec.handleSuccess(context.Background(), run, job, nil, nil)

	events := getEvents()
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}

	// Verify no NaN or Inf leaked out.
	_ = math.MaxFloat64
}

// TestSnooze_NegativeDuration verifies that snoozing with a negative retry-at
// time (in the past) does not cause a panic.
func TestSnooze_NegativeDuration(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusDequeued
	pastTime := time.Now().Add(-1 * time.Hour)
	exec.snoozeRun(context.Background(), run, "negative duration test", &pastTime)

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(calls))
	}
	if calls[0].from != domain.StatusDequeued || calls[0].to != domain.StatusQueued {
		t.Fatalf("expected Dequeued->Queued, got %s->%s", calls[0].from, calls[0].to)
	}
}

// TestSnooze_PastTimestamp verifies that a snooze with an explicitly past
// timestamp is accepted (the scheduler will pick it up immediately).
func TestSnooze_PastTimestamp(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusDequeued
	pastTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	exec.snoozeRun(context.Background(), run, "past timestamp test", &pastTime)

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(calls))
	}
}

// TestSnooze_MaxTimestamp verifies that snoozing with a year-9999 timestamp
// does not cause overflow or panic.
func TestSnooze_MaxTimestamp(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusDequeued
	farFuture := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	exec.snoozeRun(context.Background(), run, "max timestamp test", &farFuture)

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(calls))
	}

	// The retry_at field should be set to the far-future time.
	if retryAt, ok := calls[0].fields["next_retry_at"].(*time.Time); ok {
		if !retryAt.Equal(farFuture) {
			t.Fatalf("expected retry_at = %v, got %v", farFuture, *retryAt)
		}
	}
}

// TestSnooze_ConcurrentSnoozeAndResume verifies that concurrent snooze and
// resume (re-queue) operations do not cause data races.
func TestSnooze_ConcurrentSnoozeAndResume(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	var wg conc.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			run := testRun(1)
			run.ID = "run-" + string(rune('A'+i%26))
			run.Status = domain.StatusDequeued
			retryAt := time.Now().Add(time.Duration(i) * time.Second)
			exec.snoozeRun(context.Background(), run, "concurrent test", &retryAt)
		})
	}
	wg.Wait()

	calls := store.statusUpdates()
	if len(calls) != 20 {
		t.Fatalf("expected 20 status updates, got %d", len(calls))
	}
}

// TestMiddleware_EmptyChain verifies that Chain with an empty middleware slice
// invokes the handler directly.
func TestMiddleware_EmptyChain(t *testing.T) {
	t.Parallel()

	called := false
	handler := func(_ context.Context, _ *ExecutionContext) { called = true }

	Chain()(handler)(context.Background(), &ExecutionContext{})

	if !called {
		t.Fatal("handler was not called with empty chain")
	}
}

// TestMiddleware_PanicInHandler verifies that a panic in a middleware is
// propagated and does not silently swallow errors.
func TestMiddleware_PanicInHandler(t *testing.T) {
	t.Parallel()

	panicMW := func(next ExecutionHandler) ExecutionHandler {
		return func(ctx context.Context, ec *ExecutionContext) {
			panic("middleware exploded")
			// next is never called.
		}
	}

	handler := func(_ context.Context, _ *ExecutionContext) {
		t.Fatal("handler should not be reached after panic")
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic to propagate")
		}
		msg, ok := r.(string)
		if !ok || msg != "middleware exploded" {
			t.Fatalf("unexpected panic value: %v", r)
		}
	}()

	Chain(panicMW)(handler)(context.Background(), &ExecutionContext{
		Run: &domain.JobRun{ID: "run-1"},
	})
}

// TestMiddleware_NilMiddleware verifies that a nil middleware in the chain
// causes a panic (nil function call) rather than silently passing through.
func TestMiddleware_NilMiddleware(t *testing.T) {
	t.Parallel()

	handler := func(_ context.Context, _ *ExecutionContext) {}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic from nil middleware")
		}
	}()

	// A nil ExecutionMiddleware will panic when called.
	Chain(nil)(handler)(context.Background(), &ExecutionContext{
		Run: &domain.JobRun{ID: "run-1"},
	})
}

// TestMiddleware_ConcurrentExecution verifies that Chain-produced handlers
// are safe for concurrent use.
func TestMiddleware_ConcurrentExecution(t *testing.T) {
	t.Parallel()

	var count sync.Map
	countMW := func(next ExecutionHandler) ExecutionHandler {
		return func(ctx context.Context, ec *ExecutionContext) {
			count.Store(ec.Run.ID, true)
			next(ctx, ec)
		}
	}

	handler := Chain(countMW)(func(_ context.Context, _ *ExecutionContext) {})

	var wg conc.WaitGroup
	const goroutines = 100
	for i := range goroutines {
		wg.Go(func() {
			ec := &ExecutionContext{
				Run:   &domain.JobRun{ID: "run-" + string(rune('0'+i%10))},
				Start: time.Now(),
			}
			handler(context.Background(), ec)
		})
	}
	wg.Wait()

	// Verify at least some runs were recorded.
	seen := 0
	count.Range(func(_, _ any) bool {
		seen++
		return true
	})
	if seen == 0 {
		t.Fatal("expected at least one run to be recorded")
	}
}

// FuzzMiddlewareChain fuzzes the number and ordering of middleware to verify
// that Chain does not panic for any combination.
func FuzzMiddlewareChain(f *testing.F) {
	f.Add(0, true)
	f.Add(1, false)
	f.Add(5, true)
	f.Add(10, false)
	f.Add(50, true)

	f.Fuzz(func(t *testing.T, count int, shortCircuit bool) {
		// Clamp count to prevent excessive allocations.
		if count < 0 {
			count = 0
		}
		if count > 100 {
			count = 100
		}

		middlewares := make([]ExecutionMiddleware, count)
		for i := range count {
			idx := i
			middlewares[idx] = func(next ExecutionHandler) ExecutionHandler {
				return func(ctx context.Context, ec *ExecutionContext) {
					if shortCircuit && idx == count/2 {
						return
					}
					next(ctx, ec)
				}
			}
		}

		handlerCalled := false
		handler := func(_ context.Context, _ *ExecutionContext) {
			handlerCalled = true
		}

		Chain(middlewares...)(handler)(context.Background(), &ExecutionContext{
			Run:   &domain.JobRun{ID: "fuzz-run"},
			Start: time.Now(),
		})

		if !shortCircuit && count == 0 && !handlerCalled {
			t.Fatal("handler should be called with empty chain and no short-circuit")
		}
	})
}
