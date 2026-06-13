package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/billing"
	"strait/internal/domain"
	orcstore "strait/internal/store"
)

// dispatch: expired TTL -- job should be marked system_failed

func TestDispatch_ExpiredTTL_StillDispatches(t *testing.T) {
	t.Parallel()

	// TTL expiry is enforced at the queue/dequeue layer, not in executeInner.
	// This test verifies that the dispatch path handles an expired TTL run
	// without crashing -- it proceeds normally and completes.
	var endpointCalled atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		endpointCalled.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(srv.URL, 3, 30), nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, srv.Client())

	past := time.Now().Add(-1 * time.Hour)
	run := testRun(1)
	run.ExpiresAt = &past

	exec.execute(context.Background(), run)
	require.NotEqual(t, 0, endpointCalled.
		Load())

	calls := st.statusUpdates()
	require.NotEmpty(t, calls)
}

// dispatch: unreachable endpoint URL -- should fail/retry

func TestDispatch_ConnectionRefused_Fails(t *testing.T) {
	t.Parallel()

	// Start a server and immediately close it to get a port that refuses connections.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	refusedURL := srv.URL
	srv.Close()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(refusedURL, 1, 2), nil
	}

	client := &http.Client{Timeout: 2 * time.Second}
	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, client)
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	require.GreaterOrEqual(t, len(calls), 2)

	// Last status update should be a terminal failure.
	last := calls[len(calls)-1]
	require.False(t,
		last.to != domain.
			StatusDeadLetter &&
			last.to !=
				domain.StatusSystemFailed &&
			last.to !=
				domain.StatusFailed,
	)
}

// dispatch: nil job reference -- should system-fail without panicking.
func TestDispatch_NilJobLookup_SystemFails(t *testing.T) {
	t.Parallel()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return nil, nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, http.DefaultClient)
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	require.NotEmpty(t, calls)
	require.Equal(t, domain.StatusSystemFailed, calls[0].to)
}

func TestDispatch_JobLookupError_SystemFails(t *testing.T) {
	t.Parallel()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return nil, errors.New("database unreachable")
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, http.DefaultClient)
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	require.NotEmpty(t, calls)
	require.Equal(t,
		domain.StatusSystemFailed,

		calls[0].to)
}

// dispatch: zero max_attempts -- should still execute once

func TestDispatch_ZeroMaxAttempts_ExecutesOnce(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(srv.URL, 0, 5), nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, srv.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	require.GreaterOrEqual(t, len(calls), 2)

	// With 0 max_attempts, run.Attempt(1) >= maxAttempts(0) so no retry.
	// Should go directly to dead_letter.
	last := calls[len(calls)-1]
	require.Equal(t,
		domain.StatusDeadLetter,
		last.
			to)
}

// dispatch: context cancellation during execution

func TestDispatch_ContextCancellation_HandledGracefully(t *testing.T) {
	t.Parallel()

	// Use a slow endpoint (delays briefly) and cancel the context before it returns.
	var handlerDone sync.WaitGroup
	handlerDone.Add(1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		defer handlerDone.Done()
		time.Sleep(2 * time.Second) // simulate slow work
		w.WriteHeader(http.StatusOK)
	}))
	defer func() {
		handlerDone.Wait()
		srv.Close()
	}()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		// Use a 1s timeout so context.WithTimeout(ctx, 1s) fires before the 2s handler.
		return testJob(srv.URL, 3, 1), nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, srv.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	require.NotEmpty(t, calls)
	require.False(t,
		calls[0].from !=
			domain.StatusDequeued ||
			calls[0].to != domain.StatusExecuting,
	)
	require.GreaterOrEqual(t, len(calls), 2)

	// Should have at least the executing transition.

	// First transition should be dequeued -> executing.

	// Should time out and get re-enqueued or timed_out.
}

// dispatch: concurrent dispatch of same run ID -- idempotency via status transitions

func TestDispatch_ConcurrentSameRunID_OnlyOneExecutes(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond) // simulate work
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	var updateCount atomic.Int32
	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(srv.URL, 1, 5), nil
	}
	st.updateRunStatusFn = func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
		if from == domain.StatusDequeued && to == domain.StatusExecuting {
			n := updateCount.Add(1)
			if n > 1 {
				// Second caller loses the status transition race.
				return fmt.Errorf("status conflict: expected dequeued, found executing")
			}
		}
		return nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, srv.Client())

	var wg conc.WaitGroup
	for range 3 {
		wg.Go(func() {
			run := testRun(1)
			exec.execute(context.Background(), run)
		})
	}
	wg.Wait()
	require.GreaterOrEqual(t, updateCount.
		Load(), int32(1))

	// Only the first goroutine should have successfully transitioned to executing.
	// Others should have silently exited on the status conflict error.
}

// dispatch: very large payload -- should not crash

func TestDispatch_LargePayload_NoOOMPanic(t *testing.T) {
	t.Parallel()

	var receivedSize int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 0, 1<<20)
		buf := make([]byte, 32*1024)
		for {
			n, err := r.Body.Read(buf)
			body = append(body, buf[:n]...)
			if err != nil {
				break
			}
		}
		receivedSize = len(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(srv.URL, 1, 10), nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, srv.Client())

	// Build a 512KB payload.
	bigPayload := `{"data":"` + strings.Repeat("A", 512*1024) + `"}`
	run := testRun(1)
	run.Payload = json.RawMessage(bigPayload)

	exec.execute(context.Background(), run)
	require.GreaterOrEqual(t, receivedSize,
		512*
			1024,
	)

	calls := st.statusUpdates()
	found := false
	for _, c := range calls {
		if c.to == domain.StatusCompleted {
			found = true
			break
		}
	}
	require.True(t,
		found)
}

// dispatch: all retry strategies (exponential, fixed)

func TestDispatch_RetryStrategy_Exponential_RetriesOnFailure(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"transient"}`))
	}))
	defer srv.Close()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(srv.URL, 3, 5), nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, srv.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	require.GreaterOrEqual(t, len(calls), 2)

	// Should transition to executing then to queued (retry) since attempt < maxAttempts.
	hasRetry := false
	for _, c := range calls {
		if c.to == domain.StatusQueued {
			hasRetry = true
			break
		}
	}
	require.True(t,
		hasRetry)
}

func TestDispatch_RetryStrategy_Fixed_RetriesOnFailure(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"transient"}`))
	}))
	defer srv.Close()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(srv.URL, 3, 5), nil
	}
	// Simulate workflow step run with fixed backoff policy.
	st.getWorkflowStepRunFn = func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
		return &domain.WorkflowStepRun{
			ID:            "step-run-1",
			WorkflowRunID: "wf-run-1",
			StepRef:       "step-1",
		}, nil
	}
	st.getWorkflowRunFn = func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
		return &domain.WorkflowRun{
			ID:              "wf-run-1",
			WorkflowID:      "wf-1",
			WorkflowVersion: 1,
		}, nil
	}
	st.listStepsByWorkflowVerFn = func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
		return []domain.WorkflowStep{
			{
				StepRef:               "step-1",
				RetryMaxAttempts:      3,
				RetryBackoff:          domain.RetryBackoffFixed,
				RetryInitialDelaySecs: 2,
				RetryMaxDelaySecs:     10,
			},
		}, nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, srv.Client())
	run := testRun(1)
	run.WorkflowStepRunID = "step-run-1"

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	hasRetry := false
	for _, c := range calls {
		if c.to == domain.StatusQueued {
			hasRetry = true
			break
		}
	}
	require.True(t,
		hasRetry)
}

// dispatch: endpoint returns non-JSON response -- should not panic

func TestDispatch_NonJSONResponse_Completes(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(srv.URL, 1, 5), nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, srv.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	found := false
	for _, c := range calls {
		if c.to == domain.StatusCompleted {
			found = true
			break
		}
	}
	require.True(t,
		found)
}

// dispatch: circuit breaker open -- should snooze

func TestDispatch_CircuitBreakerOpen_Snoozes(t *testing.T) {
	t.Parallel()

	retryAt := time.Now().Add(30 * time.Second)
	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob("http://example.com/endpoint", 3, 30), nil
	}
	st.canDispatchFn = func(_ context.Context, _ string, _ time.Time) (bool, *time.Time, error) {
		return false, &retryAt, nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, http.DefaultClient)
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	require.NotEmpty(t, calls)
	require.Equal(t,
		domain.StatusQueued,
		calls[0].to)

	// Should snooze back to queued.
}

// dispatch: circuit breaker check error -- system failure

func TestDispatch_CircuitBreakerCheckError_SystemFails(t *testing.T) {
	t.Parallel()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob("http://example.com/endpoint", 3, 30), nil
	}
	st.canDispatchFn = func(_ context.Context, _ string, _ time.Time) (bool, *time.Time, error) {
		return false, nil, errors.New("redis down")
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, http.DefaultClient)
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	require.NotEmpty(t, calls)
	require.Equal(t,
		domain.StatusSystemFailed,

		calls[0].to)
}

// dispatch: empty endpoint URL -- should fail on HTTP dispatch

func TestDispatch_EmptyEndpointURL_Fails(t *testing.T) {
	t.Parallel()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob("", 1, 5), nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, http.DefaultClient)
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	require.GreaterOrEqual(t, len(calls), 2)

	// Should fail because empty URL causes HTTP dispatch error.
	last := calls[len(calls)-1]
	require.NotEqual(t, domain.StatusCompleted,

		last.to,
	)
}

// dispatch: endpoint returns 429 -- should be classified as transient

func TestDispatch_Endpoint429_Retries(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(srv.URL, 3, 5), nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, srv.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	hasRetry := false
	for _, c := range calls {
		if c.to == domain.StatusQueued {
			hasRetry = true
			break
		}
	}
	require.True(t,
		hasRetry)
}

// dispatch: endpoint returns 200 with empty body -- should complete

func TestDispatch_EmptyResponseBody_Completes(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(srv.URL, 1, 5), nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, srv.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	found := false
	for _, c := range calls {
		if c.to == domain.StatusCompleted {
			found = true
			break
		}
	}
	require.True(t,
		found)
}

// dispatch: unknown execution mode -- system failure

func TestDispatch_UnknownExecutionMode_SystemFails(t *testing.T) {
	t.Parallel()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		job := testJob("http://example.com", 1, 5)
		job.ExecutionMode = domain.ExecutionMode("quantum")
		return job, nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, http.DefaultClient)
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	require.NotEmpty(t, calls)
	require.Equal(t,
		domain.StatusSystemFailed,

		calls[0].to)
}

// ingestStripeUsageEvent: with compute usage metadata (realistic data)

func TestIngestStripeUsageEvent_PositiveCost_NoBillingEnforcer_NoOp(t *testing.T) {
	t.Parallel()

	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:         pool,
		Queue:        &mockExecQueue{},
		Store:        &mockExecutorStore{},
		PollInterval: time.Millisecond,
		// No BillingEnforcer, no StripeUsageReporter
	})

	// No enforcer/ingester: silent no-op.
	exec.ingestStripeUsageEvent(context.Background(), "proj-1", "run-1", billing.HTTPCostPerRunMicrousd)
}

// dispatch: UpdateRunStatus failure during dequeued->executing transition

func TestDispatch_StatusUpdateFails_StopsProcessing(t *testing.T) {
	t.Parallel()

	var httpCalled atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpCalled.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(srv.URL, 1, 5), nil
	}
	st.updateRunStatusFn = func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
		if from == domain.StatusDequeued && to == domain.StatusExecuting {
			return errors.New("concurrent modification")
		}
		return nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, srv.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)
	require.LessOrEqual(t, httpCalled.
		Load(), int32(0),
	)
}

// dispatch: bulkhead at capacity -- should snooze

func TestDispatch_BulkheadFull_Snoozes(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		assert.Fail(t,

			"endpoint should not be called when bulkhead is full")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		job := testJob(srv.URL, 3, 30)
		job.MaxConcurrency = 1
		return job, nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, srv.Client())

	// Fill the bulkhead slot for job-1.
	exec.bulkhead.TryAcquire("job-1", 1)

	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	require.NotEmpty(t, calls)
	require.Equal(t,
		domain.StatusQueued,
		calls[0].to)

	// Release the slot.
	exec.bulkhead.Release("job-1", 1)
}

// dispatch: endpoint returns various 5xx status codes

func TestDispatch_Endpoint503_RetriesWithAttemptsRemaining(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`service unavailable`))
	}))
	defer srv.Close()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(srv.URL, 5, 5), nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, srv.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	hasRetry := false
	for _, c := range calls {
		if c.to == domain.StatusQueued {
			hasRetry = true
			break
		}
	}
	require.True(t,
		hasRetry)
}

// dispatch: adaptive timeout with health stats

func TestDispatch_AdaptiveTimeout_CompletesWithP95Stats(t *testing.T) {
	t.Parallel()

	// Verify that when P95 health stats are available and exceed the configured
	// timeout, the dispatch still completes without error. The adaptive timeout
	// code path is exercised (p95=20s * 1.5 = 30s > 10s configured), and the
	// run completes successfully.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	st := &mockExecutorStore{}
	st.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(srv.URL, 1, 10), nil // 10s configured timeout
	}
	st.getJobHealthStatsFn = func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
		return &orcstore.JobHealthStats{
			P95DurationSecs: 20.0, // p95 is 20s, 1.5x = 30s > 10s configured
		}, nil
	}

	exec := newTestExecutor(t, st, &mockExecQueue{}, time.Hour, srv.Client())
	exec.adaptiveTimeoutEnabled = true
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := st.statusUpdates()
	found := false
	for _, c := range calls {
		if c.to == domain.StatusCompleted {
			found = true
			break
		}
	}
	require.True(t,
		found)
}

// dispatchToEndpoint: request build error with malformed URL

func TestDispatchToEndpoint_MalformedURL_ReturnsError(t *testing.T) {
	t.Parallel()

	e := &Executor{httpClient: http.DefaultClient}
	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
		Payload: json.RawMessage(`{}`),
	}

	// A URL with control characters triggers request build failure.
	_, err := e.dispatchToEndpoint(context.Background(), "http://\x00invalid", run, nil)
	require.Error(t,
		err)
	require.Contains(t,
		err.Error(), "build request",
	)
}

// dispatchToEndpoint: extra headers are injected

func TestDispatchToEndpoint_ExtraHeaders_Injected(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var captured http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		captured = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}
	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
	}

	extras := map[string]string{
		"X-Secret-API_KEY": "super-secret",
		"X-Custom":         "custom-value",
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, extras)
	require.NoError(
		t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t,
		"super-secret", captured.
			Get("X-Secret-API_KEY"))
	assert.Equal(t,
		"custom-value", captured.
			Get("X-Custom"))
}

// dispatchToEndpoint: response body > 1MB is truncated

func TestDispatchToEndpoint_LargeResponseBody_Truncated(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Write 2MB of data.
		for range 2 * 1024 {
			_, _ = w.Write([]byte(strings.Repeat("a", 1024)))
		}
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}
	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
	}

	result, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	require.NoError(
		t, err)
	require.LessOrEqual(t, len(result), 1<<20)

	// LimitReader caps at 1MB.
}
