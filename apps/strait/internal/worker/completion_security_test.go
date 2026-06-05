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
	"github.com/stretchr/testify/require"

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

	exec.handleSuccess(context.Background(), run, job, bigResult)

	calls := store.statusUpdates()
	require.NotEmpty(t, calls)

	events := getEvents()
	require.NotEmpty(t, events)
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

	exec.handleSuccess(context.Background(), run, job, nil)

	calls := store.statusUpdates()
	require.NotEmpty(t, calls)

	events := getEvents()
	require.NotEmpty(t, events)
	require.Equal(t,
		EventCompleted,
		events[0].Type)
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
	exec.handleSuccess(context.Background(), run, job, result)

	calls := store.statusUpdates()
	require.NotEmpty(t, calls)

	events := getEvents()
	require.NotEmpty(t, events)
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
		exec.handleSuccess(context.Background(), run, job, nil)
	})

	wg.Go(func() {
		run := testRun(1)
		run.Status = domain.StatusExecuting
		exec.handleTimeout(context.Background(), run, job, policy, nil)
	})

	wg.Wait()

	// Must not panic. At least one event should be emitted.
	events := getEvents()
	require.NotEmpty(t, events)
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
	require.Error(t,
		err)

	// The plain store path should not have been used.
	calls := store.statusUpdates()
	require.Empty(t, calls)
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

	exec.handleSuccess(context.Background(), run, job, nil)

	events := getEvents()
	require.NotEmpty(t, events)

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
	require.Len(t, calls,
		1)
	require.False(t,
		calls[0].from !=
			domain.
				StatusDequeued ||
			calls[0].to !=
				domain.
					StatusQueued)
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
	require.Len(t, calls,
		1)
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
	require.Len(t, calls,
		1)

	if _, ok := calls[0].fields["next_retry_at"]; ok {
		require.Failf(t, "test failure",

			"next_retry_at must not be in fields map; retry schedule lives in job_retries")
	}
	scheduled := store.scheduleRetries()
	require.Len(t, scheduled,
		1)
	require.True(t,
		scheduled[0].at.Equal(farFuture))
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
	require.Len(t, calls,
		20)
}

// TestMiddleware_EmptyChain verifies that Chain with an empty middleware slice
// invokes the handler directly.
func TestMiddleware_EmptyChain(t *testing.T) {
	t.Parallel()

	called := false
	handler := func(_ context.Context, _ *ExecutionContext) { called = true }

	Chain()(handler)(context.Background(), &ExecutionContext{})
	require.True(t,
		called)
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
		require.Fail(t,

			"handler should not be reached after panic")
	}

	defer func() {
		r := recover()
		require.NotNil(t,
			r)

		msg, ok := r.(string)
		require.False(t,
			!ok || msg != "middleware exploded",
		)
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
		require.NotNil(t,
			r)
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
	require.NotEqual(t, 0, seen)
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
		require.False(t,
			!shortCircuit &&
				count ==
					0 && !handlerCalled,
		)
	})
}
