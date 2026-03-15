package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	orcstore "strait/internal/store"
	"strait/internal/testutil"
)

type statusUpdateCall struct {
	id     string
	from   domain.RunStatus
	to     domain.RunStatus
	fields map[string]any
}

type mockExecutorStore struct {
	getJobFn                 func(ctx context.Context, id string) (*domain.Job, error)
	getJobAtVersionFn        func(ctx context.Context, jobID string, version int) (*domain.Job, error)
	listSecretsFn            func(ctx context.Context, jobID, environment string) ([]domain.JobSecret, error)
	getWorkflowStepRunFn     func(ctx context.Context, id string) (*domain.WorkflowStepRun, error)
	getWorkflowRunFn         func(ctx context.Context, id string) (*domain.WorkflowRun, error)
	listStepsByWorkflowVerFn func(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	updateRunStatusFn        func(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	updateHeartbeatFn        func(ctx context.Context, id string) error
	batchUpdateHeartbeatFn   func(ctx context.Context, ids []string) error
	canDispatchFn            func(ctx context.Context, endpointURL string, now time.Time) (bool, *time.Time, error)
	recordFailureFn          func(ctx context.Context, endpointURL string, now time.Time, threshold int, openDuration time.Duration) error
	recordSuccessFn          func(ctx context.Context, endpointURL string) error
	getJobHealthStatsFn      func(ctx context.Context, jobID string, since time.Time) (*orcstore.JobHealthStats, error)
	getResolvedEnvVarsFn     func(ctx context.Context, id string) (map[string]string, error)
	getLatestCheckpointFn    func(ctx context.Context, runID string) (*domain.RunCheckpoint, error)

	mu              sync.Mutex
	statusCalls     []statusUpdateCall
	heartbeatRunIDs []string
}

func (m *mockExecutorStore) GetJob(ctx context.Context, id string) (*domain.Job, error) {
	if m.getJobFn == nil {
		return nil, nil
	}
	return m.getJobFn(ctx, id)
}

func (m *mockExecutorStore) GetJobAtVersion(ctx context.Context, jobID string, version int) (*domain.Job, error) {
	if m.getJobAtVersionFn != nil {
		return m.getJobAtVersionFn(ctx, jobID, version)
	}
	// Fall back to GetJob for backwards compatibility with existing tests.
	return m.GetJob(ctx, jobID)
}

func (m *mockExecutorStore) UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	m.mu.Lock()
	m.statusCalls = append(m.statusCalls, statusUpdateCall{
		id:     id,
		from:   from,
		to:     to,
		fields: maps.Clone(fields),
	})
	m.mu.Unlock()

	if m.updateRunStatusFn == nil {
		return nil
	}
	return m.updateRunStatusFn(ctx, id, from, to, fields)
}

func (m *mockExecutorStore) ListJobSecretsByJob(ctx context.Context, jobID, environment string) ([]domain.JobSecret, error) {
	if m.listSecretsFn == nil {
		return nil, nil
	}
	return m.listSecretsFn(ctx, jobID, environment)
}

func (m *mockExecutorStore) UpdateHeartbeat(ctx context.Context, id string) error {
	m.mu.Lock()
	m.heartbeatRunIDs = append(m.heartbeatRunIDs, id)
	m.mu.Unlock()

	if m.updateHeartbeatFn == nil {
		return nil
	}
	return m.updateHeartbeatFn(ctx, id)
}

func (m *mockExecutorStore) BatchUpdateHeartbeat(ctx context.Context, ids []string) error {
	m.mu.Lock()
	m.heartbeatRunIDs = append(m.heartbeatRunIDs, ids...)
	m.mu.Unlock()

	if m.batchUpdateHeartbeatFn == nil {
		return nil
	}
	return m.batchUpdateHeartbeatFn(ctx, ids)
}

func (m *mockExecutorStore) GetWorkflowStepRun(ctx context.Context, id string) (*domain.WorkflowStepRun, error) {
	if m.getWorkflowStepRunFn != nil {
		return m.getWorkflowStepRunFn(ctx, id)
	}
	return nil, nil
}

func (m *mockExecutorStore) GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	if m.getWorkflowRunFn != nil {
		return m.getWorkflowRunFn(ctx, id)
	}
	return nil, nil
}

func (m *mockExecutorStore) ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
	if m.listStepsByWorkflowVerFn != nil {
		return m.listStepsByWorkflowVerFn(ctx, workflowID, version)
	}
	return nil, nil
}

func (m *mockExecutorStore) CanDispatchEndpoint(ctx context.Context, endpointURL string, now time.Time) (bool, *time.Time, error) {
	if m.canDispatchFn == nil {
		return true, nil, nil
	}
	return m.canDispatchFn(ctx, endpointURL, now)
}

func (m *mockExecutorStore) RecordEndpointCircuitFailure(ctx context.Context, endpointURL string, now time.Time, threshold int, openDuration time.Duration) error {
	if m.recordFailureFn == nil {
		return nil
	}
	return m.recordFailureFn(ctx, endpointURL, now, threshold, openDuration)
}

func (m *mockExecutorStore) RecordEndpointCircuitSuccess(ctx context.Context, endpointURL string) error {
	if m.recordSuccessFn == nil {
		return nil
	}
	return m.recordSuccessFn(ctx, endpointURL)
}

func (m *mockExecutorStore) GetJobHealthStats(ctx context.Context, jobID string, since time.Time) (*orcstore.JobHealthStats, error) {
	if m.getJobHealthStatsFn == nil {
		return nil, nil
	}
	return m.getJobHealthStatsFn(ctx, jobID, since)
}

func (m *mockExecutorStore) GetResolvedEnvironmentVariables(ctx context.Context, id string) (map[string]string, error) {
	if m.getResolvedEnvVarsFn == nil {
		return nil, nil
	}
	return m.getResolvedEnvVarsFn(ctx, id)
}

func (m *mockExecutorStore) GetLatestCheckpoint(ctx context.Context, runID string) (*domain.RunCheckpoint, error) {
	if m.getLatestCheckpointFn == nil {
		return nil, nil
	}
	return m.getLatestCheckpointFn(ctx, runID)
}

func (m *mockExecutorStore) statusUpdates() []statusUpdateCall {
	m.mu.Lock()
	defer m.mu.Unlock()

	calls := make([]statusUpdateCall, len(m.statusCalls))
	copy(calls, m.statusCalls)
	return calls
}

type mockExecQueue struct {
	enqueueFn           func(ctx context.Context, run *domain.JobRun) error
	dequeueFn           func(ctx context.Context) (*domain.JobRun, error)
	dequeueNFn          func(ctx context.Context, n int) ([]domain.JobRun, error)
	dequeueNByProjectFn func(ctx context.Context, n int, projectID string) ([]domain.JobRun, error)
}

func (m *mockExecQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	if m.enqueueFn == nil {
		return nil
	}
	return m.enqueueFn(ctx, run)
}

func (m *mockExecQueue) Dequeue(ctx context.Context) (*domain.JobRun, error) {
	if m.dequeueFn == nil {
		return nil, nil
	}
	return m.dequeueFn(ctx)
}

func (m *mockExecQueue) DequeueN(ctx context.Context, n int) ([]domain.JobRun, error) {
	if m.dequeueNFn == nil {
		return nil, nil
	}
	return m.dequeueNFn(ctx, n)
}

func (m *mockExecQueue) DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	if m.dequeueNByProjectFn == nil {
		return nil, nil
	}
	return m.dequeueNByProjectFn(ctx, n, projectID)
}

var _ queue.Queue = (*mockExecQueue)(nil)

func testRun(attempt int) *domain.JobRun {
	return &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusDequeued,
		Attempt:   attempt,
		Payload:   json.RawMessage(`{"hello":"world"}`),
	}
}

func testJob(endpoint string, maxAttempts, timeoutSecs int) *domain.Job {
	return &domain.Job{
		ID:          "job-1",
		ProjectID:   "proj-1",
		EndpointURL: endpoint,
		MaxAttempts: maxAttempts,
		TimeoutSecs: timeoutSecs,
	}
}

func newTestExecutor(t *testing.T, store *mockExecutorStore, q queue.Queue, heartbeatInterval time.Duration, httpClient *http.Client) *Executor {
	t.Helper()

	pool := NewPool(4)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             store,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: heartbeatInterval,
		HTTPClient:        httpClient,
	})
	return exec
}

func waitForSignal(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal(msg)
	}
}

func TestExecutor_Dispatch_Success(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Run-ID") != "run-1" {
			t.Fatalf("X-Run-ID = %q, want %q", r.Header.Get("X-Run-ID"), "run-1")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}

	gotTransitions := []string{
		fmt.Sprintf("%s->%s", calls[0].from, calls[0].to),
		fmt.Sprintf("%s->%s", calls[1].from, calls[1].to),
	}
	testutil.AssertEqual(t, gotTransitions, []string{
		"dequeued->executing",
		"executing->completed",
	})

	gotResult, ok := calls[1].fields["result"].(json.RawMessage)
	if !ok {
		t.Fatalf("result field type = %T, want json.RawMessage", calls[1].fields["result"])
	}
	if string(gotResult) != `{"ok":true}` {
		t.Fatalf("result = %s, want %s", string(gotResult), `{"ok":true}`)
	}
	if run.Status != domain.StatusCompleted {
		t.Fatalf("run status = %s, want %s", run.Status, domain.StatusCompleted)
	}
}

func TestExecutor_Dispatch_IncludesSecretHeadersWhenEnabled(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Secret-API_KEY") != "super-secret" {
			t.Fatalf("X-Secret-API_KEY = %q, want %q", r.Header.Get("X-Secret-API_KEY"), "super-secret")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}
	store.listSecretsFn = func(_ context.Context, jobID, environment string) ([]domain.JobSecret, error) {
		if jobID != "job-1" || environment != "production" {
			t.Fatalf("unexpected args: %q %q", jobID, environment)
		}
		return []domain.JobSecret{{SecretKey: "API_KEY", EncryptedValue: "super-secret"}}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)
}

func TestExecutor_CircuitOpen_RequeuesBeforeExecuting(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	retryAt := time.Now().UTC().Add(45 * time.Second)
	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}
	store.canDispatchFn = func(context.Context, string, time.Time) (bool, *time.Time, error) {
		return false, &retryAt, nil
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:              NewPool(2),
		Queue:             &mockExecQueue{},
		Store:             store,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
		HTTPClient:        server.Client(),
	})
	t.Cleanup(func() { _ = exec.pool.Shutdown(context.Background()) })

	run := testRun(1)
	exec.execute(context.Background(), run)

	if called.Load() != 0 {
		t.Fatalf("dispatch called %d times, want 0", called.Load())
	}

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}
	if calls[0].from != domain.StatusDequeued || calls[0].to != domain.StatusQueued {
		t.Fatalf("transition = %s->%s, want %s->%s", calls[0].from, calls[0].to, domain.StatusDequeued, domain.StatusQueued)
	}
	gotRetryAt, ok := calls[0].fields["next_retry_at"].(*time.Time)
	if !ok || gotRetryAt == nil {
		t.Fatalf("next_retry_at field type = %T, want *time.Time", calls[0].fields["next_retry_at"])
	}
	if !gotRetryAt.Equal(retryAt) {
		t.Fatalf("next_retry_at = %v, want %v", *gotRetryAt, retryAt)
	}
}

func TestExecutor_CircuitBreaker_RecordsFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("temporary failure"))
	}))
	defer server.Close()

	var failureCalled atomic.Int32
	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}
	store.recordFailureFn = func(context.Context, string, time.Time, int, time.Duration) error {
		failureCalled.Add(1)
		return nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	if failureCalled.Load() != 1 {
		t.Fatalf("record failure called = %d, want 1", failureCalled.Load())
	}
}

func TestExecutor_CircuitBreaker_RecordsSuccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	var successCalled atomic.Int32
	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}
	store.recordSuccessFn = func(context.Context, string) error {
		successCalled.Add(1)
		return nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	if successCalled.Load() != 1 {
		t.Fatalf("record success called = %d, want 1", successCalled.Load())
	}
}

func TestExecutor_Bulkheads_AtCapacityRequeues(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		job := testJob(server.URL, 3, 5)
		job.MaxConcurrency = 1
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	exec.jobActiveRunsMu.Lock()
	exec.jobActiveRuns["job-1"] = 1
	exec.jobActiveRunsMu.Unlock()

	run := testRun(1)
	exec.execute(context.Background(), run)

	if called.Load() != 0 {
		t.Fatalf("dispatch called %d times, want 0", called.Load())
	}

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}
	if calls[0].from != domain.StatusDequeued || calls[0].to != domain.StatusQueued {
		t.Fatalf("transition = %s->%s, want %s->%s", calls[0].from, calls[0].to, domain.StatusDequeued, domain.StatusQueued)
	}
	if calls[0].fields["error"] != "job bulkhead at capacity" {
		t.Fatalf("error field = %v, want %q", calls[0].fields["error"], "job bulkhead at capacity")
	}
}

func TestExecutor_Bulkheads_EnabledUnderLimitExecutes(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		job := testJob(server.URL, 1, 5)
		job.MaxConcurrency = 1
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())

	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[0].to != domain.StatusExecuting || calls[1].to != domain.StatusCompleted {
		t.Fatalf("transitions = %s then %s, want executing then completed", calls[0].to, calls[1].to)
	}

	exec.jobActiveRunsMu.Lock()
	defer exec.jobActiveRunsMu.Unlock()
	if _, ok := exec.jobActiveRuns["job-1"]; ok {
		t.Fatal("bulkhead active run entry still present, want released")
	}
}

func TestExecutor_Dispatch_NonOKStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream exploded"))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	job := testJob(server.URL, 1, 5)
	run := testRun(1)

	err := exec.dispatch(context.Background(), job, run)
	if err == nil {
		t.Fatal("dispatch error = nil, want EndpointError")
	}

	var endpointErr *domain.EndpointError
	if !errors.As(err, &endpointErr) {
		t.Fatalf("dispatch error type = %T, want *domain.EndpointError", err)
	}
	if endpointErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("endpoint status = %d, want %d", endpointErr.StatusCode, http.StatusInternalServerError)
	}
	if endpointErr.Body != "upstream exploded" {
		t.Fatalf("endpoint body = %q, want %q", endpointErr.Body, "upstream exploded")
	}

	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusDeadLetter {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusDeadLetter)
	}
}

func TestExecutor_Dispatch_Timeout(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		attempt     int
		maxAttempts int
		wantStatus  domain.RunStatus
	}{
		{name: "retry queued", attempt: 1, maxAttempts: 2, wantStatus: domain.StatusQueued},
		{name: "final timed out", attempt: 2, maxAttempts: 2, wantStatus: domain.StatusTimedOut},
	}

	timeoutTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	httpClient := &http.Client{Transport: timeoutTransport}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &mockExecutorStore{}
			store.getJobFn = func(context.Context, string) (*domain.Job, error) {
				return testJob("http://timeout.test", tt.maxAttempts, 1), nil
			}

			exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, httpClient)
			run := testRun(tt.attempt)

			exec.execute(context.Background(), run)

			calls := store.statusUpdates()
			if len(calls) != 2 {
				t.Fatalf("status update calls = %d, want 2", len(calls))
			}
			if calls[1].to != tt.wantStatus {
				t.Fatalf("final status = %s, want %s", calls[1].to, tt.wantStatus)
			}
		})
	}
}

func TestExecutor_Dispatch_RetryOnFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("temporary failure"))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusQueued {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusQueued)
	}
	attempt, ok := calls[1].fields["attempt"].(int)
	if !ok {
		t.Fatalf("attempt field type = %T, want int", calls[1].fields["attempt"])
	}
	if attempt != 2 {
		t.Fatalf("attempt field = %d, want 2", attempt)
	}
}

func TestExecutor_SmartRetry_ClientErrorSkipsRetry(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid payload"))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusDeadLetter {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusDeadLetter)
	}
}

func TestExecutor_SmartRetry_ServerErrorRetries(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("temporary"))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusQueued {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusQueued)
	}
}

func TestExecutor_Fallback_TransientErrorUsesFallbackEndpoint(t *testing.T) {
	t.Parallel()
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer primary.Close()

	fallbackCalled := atomic.Int32{}
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fallbackCalled.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"source":"fallback"}`))
	}))
	defer fallback.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		job := testJob(primary.URL, 2, 5)
		job.FallbackEndpointURL = fallback.URL
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, primary.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	if fallbackCalled.Load() != 1 {
		t.Fatalf("fallback call count = %d, want 1", fallbackCalled.Load())
	}

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
}

func TestExecutor_Fallback_ClientErrorDoesNotUseFallback(t *testing.T) {
	t.Parallel()
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer primary.Close()

	fallbackCalled := atomic.Int32{}
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fallbackCalled.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"source":"fallback"}`))
	}))
	defer fallback.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		job := testJob(primary.URL, 1, 5)
		job.FallbackEndpointURL = fallback.URL
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, primary.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	if fallbackCalled.Load() != 0 {
		t.Fatalf("fallback call count = %d, want 0", fallbackCalled.Load())
	}

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusDeadLetter {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusDeadLetter)
	}
}

func TestExecutor_Dispatch_FinalFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("hard failure"))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 2, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(2)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusDeadLetter {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusDeadLetter)
	}
	if run.Status != domain.StatusDeadLetter {
		t.Fatalf("run status = %s, want %s", run.Status, domain.StatusDeadLetter)
	}
}

func TestExecutor_DLQ_TransitionsToDeadLetterOnExhaustedRetries(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("hard failure"))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())

	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusDeadLetter {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusDeadLetter)
	}
	if run.Status != domain.StatusDeadLetter {
		t.Fatalf("run status = %s, want %s", run.Status, domain.StatusDeadLetter)
	}
}

func TestExecutor_HandleSystemFailure(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, nil)
	run := testRun(1)
	run.Status = domain.StatusExecuting

	exec.handleSystemFailure(context.Background(), run, "db unavailable")

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}
	if calls[0].from != domain.StatusExecuting || calls[0].to != domain.StatusSystemFailed {
		t.Fatalf("transition = %s->%s, want %s->%s", calls[0].from, calls[0].to, domain.StatusExecuting, domain.StatusSystemFailed)
	}
}

func TestExecutor_Poll_NoAvailableSlots(t *testing.T) {
	t.Parallel()
	pool := NewPool(1)
	defer func() { _ = pool.Shutdown(context.Background()) }()

	started := make(chan struct{})
	release := make(chan struct{})
	pool.Submit(context.Background(), func() {
		close(started)
		<-release
	})
	waitForSignal(t, started, "blocking task did not start")

	var called atomic.Int32
	q := &mockExecQueue{
		dequeueNFn: func(context.Context, int) ([]domain.JobRun, error) {
			called.Add(1)
			return nil, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             &mockExecutorStore{},
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
	})

	exec.poll(context.Background())
	if called.Load() != 0 {
		t.Fatalf("DequeueN call count = %d, want 0", called.Load())
	}

	close(release)
}

func TestExecutor_Poll_EmptyQueue(t *testing.T) {
	t.Parallel()
	var dequeueCalls atomic.Int32
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, n int) ([]domain.JobRun, error) {
			dequeueCalls.Add(1)
			if n <= 0 {
				t.Fatalf("dequeue n = %d, want > 0", n)
			}
			return []domain.JobRun{}, nil
		},
	}

	exec := newTestExecutor(t, &mockExecutorStore{}, q, time.Hour, nil)
	exec.poll(context.Background())

	if dequeueCalls.Load() != 1 {
		t.Fatalf("DequeueN call count = %d, want 1", dequeueCalls.Load())
	}
	if got := exec.pool.ActiveCount(); got != 0 {
		t.Fatalf("pool active count = %d, want 0", got)
	}
}

func TestExecutor_GracefulShutdown(t *testing.T) {
	t.Parallel()
	jobStarted := make(chan struct{})
	jobCanProceed := make(chan struct{})

	var startedOnce sync.Once
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		startedOnce.Do(func() { close(jobStarted) })
		<-jobCanProceed
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer ts.Close()

	var transitionsMu sync.Mutex
	transitions := make([]string, 0, 2)

	store := &mockExecutorStore{}
	store.getJobFn = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{
			ID:          id,
			EndpointURL: ts.URL,
			MaxAttempts: 3,
			TimeoutSecs: 30,
		}, nil
	}
	store.updateRunStatusFn = func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
		transitionsMu.Lock()
		transitions = append(transitions, fmt.Sprintf("%s->%s", from, to))
		transitionsMu.Unlock()
		return nil
	}

	var dequeueCount atomic.Int32
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			if dequeueCount.Add(1) == 1 {
				return []domain.JobRun{{
					ID:      "run-shutdown-1",
					JobID:   "job-1",
					Status:  domain.StatusDequeued,
					Attempt: 1,
				}}, nil
			}
			return nil, nil
		},
	}

	pool := NewPool(5)
	ctx, cancel := context.WithCancel(context.Background())

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             store,
		HTTPClient:        ts.Client(),
		PollInterval:      5 * time.Millisecond,
		HeartbeatInterval: time.Hour,
	})

	runDone := make(chan struct{})
	go func() {
		exec.Run(ctx)
		close(runDone)
	}()

	select {
	case <-jobStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for job to start")
	}

	cancel()

	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not exit after context cancellation")
	}

	close(jobCanProceed)

	shutdownDone := make(chan struct{})
	go func() {
		_ = pool.Shutdown(context.Background())
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
	case <-time.After(5 * time.Second):
		t.Fatal("pool.Shutdown() did not return")
	}

	transitionsMu.Lock()
	defer transitionsMu.Unlock()

	if len(transitions) < 2 {
		t.Fatalf("expected at least 2 transitions, got %d: %v", len(transitions), transitions)
	}
	if transitions[0] != "dequeued->executing" {
		t.Errorf("first transition = %s, want dequeued->executing", transitions[0])
	}
	last := transitions[len(transitions)-1]
	if last != "executing->completed" {
		t.Errorf("last transition = %s, want executing->completed", last)
	}
}

func TestExecutor_Run_PollsOnWakeSignal(t *testing.T) {
	t.Parallel()

	wake := make(chan struct{}, 1)
	polled := make(chan struct{}, 1)

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			select {
			case polled <- struct{}{}:
			default:
			}
			return nil, nil
		},
	}

	pool := NewPool(1)
	defer func() { _ = pool.Shutdown(context.Background()) }()

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Wake:              wake,
		Store:             &mockExecutorStore{},
		PollInterval:      time.Hour,
		HeartbeatInterval: time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		exec.Run(ctx)
		close(done)
	}()

	wake <- struct{}{}

	select {
	case <-polled:
	case <-time.After(time.Second):
		t.Fatal("expected poll to run after wake signal")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("executor did not stop after context cancel")
	}
}

func TestExecutor_Shutdown_NoInFlight(t *testing.T) {
	t.Parallel()

	exec := newTestExecutor(t, &mockExecutorStore{}, &mockExecQueue{}, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	runDone := make(chan struct{})
	go func() {
		exec.Run(ctx)
		close(runDone)
	}()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()

	if err := exec.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v, want nil", err)
	}

	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("executor Run did not stop after shutdown")
	}
}

func TestExecutor_Shutdown_WaitsForInFlight(t *testing.T) {
	t.Parallel()

	pollStarted := make(chan struct{})
	allowPollExit := make(chan struct{})
	wake := make(chan struct{}, 1)

	q := &mockExecQueue{
		dequeueNFn: func(ctx context.Context, _ int) ([]domain.JobRun, error) {
			select {
			case <-pollStarted:
			default:
				close(pollStarted)
			}
			select {
			case <-allowPollExit:
				return nil, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}

	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Wake:              wake,
		Store:             &mockExecutorStore{},
		PollInterval:      time.Hour,
		HeartbeatInterval: time.Hour,
	})

	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	runDone := make(chan struct{})
	go func() {
		exec.Run(runCtx)
		close(runDone)
	}()

	wake <- struct{}{}
	waitForSignal(t, pollStarted, "poll did not start")

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		shutdownDone <- exec.Shutdown(shutdownCtx)
	}()

	select {
	case err := <-shutdownDone:
		t.Fatalf("Shutdown returned early with err=%v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(allowPollExit)

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not return after poll completed")
	}

	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("executor Run did not stop after shutdown")
	}
}

func TestExecutor_Shutdown_Timeout(t *testing.T) {
	t.Parallel()

	pollStarted := make(chan struct{})
	allowPollExit := make(chan struct{})
	wake := make(chan struct{}, 1)

	q := &mockExecQueue{
		dequeueNFn: func(ctx context.Context, _ int) ([]domain.JobRun, error) {
			select {
			case <-pollStarted:
			default:
				close(pollStarted)
			}
			select {
			case <-allowPollExit:
				return nil, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}

	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Wake:              wake,
		Store:             &mockExecutorStore{},
		PollInterval:      time.Hour,
		HeartbeatInterval: time.Hour,
	})

	runCtx, runCancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		exec.Run(runCtx)
		close(runDone)
	}()

	wake <- struct{}{}
	waitForSignal(t, pollStarted, "poll did not start")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer shutdownCancel()
	err := exec.Shutdown(shutdownCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want %v", err, context.DeadlineExceeded)
	}

	runCancel()
	close(allowPollExit)
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("executor Run did not stop after cancel")
	}
}

func TestExecutor_Poll_DequeueError(t *testing.T) {
	t.Parallel()
	var dequeueCalls atomic.Int32
	q := &mockExecQueue{
		dequeueNFn: func(context.Context, int) ([]domain.JobRun, error) {
			dequeueCalls.Add(1)
			return nil, errors.New("queue down")
		},
	}

	exec := newTestExecutor(t, &mockExecutorStore{}, q, time.Hour, nil)
	exec.poll(context.Background())

	if dequeueCalls.Load() != 1 {
		t.Fatalf("DequeueN call count = %d, want 1", dequeueCalls.Load())
	}
}

func TestExecutor_Execute_JobLookupFails(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return nil, errors.New("job lookup failed")
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, nil)
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}
	if calls[0].to != domain.StatusSystemFailed {
		t.Fatalf("final status = %s, want %s", calls[0].to, domain.StatusSystemFailed)
	}
}

func TestExecutor_Execute_StatusTransitionFails(t *testing.T) {
	t.Parallel()
	hit := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hit <- struct{}{}
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}
	store.updateRunStatusFn = func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
		if from == domain.StatusDequeued && to == domain.StatusExecuting {
			return errors.New("write conflict")
		}
		return nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}

	select {
	case <-hit:
		t.Fatal("dispatch was called after transition failure")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHeartbeatSender_Run(t *testing.T) {
	t.Parallel()
	beats := make(chan struct{}, 10)
	store := &mockExecutorStore{}
	store.batchUpdateHeartbeatFn = func(_ context.Context, ids []string) error {
		if len(ids) != 1 || ids[0] != "run-1" {
			t.Fatalf("batch ids = %v, want [run-1]", ids)
		}
		beats <- struct{}{}
		return nil
	}

	hb := NewHeartbeatSender(store, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		hb.Run(ctx, "run-1")
		close(done)
	}()

	for i := range 2 {
		select {
		case <-beats:
		case <-time.After(300 * time.Millisecond):
			t.Fatalf("heartbeat %d not received in time", i+1)
		}
	}

	cancel()
	waitForSignal(t, done, "heartbeat sender did not stop after cancel")
}

func TestExecutor_Dispatch_EmptyResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
	if _, ok := calls[1].fields["result"]; ok {
		t.Fatal("result field present for empty response, want absent")
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestExecutor_NilMetrics(t *testing.T) {
	t.Parallel()
	errTransport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})

	exec := NewExecutor(ExecutorConfig{
		Pool:              NewPool(1),
		Queue:             &mockExecQueue{},
		Store:             &mockExecutorStore{},
		HTTPClient:        &http.Client{Transport: errTransport},
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
	})
	t.Cleanup(func() { _ = exec.pool.Shutdown(context.Background()) })

	job := testJob("http://example.invalid", 1, 5)
	run := testRun(1)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("dispatch panicked with nil metrics: %v", r)
		}
	}()

	if err := exec.dispatch(context.Background(), job, run); err == nil {
		t.Fatal("dispatch error = nil, want non-nil")
	}
}

func TestSendWebhookOnce_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing Content-Type header")
		}
		if r.Header.Get("X-Run-ID") == "" {
			t.Error("missing X-Run-ID header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusCompleted}

	result := sendWebhookOnce(t.Context(), job, run)
	if !result.Delivered {
		t.Errorf("Delivered = false, want true")
	}
	if result.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
}

func TestSendWebhookOnce_ServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusFailed}

	result := sendWebhookOnce(t.Context(), job, run)
	if result.Delivered {
		t.Error("Delivered = true, want false")
	}
	if result.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", result.StatusCode)
	}
}

func TestSendWebhookOnce_ClientError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusFailed}

	result := sendWebhookOnce(t.Context(), job, run)
	if result.Delivered {
		t.Error("Delivered = true, want false")
	}
	if result.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", result.StatusCode)
	}
}

func TestSendWebhookOnce_WithSignature(t *testing.T) {
	t.Parallel()
	var gotSig string
	var gotStraitSig string
	var gotTimestamp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Webhook-Signature")
		gotStraitSig = r.Header.Get("X-Strait-Signature")
		gotTimestamp = r.Header.Get("X-Strait-Timestamp")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL, WebhookSecret: "my-secret"}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}

	result := sendWebhookOnce(t.Context(), job, run)
	if !result.Delivered {
		t.Fatal("Delivered = false")
	}
	if gotSig == "" {
		t.Error("expected X-Webhook-Signature header")
	}
	if len(gotSig) < 5 || gotSig[:3] != "v1=" {
		t.Errorf("signature format wrong: %s", gotSig)
	}
	if gotStraitSig == "" {
		t.Error("expected X-Strait-Signature header")
	}
	if gotTimestamp == "" {
		t.Error("expected X-Strait-Timestamp header")
	}
}

func TestSendWebhookOnce_PayloadContent(t *testing.T) {
	t.Parallel()
	var gotPayload WebhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotPayload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{
		ID:        "run-123",
		JobID:     "job-456",
		ProjectID: "proj-789",
		Status:    domain.StatusCompleted,
		Attempt:   2,
	}

	sendWebhookOnce(t.Context(), job, run)

	if gotPayload.RunID != "run-123" {
		t.Errorf("RunID = %s, want run-123", gotPayload.RunID)
	}
	if gotPayload.JobID != "job-456" {
		t.Errorf("JobID = %s, want job-456", gotPayload.JobID)
	}
	if gotPayload.ProjectID != "proj-789" {
		t.Errorf("ProjectID = %s, want proj-789", gotPayload.ProjectID)
	}
	if gotPayload.Status != "completed" {
		t.Errorf("Status = %s, want completed", gotPayload.Status)
	}
	if gotPayload.Attempt != 2 {
		t.Errorf("Attempt = %d, want 2", gotPayload.Attempt)
	}
}

func TestSendWebhookOnce_NetworkError(t *testing.T) {
	t.Parallel()
	job := &domain.Job{WebhookURL: "http://localhost:59999/webhook"}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusFailed}

	result := sendWebhookOnce(t.Context(), job, run)
	if result.Delivered {
		t.Error("Delivered = true, want false")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestSendWebhookWithRetry_EmptyURL(t *testing.T) {
	t.Parallel()
	job := &domain.Job{WebhookURL: ""}
	run := &domain.JobRun{ID: "run-1"}

	result := SendWebhookWithRetry(t.Context(), job, run, 3)
	if !result.Delivered {
		t.Error("Delivered = false, want true for empty URL")
	}
}

func TestSendWebhookWithRetry_SuccessFirstAttempt(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}

	result := SendWebhookWithRetry(t.Context(), job, run, 3)
	if !result.Delivered {
		t.Error("Delivered = false")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
}

func TestSendWebhookWithRetry_ClientErrorNoRetry(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusFailed}

	result := SendWebhookWithRetry(t.Context(), job, run, 3)
	if result.Delivered {
		t.Error("Delivered = true, want false")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (should not retry 4xx)", got)
	}
	if result.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", result.StatusCode)
	}
}

func TestSendWebhookWithRetry_DefaultMaxAttempts(t *testing.T) {
	t.Parallel()
	job := &domain.Job{WebhookURL: ""}
	run := &domain.JobRun{ID: "run-1"}

	result := SendWebhookWithRetry(t.Context(), job, run, 0)
	if !result.Delivered {
		t.Error("Delivered = false")
	}
}

func TestSendWebhookWithRetry_ContextCanceled(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusFailed}

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result := SendWebhookWithRetry(ctx, job, run, 3)
	if result.Delivered {
		t.Error("Delivered = true, want false")
	}
	if got := attempts.Load(); got < 1 {
		t.Errorf("attempts = %d, want >= 1", got)
	}
}

func TestSendWebhookWithRetry_SuccessOnSecondAttempt(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}

	result := SendWebhookWithRetry(t.Context(), job, run, 3)
	if !result.Delivered {
		t.Error("Delivered = false, want true")
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

func TestSendWebhookWithRetry_ExhaustsAllRetries(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusFailed}

	result := SendWebhookWithRetry(t.Context(), job, run, 2)
	if result.Delivered {
		t.Error("Delivered = true, want false")
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

func TestExecutor_PanicRecovery(t *testing.T) {
	t.Parallel()
	pool := NewPool(1)
	defer func() { _ = pool.Shutdown(context.Background()) }()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		panic("simulated crash in job lookup")
	}

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			return []domain.JobRun{*testRun(1)}, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             store,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
	})

	exec.poll(context.Background())
	_ = pool.Shutdown(context.Background())

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}
	if calls[0].to != domain.StatusSystemFailed {
		t.Fatalf("status = %s, want %s", calls[0].to, domain.StatusSystemFailed)
	}
	errMsg, ok := calls[0].fields["error"].(string)
	if !ok {
		t.Fatal("expected error field in status update")
	}
	if !strings.Contains(errMsg, "panic:") {
		t.Fatalf("error = %q, want to contain 'panic:'", errMsg)
	}
}

func TestExecutor_PanicRecovery_ErrorValue(t *testing.T) {
	t.Parallel()
	pool := NewPool(1)
	defer func() { _ = pool.Shutdown(context.Background()) }()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		panic(errors.New("runtime error: index out of range"))
	}

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			return []domain.JobRun{*testRun(1)}, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             store,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
	})

	exec.poll(context.Background())
	_ = pool.Shutdown(context.Background())

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}
	if calls[0].to != domain.StatusSystemFailed {
		t.Fatalf("status = %s, want %s", calls[0].to, domain.StatusSystemFailed)
	}
}

func TestExecutor_Poll_UsesProjectPartitionDequeue(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}

	called := false
	q := &mockExecQueue{
		dequeueNByProjectFn: func(_ context.Context, n int, projectID string) ([]domain.JobRun, error) {
			called = true
			if n <= 0 {
				t.Fatalf("expected positive dequeue size")
			}
			if projectID != "proj-a" {
				t.Fatalf("projectID = %q, want %q", projectID, "proj-a")
			}
			return nil, nil
		},
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			t.Fatal("did not expect global DequeueN when partitions are configured")
			return nil, nil
		},
	}

	pool := NewPool(1)
	defer func() { _ = pool.Shutdown(context.Background()) }()

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             store,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Second,
		Partitions:        []string{"proj-a"},
	})

	exec.poll(context.Background())
	if !called {
		t.Fatal("expected partitioned dequeue to be called")
	}
}

func TestBuildPartitionCycle_Weights(t *testing.T) {
	t.Parallel()
	cycle := buildPartitionCycle([]string{"proj-a", "proj-b"}, "proj-a:2,proj-b:1")
	if len(cycle) != 3 {
		t.Fatalf("cycle len = %d, want 3", len(cycle))
	}
	testutil.AssertEqual(t, cycle, []string{"proj-a", "proj-a", "proj-b"})
}

func BenchmarkExecutorPoll(b *testing.B) {
	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob("http://example.invalid", 1, 1), nil
	}

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, n int) ([]domain.JobRun, error) {
			runs := make([]domain.JobRun, 0, n)
			for i := range n {
				runs = append(runs, domain.JobRun{
					ID:        fmt.Sprintf("run-%d", i),
					JobID:     "job-1",
					ProjectID: "proj-1",
					Status:    domain.StatusDequeued,
					Attempt:   1,
				})
			}
			return runs, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:              NewPool(16),
		Queue:             q,
		Store:             store,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
		HTTPClient:        &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("skip") })},
	})
	defer func() { _ = exec.pool.Shutdown(context.Background()) }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		exec.poll(context.Background())
	}
}

func TestExecutor_ExecutionTracing_Enabled_CapturesTrace(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.CreatedAt = time.Now().Add(-50 * time.Millisecond)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	traceValue, ok := calls[1].fields["execution_trace"]
	if !ok {
		t.Fatal("execution_trace missing from terminal update fields")
	}
	trace, ok := traceValue.(*domain.ExecutionTrace)
	if !ok {
		t.Fatalf("execution_trace type = %T, want *domain.ExecutionTrace", traceValue)
	}
	if trace.QueueWaitMs <= 0 {
		t.Fatalf("QueueWaitMs = %d, want > 0", trace.QueueWaitMs)
	}
	if trace.DispatchMs <= 0 {
		t.Fatalf("DispatchMs = %d, want > 0", trace.DispatchMs)
	}
	if trace.TotalMs <= 0 {
		t.Fatalf("TotalMs = %d, want > 0", trace.TotalMs)
	}
}

func TestExecutor_ExecutionTracing_OnFailure_CapturesTrace(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream failed"))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.CreatedAt = time.Now().Add(-50 * time.Millisecond)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusDeadLetter {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusDeadLetter)
	}
	traceValue, ok := calls[1].fields["execution_trace"]
	if !ok {
		t.Fatal("execution_trace missing from failure terminal update")
	}
	trace, ok := traceValue.(*domain.ExecutionTrace)
	if !ok {
		t.Fatalf("execution_trace type = %T, want *domain.ExecutionTrace", traceValue)
	}
	if trace.QueueWaitMs <= 0 {
		t.Fatalf("QueueWaitMs = %d, want > 0", trace.QueueWaitMs)
	}
	if trace.DispatchMs <= 0 {
		t.Fatalf("DispatchMs = %d, want > 0", trace.DispatchMs)
	}
	if trace.TotalMs <= 0 {
		t.Fatalf("TotalMs = %d, want > 0", trace.TotalMs)
	}
}

func TestExecutor_ExecutionTracing_OnTimeout_CapturesTrace(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 1), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.CreatedAt = time.Now().Add(-50 * time.Millisecond)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusTimedOut {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusTimedOut)
	}
	traceValue, ok := calls[1].fields["execution_trace"]
	if !ok {
		t.Fatal("execution_trace missing from timeout terminal update")
	}
	trace, ok := traceValue.(*domain.ExecutionTrace)
	if !ok {
		t.Fatalf("execution_trace type = %T, want *domain.ExecutionTrace", traceValue)
	}
	if trace.QueueWaitMs <= 0 {
		t.Fatalf("QueueWaitMs = %d, want > 0", trace.QueueWaitMs)
	}
	if trace.TotalMs <= 0 {
		t.Fatalf("TotalMs = %d, want > 0", trace.TotalMs)
	}
}

func TestExecutor_AdaptiveTimeout_UsesP95WhenHigherThanStatic(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 1), nil
	}
	store.getJobHealthStatsFn = func(context.Context, string, time.Time) (*orcstore.JobHealthStats, error) {
		return &orcstore.JobHealthStats{P95DurationSecs: 2.0}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
	if calls[1].to == domain.StatusTimedOut {
		t.Fatal("run timed out with adaptive timeout enabled, want completed")
	}
}

func TestExecutor_AdaptiveTimeout_FallsBackToStaticWhenP95Lower(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 3), nil
	}
	store.getJobHealthStatsFn = func(context.Context, string, time.Time) (*orcstore.JobHealthStats, error) {
		return &orcstore.JobHealthStats{P95DurationSecs: 0.5}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
}

func TestExecutor_AdaptiveTimeout_FallsBackOnError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 2), nil
	}
	store.getJobHealthStatsFn = func(context.Context, string, time.Time) (*orcstore.JobHealthStats, error) {
		return nil, errors.New("health stats unavailable")
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
}

func TestExecutor_EnvironmentOverride_Success(t *testing.T) {
	t.Parallel()
	// The override server should receive the request, not the original.
	overrideCalled := false
	overrideServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		overrideCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"from":"override"}`))
	}))
	defer overrideServer.Close()

	originalServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("original server should not be called when override is active")
	}))
	defer originalServer.Close()

	overrideParsed, err := url.Parse(overrideServer.URL)
	if err != nil {
		t.Fatalf("parse override server url: %v", err)
	}
	overrideURL := "http://example.com" + ":" + overrideParsed.Port()

	transport := overrideServer.Client().Transport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", overrideParsed.Host)
	}
	client := &http.Client{Transport: transport}

	store := &mockExecutorStore{}
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		job := testJob(originalServer.URL, 1, 5)
		job.EnvironmentID = "env-1"
		return job, nil
	}
	store.getResolvedEnvVarsFn = func(_ context.Context, id string) (map[string]string, error) {
		if id != "env-1" {
			t.Fatalf("unexpected environment ID: %q", id)
		}
		return map[string]string{"ENDPOINT_URL": overrideURL}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, client)
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
	if !overrideCalled {
		t.Fatal("override server should have been called")
	}
}

func TestExecutor_EnvironmentOverride_ErrorFallsBackToOriginal(t *testing.T) {
	t.Parallel()
	// When env resolution fails, the original endpoint should be used.
	originalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"from":"original"}`))
	}))
	defer originalServer.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		job := testJob(originalServer.URL, 1, 5)
		job.EnvironmentID = "env-1"
		return job, nil
	}
	store.getResolvedEnvVarsFn = func(_ context.Context, _ string) (map[string]string, error) {
		return nil, errors.New("env resolution failed")
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, originalServer.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
}

func TestExecutor_EnvironmentOverride_SSRFBlocked(t *testing.T) {
	t.Parallel()
	// Override to a private IP should be rejected; original endpoint used.
	originalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"from":"original"}`))
	}))
	defer originalServer.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		job := testJob(originalServer.URL, 1, 5)
		job.EnvironmentID = "env-1"
		return job, nil
	}
	store.getResolvedEnvVarsFn = func(_ context.Context, _ string) (map[string]string, error) {
		// Try to override to AWS metadata endpoint (SSRF attack)
		return map[string]string{"ENDPOINT_URL": "http://169.254.169.254/latest/meta-data/"}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, originalServer.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	// Should complete using original endpoint, not the blocked override.
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
}

func TestExecutor_EnvironmentOverride_EmptyValueKeepsOriginal(t *testing.T) {
	t.Parallel()
	// Empty ENDPOINT_URL should not override.
	originalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"from":"original"}`))
	}))
	defer originalServer.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		job := testJob(originalServer.URL, 1, 5)
		job.EnvironmentID = "env-1"
		return job, nil
	}
	store.getResolvedEnvVarsFn = func(_ context.Context, _ string) (map[string]string, error) {
		return map[string]string{"ENDPOINT_URL": ""}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, originalServer.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
}

func TestExecute_UsesVersionedJobConfig(t *testing.T) {
	t.Parallel()

	// v1 endpoint (the one the run was enqueued with)
	v1Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":"v1"}`))
	}))
	defer v1Server.Close()

	// v2 endpoint (the current/live endpoint — should NOT be used)
	v2Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("v2 endpoint was called — executor should have used v1")
		w.WriteHeader(http.StatusOK)
	}))
	defer v2Server.Close()

	store := &mockExecutorStore{}

	// GetJob returns the "current" v2 config
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(v2Server.URL, 1, 5), nil
	}

	// GetJobAtVersion returns the v1 snapshot
	store.getJobAtVersionFn = func(_ context.Context, _ string, version int) (*domain.Job, error) {
		if version == 1 {
			return testJob(v1Server.URL, 1, 5), nil
		}
		return testJob(v2Server.URL, 1, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, v1Server.Client())

	run := testRun(1)
	run.JobVersion = 1

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
}

func TestExecute_FallsBackToLiveJob(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := &mockExecutorStore{}

	// GetJob returns live config (the fallback)
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}

	// GetJobAtVersion delegates to GetJob (simulating no snapshot exists)
	store.getJobAtVersionFn = func(ctx context.Context, jobID string, _ int) (*domain.Job, error) {
		return store.GetJob(ctx, jobID)
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())

	run := testRun(1)
	run.JobVersion = 1

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
}

func TestExecute_VersionedConfig_PreservesTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := &mockExecutorStore{}

	// Live job has timeout=1s, versioned snapshot has timeout=300s
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(server.URL, 1, 1), nil
	}
	store.getJobAtVersionFn = func(_ context.Context, _ string, version int) (*domain.Job, error) {
		if version == 1 {
			return testJob(server.URL, 1, 300), nil // original generous timeout
		}
		return testJob(server.URL, 1, 1), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())

	run := testRun(1)
	run.JobVersion = 1

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s (v1 timeout should be 300s not 1s)", calls[1].to, domain.StatusCompleted)
	}
}

func TestResolveJobForRun_Pin(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 3, VersionID: "ver_v3", VersionPolicy: domain.VersionPolicyPin,
				EndpointURL: "https://v3.example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
		getJobAtVersionFn: func(_ context.Context, _ string, v int) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: v, VersionID: "ver_v1",
				EndpointURL: "https://v1.example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1, Status: domain.StatusDequeued}

	job, err := e.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.EndpointURL != "https://v1.example.com" {
		t.Fatalf("expected v1 endpoint, got %s", job.EndpointURL)
	}
	if run.JobVersion != 1 {
		t.Fatalf("expected run version to stay 1, got %d", run.JobVersion)
	}
}

func TestResolveJobForRun_Latest(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 3, VersionID: "ver_v3", VersionPolicy: domain.VersionPolicyLatest,
				EndpointURL: "https://v3.example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1, Status: domain.StatusDequeued}

	job, err := e.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.EndpointURL != "https://v3.example.com" {
		t.Fatalf("expected v3 endpoint, got %s", job.EndpointURL)
	}
	if run.JobVersion != 3 {
		t.Fatalf("expected run version upgraded to 3, got %d", run.JobVersion)
	}
	if run.JobVersionID != "ver_v3" {
		t.Fatalf("expected run version_id upgraded to ver_v3, got %s", run.JobVersionID)
	}
}

func TestResolveJobForRun_Minor_Compatible(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 3, VersionID: "ver_v3", VersionPolicy: domain.VersionPolicyMinor,
				BackwardsCompatible: true,
				EndpointURL:         "https://v3.example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1, Status: domain.StatusDequeued}

	job, err := e.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.EndpointURL != "https://v3.example.com" {
		t.Fatalf("expected v3 endpoint, got %s", job.EndpointURL)
	}
	if run.JobVersion != 3 {
		t.Fatalf("expected run version upgraded to 3, got %d", run.JobVersion)
	}
}

func TestResolveJobForRun_Minor_Incompatible(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 3, VersionID: "ver_v3", VersionPolicy: domain.VersionPolicyMinor,
				BackwardsCompatible: false,
				EndpointURL:         "https://v3.example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
		getJobAtVersionFn: func(_ context.Context, _ string, v int) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: v, VersionID: "ver_v1",
				EndpointURL: "https://v1.example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1, Status: domain.StatusDequeued}

	job, err := e.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.EndpointURL != "https://v1.example.com" {
		t.Fatalf("expected v1 endpoint (no upgrade), got %s", job.EndpointURL)
	}
	if run.JobVersion != 1 {
		t.Fatalf("expected run version to stay 1, got %d", run.JobVersion)
	}
}

func TestResolveJobForRun_SameVersion(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 2, VersionID: "ver_v2", VersionPolicy: domain.VersionPolicyLatest,
				EndpointURL: "https://v2.example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 2, Status: domain.StatusDequeued}

	job, err := e.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.EndpointURL != "https://v2.example.com" {
		t.Fatalf("expected current endpoint, got %s", job.EndpointURL)
	}
}

func TestAdaptiveDequeue_SkipsWhenPoolSaturated(t *testing.T) {
	t.Parallel()

	var dequeueCalled atomic.Bool
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			dequeueCalled.Store(true)
			return nil, nil
		},
	}

	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	// Saturate the pool
	started := make(chan struct{})
	done := make(chan struct{})
	pool.Submit(context.Background(), func() {
		close(started)
		<-done
	})
	<-started

	exec := NewExecutor(ExecutorConfig{
		Pool:         pool,
		Queue:        q,
		Store:        &mockExecutorStore{},
		PollInterval: time.Hour,
	})

	exec.poll(context.Background())
	close(done)

	if dequeueCalled.Load() {
		t.Fatal("expected dequeue to be skipped when pool is saturated")
	}
}

func TestAdaptiveDequeue_UsesIdleCount(t *testing.T) {
	t.Parallel()

	var requestedN atomic.Int32
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, n int) ([]domain.JobRun, error) {
			requestedN.Store(int32(n))
			return nil, nil
		},
	}

	pool := NewPool(10)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	// Use 5 of 10 slots
	started := make(chan struct{}, 5)
	done := make(chan struct{})
	for range 5 {
		pool.Submit(context.Background(), func() {
			started <- struct{}{}
			<-done
		})
	}
	for range 5 {
		<-started
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:         pool,
		Queue:        q,
		Store:        &mockExecutorStore{},
		PollInterval: time.Hour,
	})

	exec.poll(context.Background())
	close(done)

	got := requestedN.Load()
	if got != 5 {
		t.Fatalf("expected dequeue with n=5 (idle workers), got n=%d", got)
	}
}

func TestAdaptiveDequeue_CapsAtMaxBatch(t *testing.T) {
	t.Parallel()

	var requestedN atomic.Int32
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, n int) ([]domain.JobRun, error) {
			requestedN.Store(int32(n))
			return nil, nil
		},
	}

	pool := NewPool(100)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:                pool,
		Queue:               q,
		Store:               &mockExecutorStore{},
		PollInterval:        time.Hour,
		MaxDequeueBatchSize: 10,
	})

	exec.poll(context.Background())

	got := requestedN.Load()
	if got != 10 {
		t.Fatalf("expected dequeue capped at maxBatchSize=10, got n=%d", got)
	}
}

func TestAdaptiveDequeue_SingleIdleWorker(t *testing.T) {
	t.Parallel()

	var requestedN atomic.Int32
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, n int) ([]domain.JobRun, error) {
			requestedN.Store(int32(n))
			return nil, nil
		},
	}

	pool := NewPool(2)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	// Occupy 1 of 2 slots
	started := make(chan struct{})
	done := make(chan struct{})
	pool.Submit(context.Background(), func() {
		close(started)
		<-done
	})
	<-started

	exec := NewExecutor(ExecutorConfig{
		Pool:                pool,
		Queue:               q,
		Store:               &mockExecutorStore{},
		PollInterval:        time.Hour,
		MaxDequeueBatchSize: 10,
	})

	exec.poll(context.Background())
	close(done)

	got := requestedN.Load()
	if got != 1 {
		t.Fatalf("expected dequeue with n=1 (single idle worker), got n=%d", got)
	}
}

func TestDispatch_RetryIncludesCheckpointHeaders(t *testing.T) {
	t.Parallel()

	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	cpTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}
	store.getLatestCheckpointFn = func(_ context.Context, _ string) (*domain.RunCheckpoint, error) {
		return &domain.RunCheckpoint{
			ID:        "cp-1",
			RunID:     "run-1",
			Sequence:  1,
			State:     json.RawMessage(`{"cursor":42}`),
			CreatedAt: cpTime,
		}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(2) // attempt > 1

	exec.execute(context.Background(), run)

	if headers.Get("X-Last-Checkpoint") != `{"cursor":42}` {
		t.Fatalf("X-Last-Checkpoint = %q, want %q", headers.Get("X-Last-Checkpoint"), `{"cursor":42}`)
	}
	if headers.Get("X-Checkpoint-At") != cpTime.Format(time.RFC3339) {
		t.Fatalf("X-Checkpoint-At = %q, want %q", headers.Get("X-Checkpoint-At"), cpTime.Format(time.RFC3339))
	}
}

func TestDispatch_FirstAttemptNoCheckpointHeaders(t *testing.T) {
	t.Parallel()

	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}
	store.getLatestCheckpointFn = func(_ context.Context, _ string) (*domain.RunCheckpoint, error) {
		t.Fatal("should not call GetLatestCheckpoint on first attempt")
		return nil, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1) // first attempt

	exec.execute(context.Background(), run)

	if headers.Get("X-Last-Checkpoint") != "" {
		t.Fatalf("expected no X-Last-Checkpoint on first attempt, got %q", headers.Get("X-Last-Checkpoint"))
	}
}

func TestDispatch_NoCheckpointGraceful(t *testing.T) {
	t.Parallel()

	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}
	store.getLatestCheckpointFn = func(_ context.Context, _ string) (*domain.RunCheckpoint, error) {
		return nil, nil // no checkpoint exists
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(2) // retry

	exec.execute(context.Background(), run)

	if headers.Get("X-Last-Checkpoint") != "" {
		t.Fatalf("expected no X-Last-Checkpoint when none exists, got %q", headers.Get("X-Last-Checkpoint"))
	}
	// Should still have completed successfully
	if run.Status != domain.StatusCompleted {
		t.Fatalf("run status = %s, want completed", run.Status)
	}
}

func TestDispatch_RetryIncludesPreviousError(t *testing.T) {
	t.Parallel()

	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(2)
	run.Error = "connection timeout"

	exec.execute(context.Background(), run)

	if headers.Get("X-Previous-Error") != "connection timeout" {
		t.Fatalf("X-Previous-Error = %q, want %q", headers.Get("X-Previous-Error"), "connection timeout")
	}
}

func TestDispatch_RetryNoPreviousError(t *testing.T) {
	t.Parallel()

	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(2)
	run.Error = ""

	exec.execute(context.Background(), run)

	if headers.Get("X-Previous-Error") != "" {
		t.Fatalf("expected no X-Previous-Error when empty, got %q", headers.Get("X-Previous-Error"))
	}
}

func TestHandleFailure_RetryBoostsPriority(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 3, 5)
	job.RetryPriorityBoost = 2
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.Priority = 3

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	// Find the retry transition (executing -> queued)
	var retryCall *statusUpdateCall
	for i, c := range calls {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			retryCall = &calls[i]
			break
		}
	}
	if retryCall == nil {
		t.Fatal("expected retry transition (executing -> queued)")
	}
	gotPriority, ok := retryCall.fields["priority"].(int)
	if !ok {
		t.Fatalf("expected priority field in retry, got %v", retryCall.fields["priority"])
	}
	if gotPriority != 5 {
		t.Fatalf("expected priority=5 (3+2), got %d", gotPriority)
	}
}

func TestHandleFailure_RetryPriorityCappedAt10(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 3, 5)
	job.RetryPriorityBoost = 3
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.Priority = 9

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	var retryCall *statusUpdateCall
	for i, c := range calls {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			retryCall = &calls[i]
			break
		}
	}
	if retryCall == nil {
		t.Fatal("expected retry transition")
	}
	gotPriority := retryCall.fields["priority"].(int)
	if gotPriority != 10 {
		t.Fatalf("expected priority capped at 10, got %d", gotPriority)
	}
}

func TestHandleFailure_ZeroBoostNoChange(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 3, 5)
	job.RetryPriorityBoost = 0
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.Priority = 3

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	var retryCall *statusUpdateCall
	for i, c := range calls {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			retryCall = &calls[i]
			break
		}
	}
	if retryCall == nil {
		t.Fatal("expected retry transition")
	}
	if _, ok := retryCall.fields["priority"]; ok {
		t.Fatal("expected no priority field when boost is 0")
	}
}

func TestHandleFailure_DefaultBoostIsOne(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 3, 5)
	job.RetryPriorityBoost = 1 // default from DB
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.Priority = 0

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	var retryCall *statusUpdateCall
	for i, c := range calls {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			retryCall = &calls[i]
			break
		}
	}
	if retryCall == nil {
		t.Fatal("expected retry transition")
	}
	gotPriority := retryCall.fields["priority"].(int)
	if gotPriority != 1 {
		t.Fatalf("expected priority=1 (0+1 default boost), got %d", gotPriority)
	}
}
