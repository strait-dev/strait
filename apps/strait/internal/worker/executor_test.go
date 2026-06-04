package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"

	"strait/internal/domain"
	"strait/internal/httputil"
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
	snoozeRunWithLockFn      func(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	updateHeartbeatFn        func(ctx context.Context, id string) error
	batchUpdateHeartbeatFn   func(ctx context.Context, ids []string) error
	scheduleRetryFn          func(ctx context.Context, runID string, at time.Time, attempt int) error
	clearRetryFn             func(ctx context.Context, runID string) error
	canDispatchFn            func(ctx context.Context, endpointURL string, now time.Time) (bool, *time.Time, error)
	recordFailureFn          func(ctx context.Context, endpointURL string, now time.Time, threshold int, openDuration time.Duration) error
	recordSuccessFn          func(ctx context.Context, endpointURL string) error
	getJobHealthStatsFn      func(ctx context.Context, jobID string, since time.Time) (*orcstore.JobHealthStats, error)
	getResolvedEnvVarsFn     func(ctx context.Context, id string) (map[string]string, error)
	getLatestCheckpointFn    func(ctx context.Context, runID string) (*domain.RunCheckpoint, error)
	getRunFn                 func(ctx context.Context, id string) (*domain.JobRun, error)
	getProjectQuotaFn        func(ctx context.Context, projectID string) (*orcstore.ProjectQuota, error)
	insertEventFn            func(ctx context.Context, event *domain.RunEvent) error

	mu                 sync.Mutex
	statusCalls        []statusUpdateCall
	heartbeatRunIDs    []string
	scheduleRetryCalls []scheduleRetryCall
	clearRetryCalls    []string
	healthResultKeys   []string
}

type scheduleRetryCall struct {
	runID   string
	at      time.Time
	attempt int
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

func (m *mockExecutorStore) SnoozeRunWithLock(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	m.mu.Lock()
	m.statusCalls = append(m.statusCalls, statusUpdateCall{
		id:     id,
		from:   from,
		to:     to,
		fields: maps.Clone(fields),
	})
	m.mu.Unlock()

	if m.snoozeRunWithLockFn != nil {
		return m.snoozeRunWithLockFn(ctx, id, from, to, fields)
	}
	// Default: delegate to UpdateRunStatus so existing tests that only stub
	// updateRunStatusFn keep observing snooze attempts.
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

func (m *mockExecutorStore) ScheduleRetry(ctx context.Context, runID string, at time.Time, attempt int) error {
	m.mu.Lock()
	m.scheduleRetryCalls = append(m.scheduleRetryCalls, scheduleRetryCall{runID: runID, at: at, attempt: attempt})
	m.mu.Unlock()
	if m.scheduleRetryFn != nil {
		return m.scheduleRetryFn(ctx, runID, at, attempt)
	}
	return nil
}

func (m *mockExecutorStore) ClearRetry(ctx context.Context, runID string) error {
	m.mu.Lock()
	m.clearRetryCalls = append(m.clearRetryCalls, runID)
	m.mu.Unlock()
	if m.clearRetryFn != nil {
		return m.clearRetryFn(ctx, runID)
	}
	return nil
}

func (m *mockExecutorStore) scheduleRetries() []scheduleRetryCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]scheduleRetryCall, len(m.scheduleRetryCalls))
	copy(out, m.scheduleRetryCalls)
	return out
}

func (m *mockExecutorStore) clearRetries() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.clearRetryCalls))
	copy(out, m.clearRetryCalls)
	return out
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

func (m *mockExecutorStore) GetRun(ctx context.Context, id string) (*domain.JobRun, error) {
	if m.getRunFn == nil {
		return nil, nil
	}
	return m.getRunFn(ctx, id)
}

func (m *mockExecutorStore) GetProjectQuota(ctx context.Context, projectID string) (*orcstore.ProjectQuota, error) {
	if m.getProjectQuotaFn == nil {
		return nil, nil
	}
	return m.getProjectQuotaFn(ctx, projectID)
}

func (m *mockExecutorStore) InsertEvent(ctx context.Context, event *domain.RunEvent) error {
	if m.insertEventFn == nil {
		return nil
	}
	return m.insertEventFn(ctx, event)
}

func (m *mockExecutorStore) GetEndpointHealthScore(_ context.Context, _ string) (*domain.EndpointHealthScore, error) {
	return nil, nil
}

func (m *mockExecutorStore) UpsertEndpointHealthScore(_ context.Context, _ *domain.EndpointHealthScore) error {
	return nil
}

func (m *mockExecutorStore) AtomicRecordHealthResult(
	_ context.Context,
	endpointURL string,
	_, _, _, _ float64,
	_, _, _ float64,
	_ float64,
) (*domain.EndpointHealthScore, error) {
	m.mu.Lock()
	m.healthResultKeys = append(m.healthResultKeys, endpointURL)
	m.mu.Unlock()

	return &domain.EndpointHealthScore{
		EndpointURL:  endpointURL,
		HealthScore:  100.0,
		SuccessRate:  1.0,
		LatencyScore: 1.0,
	}, nil
}

func (m *mockExecutorStore) healthResults() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.healthResultKeys))
	copy(out, m.healthResultKeys)
	return out
}

func (m *mockExecutorStore) CountExecutingRunsByOrg(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockExecutorStore) statusUpdates() []statusUpdateCall {
	m.mu.Lock()
	defer m.mu.Unlock()

	calls := make([]statusUpdateCall, len(m.statusCalls))
	copy(calls, m.statusCalls)
	return calls
}

type mockDegradedNotifier struct {
	ch <-chan struct{}
}

func (m *mockDegradedNotifier) Degraded() <-chan struct{} { return m.ch }

type mockExecQueue struct {
	enqueueFn           func(ctx context.Context, run *domain.JobRun) error
	enqueueExistingFn   func(ctx context.Context, run *domain.JobRun) error
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

func (m *mockExecQueue) EnqueueInTx(ctx context.Context, _ orcstore.DBTX, run *domain.JobRun) error {
	return m.Enqueue(ctx, run)
}

func (m *mockExecQueue) EnqueueExisting(ctx context.Context, run *domain.JobRun) error {
	if m.enqueueExistingFn == nil {
		return nil
	}
	return m.enqueueExistingFn(ctx, run)
}

func (m *mockExecQueue) EnqueueBatch(_ context.Context, runs []*domain.JobRun) (int64, error) {
	return int64(len(runs)), nil
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
		job := testJob(server.URL, 1, 5)
		job.EnvironmentID = "env-secret"
		return job, nil
	}
	store.listSecretsFn = func(_ context.Context, jobID, environment string) ([]domain.JobSecret, error) {
		if jobID != "job-1" || environment != "env-secret" {
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
	if _, ok := calls[0].fields["next_retry_at"]; ok {
		t.Fatalf("next_retry_at must not be in job_runs UPDATE fields; lives in job_retries side table now")
	}
	scheduled := store.scheduleRetries()
	if len(scheduled) != 1 {
		t.Fatalf("ScheduleRetry calls = %d, want 1", len(scheduled))
	}
	if !scheduled[0].at.Equal(retryAt) {
		t.Fatalf("ScheduleRetry at = %v, want %v", scheduled[0].at, retryAt)
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

func TestExecutor_CircuitBreakerFailureUsesProjectScopedEndpointKey(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("temporary failure"))
	}))
	defer server.Close()

	var precheckKey string
	var failureKey string
	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}
	store.canDispatchFn = func(_ context.Context, endpointURL string, _ time.Time) (bool, *time.Time, error) {
		precheckKey = endpointURL
		return true, nil, nil
	}
	store.recordFailureFn = func(_ context.Context, endpointURL string, _ time.Time, _ int, _ time.Duration) error {
		failureKey = endpointURL
		return nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	want := endpointStateKey("proj-1", server.URL)
	if precheckKey != want {
		t.Fatalf("precheck endpoint key = %q, want %q", precheckKey, want)
	}
	if failureKey != want {
		t.Fatalf("failure endpoint key = %q, want %q", failureKey, want)
	}
	healthKeys := store.healthResults()
	if len(healthKeys) == 0 {
		t.Fatal("expected health failure to be recorded")
	}
	if healthKeys[len(healthKeys)-1] != want {
		t.Fatalf("health endpoint key = %q, want %q", healthKeys[len(healthKeys)-1], want)
	}
}

func TestExecutor_TimeoutFailureUsesProjectScopedEndpointKey(t *testing.T) {
	t.Parallel()

	var failureKey string
	store := &mockExecutorStore{}
	store.recordFailureFn = func(_ context.Context, endpointURL string, _ time.Time, _ int, _ time.Duration) error {
		failureKey = endpointURL
		return nil
	}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	job := testJob("http://timeout.test", 3, 30)
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	exec.handleTimeout(context.Background(), run, job, policy, nil)

	want := endpointStateKey("proj-1", job.EndpointURL)
	if failureKey != want {
		t.Fatalf("timeout failure endpoint key = %q, want %q", failureKey, want)
	}
	healthKeys := store.healthResults()
	if len(healthKeys) == 0 {
		t.Fatal("expected timeout health result to be recorded")
	}
	if healthKeys[len(healthKeys)-1] != want {
		t.Fatalf("timeout health endpoint key = %q, want %q", healthKeys[len(healthKeys)-1], want)
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
	// Pre-fill one bulkhead slot so the job is at capacity (max_concurrency=1).
	exec.bulkhead.TryAcquire("job-1", 1)

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

	if count := exec.bulkhead.ActiveCount("job-1"); count != 0 {
		t.Fatalf("bulkhead active count = %d, want 0 (released)", count)
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

func TestExecutor_HandleFailure_DLQCapDropOldest(t *testing.T) {
	store := &mockExecutorStore{}
	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, http.DefaultClient)
	dlqStore := newFakeDLQStore()
	dlqStore.perJob["proj-1:job-1"] = 1
	dlqStore.perProject["proj-1"] = 1
	exec.dlqCapEnforcer = NewDLQCapEnforcer(dlqStore, DLQCapConfig{
		MaxPerJob: 1,
		Policy:    DLQOverflowDropOldest,
	}, nil)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	job := testJob("https://example.com", 1, 5)
	policy := executionPolicy{maxAttempts: 1}

	if ok := exec.handleFailure(context.Background(), run, job, policy, errors.New("terminal failure"), nil); !ok {
		t.Fatal("handleFailure returned false")
	}
	if len(dlqStore.masked) != 1 {
		t.Fatalf("masked rows = %d, want 1", len(dlqStore.masked))
	}
	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}
	if calls[0].to != domain.StatusDeadLetter {
		t.Fatalf("status = %s, want %s", calls[0].to, domain.StatusDeadLetter)
	}
}

func TestExecutor_HandleFailure_DLQCapRejectBecomesSystemFailed(t *testing.T) {
	store := &mockExecutorStore{}
	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, http.DefaultClient)
	dlqStore := newFakeDLQStore()
	dlqStore.perJob["proj-1:job-1"] = 1
	exec.dlqCapEnforcer = NewDLQCapEnforcer(dlqStore, DLQCapConfig{
		MaxPerJob: 1,
		Policy:    DLQOverflowReject,
	}, nil)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	job := testJob("https://example.com", 1, 5)
	policy := executionPolicy{maxAttempts: 1}

	if ok := exec.handleFailure(context.Background(), run, job, policy, errors.New("terminal failure"), nil); !ok {
		t.Fatal("handleFailure returned false")
	}
	if len(dlqStore.masked) != 0 {
		t.Fatalf("masked rows = %d, want 0", len(dlqStore.masked))
	}
	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}
	if calls[0].to != domain.StatusSystemFailed {
		t.Fatalf("status = %s, want %s", calls[0].to, domain.StatusSystemFailed)
	}
	if got := calls[0].fields["error_class"]; got != "dlq_overflow" {
		t.Fatalf("error_class = %v, want dlq_overflow", got)
	}
	errMsg, _ := calls[0].fields["error"].(string)
	if !strings.Contains(errMsg, "dlq overflow: cap reached") || !strings.Contains(errMsg, "terminal failure") {
		t.Fatalf("error = %q, want dlq overflow with original error", errMsg)
	}
	if run.Status != domain.StatusSystemFailed {
		t.Fatalf("run status = %s, want %s", run.Status, domain.StatusSystemFailed)
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
	var concWG conc.WaitGroup
	defer concWG.Wait()
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
	concWG.Go(func() {
		exec.Run(ctx)
		close(runDone)
	})

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
	concWG.Go(func() {
		_ = pool.Shutdown(context.Background())
		close(shutdownDone)
	})

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
	var concWG conc.WaitGroup
	defer concWG.Wait()
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
	concWG.Go(func() {
		exec.Run(ctx)
		close(done)
	})

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

func TestExecutor_Run_DegradedModeShortensPollInterval(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	wake := make(chan struct{}, 1)
	degradedCh := make(chan struct{})
	pollCount := make(chan struct{}, 100)

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			select {
			case pollCount <- struct{}{}:
			default:
			}
			return nil, nil
		},
	}

	pool := NewPool(1)
	defer func() { _ = pool.Shutdown(context.Background()) }()

	exec := NewExecutor(ExecutorConfig{
		Pool:                 pool,
		Queue:                q,
		Wake:                 wake,
		Degraded:             &mockDegradedNotifier{ch: degradedCh},
		DegradedPollInterval: 50 * time.Millisecond,
		Store:                &mockExecutorStore{},
		PollInterval:         time.Hour,
		HeartbeatInterval:    time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		exec.Run(ctx)
		close(done)
	})

	// Close the degraded channel to simulate notifier entering degraded mode.
	close(degradedCh)

	// With a 50ms degraded poll interval, we should see multiple polls quickly.
	deadline := time.After(2 * time.Second)
	polls := 0
	for polls < 3 {
		select {
		case <-pollCount:
			polls++
		case <-deadline:
			t.Fatalf("expected at least 3 degraded polls, got %d", polls)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("executor did not stop after context cancel")
	}
}

type rearmDegradedNotifier struct {
	mu    sync.Mutex
	calls int
	chs   []<-chan struct{}
}

func (r *rearmDegradedNotifier) Degraded() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	idx := r.calls
	r.calls++
	if idx < len(r.chs) {
		return r.chs[idx]
	}
	return make(chan struct{})
}

func TestExecutor_DegradedRecoveryDoesNotReenterOnStaleChannel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	wake := make(chan struct{}, 1)
	pollCount := atomic.Int64{}

	closedCh := make(chan struct{})
	close(closedCh)
	openCh := make(chan struct{})

	notifier := &rearmDegradedNotifier{
		chs: []<-chan struct{}{closedCh, openCh},
	}

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			pollCount.Add(1)
			return nil, nil
		},
	}

	pool := NewPool(1)
	defer func() { _ = pool.Shutdown(context.Background()) }()

	exec := NewExecutor(ExecutorConfig{
		Pool:                 pool,
		Queue:                q,
		Wake:                 wake,
		Degraded:             notifier,
		DegradedPollInterval: 50 * time.Millisecond,
		Store:                &mockExecutorStore{},
		PollInterval:         time.Hour,
		HeartbeatInterval:    time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		exec.Run(ctx)
		close(done)
	})

	// The first Degraded() call returns closedCh, so the executor should
	// enter degraded mode and start fast polling.
	time.Sleep(200 * time.Millisecond)

	// Simulate reconnect: send a wake event.
	wake <- struct{}{}
	time.Sleep(100 * time.Millisecond)

	// After recovery, the executor re-armed with openCh (the second call
	// to Degraded()). Since openCh is never closed, the executor should NOT
	// re-enter degraded mode. Record the poll count and wait.
	baseline := pollCount.Load()
	time.Sleep(300 * time.Millisecond)
	final := pollCount.Load()

	// Under normal poll interval (1 hour), no additional polls should fire.
	// Allow 1 extra poll from timing slack.
	if final-baseline > 2 {
		t.Errorf("executor re-entered degraded fast-poll: %d polls after recovery, want <= 2", final-baseline)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("executor did not stop after context cancel")
	}
}

func TestExecutor_Shutdown_NoInFlight(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	exec := newTestExecutor(t, &mockExecutorStore{}, &mockExecQueue{}, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	runDone := make(chan struct{})
	concWG.Go(func() {
		exec.Run(ctx)
		close(runDone)
	})

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
	var concWG conc.WaitGroup
	defer concWG.Wait()
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
	concWG.Go(func() {
		exec.Run(runCtx)
		close(runDone)
	})

	wake <- struct{}{}
	waitForSignal(t, pollStarted, "poll did not start")

	shutdownDone := make(chan error, 1)
	concWG.Go(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		shutdownDone <- exec.Shutdown(shutdownCtx)
	})

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
	var concWG conc.WaitGroup
	defer concWG.Wait()
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
	concWG.Go(func() {
		exec.Run(runCtx)
		close(runDone)
	})

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
	var concWG conc.WaitGroup
	defer concWG.Wait()
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
	concWG.Go(func() {
		hb.Run(ctx, "run-1")
		close(done)
	})

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

	result := sendWebhookOnceWith(t.Context(), webhookClient, job, run)
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

	result := sendWebhookOnceWith(t.Context(), webhookClient, job, run)
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

	result := sendWebhookOnceWith(t.Context(), webhookClient, job, run)
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

	result := sendWebhookOnceWith(t.Context(), webhookClient, job, run)
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

	sendWebhookOnceWith(t.Context(), webhookClient, job, run)

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

	result := sendWebhookOnceWith(t.Context(), webhookClient, job, run)
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
	var concWG conc.WaitGroup
	defer concWG.Wait()
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
	concWG.Go(func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if attempts.Load() >= 1 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		cancel()
	})

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
	exec.logger = slog.New(slog.DiscardHandler)
	defer func() { _ = exec.pool.Shutdown(context.Background()) }()

	b.ResetTimer()
	for range b.N {
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
	exec.executionTraceMode = executionTraceFull
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
	exec.executionTraceMode = executionTraceFull
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
	exec.executionTraceMode = executionTraceFull
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

func TestExecutor_EnvironmentOverride_WithSecretsUsesOriginalEndpoint(t *testing.T) {
	t.Parallel()

	var overrideCalled atomic.Bool
	overrideServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		overrideCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer overrideServer.Close()

	var originalSecretHeader atomic.Value
	originalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originalSecretHeader.Store(r.Header.Get("X-Secret-API_KEY"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"from":"original"}`))
	}))
	defer originalServer.Close()

	overrideParsed, err := url.Parse(overrideServer.URL)
	if err != nil {
		t.Fatalf("parse override server url: %v", err)
	}
	overrideURL := "http://example.com" + ":" + overrideParsed.Port()

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
	store.listSecretsFn = func(_ context.Context, jobID, environment string) ([]domain.JobSecret, error) {
		if jobID != "job-1" || environment != "env-1" {
			t.Fatalf("unexpected secret scope: job_id=%q environment=%q", jobID, environment)
		}
		return []domain.JobSecret{{SecretKey: "API_KEY", EncryptedValue: "super-secret"}}, nil
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
	if overrideCalled.Load() {
		t.Fatal("override endpoint received dispatch despite job secrets")
	}
	if got, _ := originalSecretHeader.Load().(string); got != "super-secret" {
		t.Fatalf("original endpoint secret header = %q, want super-secret", got)
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

// TestTracedDispatch_RetryEmitsCheckpointHeadersWhenSecretsCacheWarm pins the
// fix for a durable-resume regression: when the dispatch secrets cache is
// pre-populated (as it is on attempt 1 when an ENDPOINT_URL environment
// override resolves dispatch secrets to the empty set), attempt 2 used to
// hit the cached secrets and never load the checkpoint, dropping the
// X-Last-Checkpoint and X-Checkpoint-At resume headers the endpoint needs
// to skip already-completed steps.
func TestTracedDispatch_RetryEmitsCheckpointHeadersWhenSecretsCacheWarm(t *testing.T) {
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

	// Simulate the ENDPOINT_URL override path: secrets are resolved once on
	// attempt 1 to an empty slice and cached. Attempt 2 sees the warm cache.
	ctx := withDispatchCache(context.Background())
	job := testJob(server.URL, 3, 5)
	dispatchCacheSet(ctx, dispatchSecretsCacheKey(job), []domain.JobSecret{})

	if _, _, err := exec.tracedDispatch(ctx, job, testRun(2)); err != nil {
		t.Fatalf("tracedDispatch: %v", err)
	}

	if got := headers.Get("X-Last-Checkpoint"); got != `{"cursor":42}` {
		t.Fatalf("X-Last-Checkpoint = %q, want %q (the warm secrets cache must not suppress the checkpoint load)", got, `{"cursor":42}`)
	}
	if got := headers.Get("X-Checkpoint-At"); got != cpTime.Format(time.RFC3339) {
		t.Fatalf("X-Checkpoint-At = %q, want %q", got, cpTime.Format(time.RFC3339))
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
		return
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
		return
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
		return
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
		return
	}
	gotPriority := retryCall.fields["priority"].(int)
	if gotPriority != 1 {
		t.Fatalf("expected priority=1 (0+1 default boost), got %d", gotPriority)
	}
}

func TestHandleFailure_BoostFromMaxPriority(t *testing.T) {
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
	run.Priority = 10

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
		t.Fatal("expected retry transition (executing -> queued)")
		return
	}
	gotPriority := retryCall.fields["priority"].(int)
	if gotPriority != 10 {
		t.Fatalf("expected priority=10 (already at max), got %d", gotPriority)
	}
}

func TestHandleFailure_BoostExactlyToMax(t *testing.T) {
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
	run.Priority = 8

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
		return
	}
	gotPriority := retryCall.fields["priority"].(int)
	if gotPriority != 10 {
		t.Fatalf("expected priority=10 (8+2 exactly at max), got %d", gotPriority)
	}
}

func TestHandleFailure_LargeBoostValue(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 3, 5)
	job.RetryPriorityBoost = 10
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
		return
	}
	gotPriority := retryCall.fields["priority"].(int)
	if gotPriority != 10 {
		t.Fatalf("expected priority=10 (0+10 capped at max), got %d", gotPriority)
	}
}

func TestHandleFailure_BoostOnHighAttempt(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 6, 5)
	job.RetryPriorityBoost = 1
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(4) // high attempt, still retryable
	run.Priority = 2

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
		t.Fatal("expected retry transition on high attempt")
		return
	}
	gotPriority := retryCall.fields["priority"].(int)
	if gotPriority != 3 {
		t.Fatalf("expected priority=3 (2+1), got %d", gotPriority)
	}
}

func TestHandleFailure_BoostNotAppliedWhenPoisonPill(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})
	threshold := 3
	errBody := "fail"
	endpointErr := &domain.EndpointError{StatusCode: 500, Body: errBody}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 3, Priority: 3, Metadata: map[string]string{
		"_error_hash":       errorHashForError(endpointErr),
		"_error_hash_count": "2",
	}}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 2, PoisonPillThreshold: &threshold}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, endpointErr, nil)

	calls := store.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected at least one status update")
	}
	last := calls[len(calls)-1]
	if last.to != domain.StatusDeadLetter {
		t.Errorf("expected dead_letter due to poison pill, got %s", last.to)
	}
	if _, ok := last.fields["priority"]; ok {
		t.Error("expected no priority field when poison pill triggers")
	}
}

func TestHandleFailure_BoostAppliedWhenPoisonPillNotTriggered(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})
	// No PoisonPillThreshold set, so poison pill is disabled.
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 3, Priority: 3}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 2}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

	calls := store.statusUpdates()
	var retryCall *statusUpdateCall
	for i, c := range calls {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			retryCall = &calls[i]
			break
		}
	}
	if retryCall == nil {
		t.Fatal("expected retry transition when poison pill doesn't trigger")
		return
	}
	gotPriority, ok := retryCall.fields["priority"].(int)
	if !ok {
		t.Fatal("expected priority field in retry")
	}
	if gotPriority != 5 {
		t.Fatalf("expected priority=5 (3+2), got %d", gotPriority)
	}
}

func TestHandleFailure_BoostNotAppliedOnLastAttempt(t *testing.T) {
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
	run := testRun(3) // last attempt
	run.Priority = 3

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	// Should go to dead_letter, not queued
	for _, c := range calls {
		if c.to == domain.StatusQueued {
			t.Fatal("should not retry on last attempt")
		}
	}
	foundDL := false
	for _, c := range calls {
		if c.to == domain.StatusDeadLetter {
			foundDL = true
			break
		}
	}
	if !foundDL {
		t.Fatal("expected dead_letter on last attempt")
	}
}

func TestHandleFailure_BoostWithNonRetryableError(t *testing.T) {
	t.Parallel()

	// 400 status code -> client error class -> non-retryable
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`bad request`))
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
	for _, c := range calls {
		if c.to == domain.StatusQueued {
			t.Fatal("should not retry on non-retryable client error")
		}
	}
	foundDL := false
	for _, c := range calls {
		if c.to == domain.StatusDeadLetter {
			foundDL = true
			break
		}
	}
	if !foundDL {
		t.Fatal("expected dead_letter for non-retryable error")
	}
}

// handleTimeout boost tests.

func TestHandleTimeout_RetryBoostsPriority(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	run.Priority = 3
	job := testJob("http://localhost", 3, 30)
	job.RetryPriorityBoost = 2
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	exec.handleTimeout(context.Background(), run, job, policy, nil)

	calls := store.statusUpdates()
	var retryCall *statusUpdateCall
	for i, c := range calls {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			retryCall = &calls[i]
			break
		}
	}
	if retryCall == nil {
		t.Fatal("expected retry transition (executing -> queued)")
		return
	}
	gotPriority, ok := retryCall.fields["priority"].(int)
	if !ok {
		t.Fatalf("expected priority field in timeout retry, got %v", retryCall.fields["priority"])
	}
	if gotPriority != 5 {
		t.Fatalf("expected priority=5 (3+2), got %d", gotPriority)
	}
}

func TestHandleTimeout_RetryPriorityCappedAt10(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	run.Priority = 9
	job := testJob("http://localhost", 3, 30)
	job.RetryPriorityBoost = 3
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	exec.handleTimeout(context.Background(), run, job, policy, nil)

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
		return
	}
	gotPriority := retryCall.fields["priority"].(int)
	if gotPriority != 10 {
		t.Fatalf("expected priority capped at 10, got %d", gotPriority)
	}
}

func TestHandleTimeout_ZeroBoostNoChange(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	run.Priority = 3
	job := testJob("http://localhost", 3, 30)
	job.RetryPriorityBoost = 0
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	exec.handleTimeout(context.Background(), run, job, policy, nil)

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

func TestHandleTimeout_BoostFromMaxPriority(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	run.Priority = 10
	job := testJob("http://localhost", 3, 30)
	job.RetryPriorityBoost = 1
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	exec.handleTimeout(context.Background(), run, job, policy, nil)

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
		t.Fatalf("expected priority=10 (already at max), got %d", gotPriority)
	}
}

func TestHandleTimeout_BoostNotAppliedOnLastAttempt(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(3) // last attempt
	run.Status = domain.StatusExecuting
	run.Priority = 3
	job := testJob("http://localhost", 3, 30)
	job.RetryPriorityBoost = 2
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	exec.handleTimeout(context.Background(), run, job, policy, nil)

	calls := store.statusUpdates()
	for _, c := range calls {
		if c.to == domain.StatusQueued {
			t.Fatal("should not retry on last attempt")
		}
	}
	foundTimeout := false
	for _, c := range calls {
		if c.to == domain.StatusTimedOut {
			foundTimeout = true
			break
		}
	}
	if !foundTimeout {
		t.Fatal("expected timed_out status on last attempt")
	}
}

// Cumulative boost simulation tests.

func TestHandleFailure_CumulativeBoostAcrossRetries(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 2, MaxAttempts: 6}
	policy := executionPolicy{maxAttempts: 6, timeoutSecs: 30}
	expectedPriorities := []int{2, 4, 6, 8, 10}

	priority := 0
	for i, expected := range expectedPriorities {
		store.mu.Lock()
		store.statusCalls = nil
		store.mu.Unlock()

		run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: i + 1, Priority: priority}
		exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

		calls := store.statusUpdates()
		var retryCall *statusUpdateCall
		for j, c := range calls {
			if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
				retryCall = &calls[j]
				break
			}
		}
		if retryCall == nil {
			t.Fatalf("attempt %d: expected retry transition", i+1)
		}
		gotPriority := retryCall.fields["priority"].(int)
		if gotPriority != expected {
			t.Fatalf("attempt %d: expected priority=%d, got %d", i+1, expected, gotPriority)
		}
		priority = gotPriority
	}
}

func TestHandleFailure_CumulativeBoostWithBoostOne(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 1, MaxAttempts: 5}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}
	expectedPriorities := []int{1, 2, 3}

	priority := 0
	for i, expected := range expectedPriorities {
		store.mu.Lock()
		store.statusCalls = nil
		store.mu.Unlock()

		run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: i + 1, Priority: priority}
		exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

		calls := store.statusUpdates()
		var retryCall *statusUpdateCall
		for j, c := range calls {
			if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
				retryCall = &calls[j]
				break
			}
		}
		if retryCall == nil {
			t.Fatalf("attempt %d: expected retry transition", i+1)
		}
		gotPriority := retryCall.fields["priority"].(int)
		if gotPriority != expected {
			t.Fatalf("attempt %d: expected priority=%d, got %d", i+1, expected, gotPriority)
		}
		priority = gotPriority
	}
}

func TestHandleTimeout_CumulativeBoostAcrossRetries(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 3, MaxAttempts: 5}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}
	expectedPriorities := []int{3, 6, 9, 10}

	priority := 0
	for i, expected := range expectedPriorities {
		store.mu.Lock()
		store.statusCalls = nil
		store.mu.Unlock()

		run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: i + 1, Priority: priority, Status: domain.StatusExecuting}
		exec.handleTimeout(context.Background(), run, job, policy, nil)

		calls := store.statusUpdates()
		var retryCall *statusUpdateCall
		for j, c := range calls {
			if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
				retryCall = &calls[j]
				break
			}
		}
		if retryCall == nil {
			t.Fatalf("attempt %d: expected retry transition", i+1)
		}
		gotPriority := retryCall.fields["priority"].(int)
		if gotPriority != expected {
			t.Fatalf("attempt %d: expected priority=%d, got %d", i+1, expected, gotPriority)
		}
		priority = gotPriority
	}
}

// Adversarial and edge case tests.

func TestHandleFailure_BoostWithMaxIntPriority(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1, Priority: math.MaxInt}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 1, MaxAttempts: 3}
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

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
		t.Fatalf("expected priority capped at 10 even with MaxInt input, got %d", gotPriority)
	}
}

func TestHandleFailure_BoostDoesNotMutateOriginalRun(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1, Priority: 3}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 5, MaxAttempts: 3}
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

	// The in-memory run struct should NOT be mutated
	if run.Priority != 3 {
		t.Fatalf("expected run.Priority to remain 3 (not mutated), got %d", run.Priority)
	}
}

func TestHandleTimeout_BoostFieldsMapIsolation(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 2, MaxAttempts: 5}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}

	// Two consecutive timeout retries should each have their own fields map
	run1 := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1, Priority: 0, Status: domain.StatusExecuting}
	exec.handleTimeout(context.Background(), run1, job, policy, nil)

	run2 := &domain.JobRun{ID: "run-2", JobID: "job-1", Attempt: 1, Priority: 5, Status: domain.StatusExecuting}
	exec.handleTimeout(context.Background(), run2, job, policy, nil)

	calls := store.statusUpdates()
	var priorities []int
	for _, c := range calls {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			if p, ok := c.fields["priority"].(int); ok {
				priorities = append(priorities, p)
			}
		}
	}
	if len(priorities) != 2 {
		t.Fatalf("expected 2 retry calls with priority, got %d", len(priorities))
	}
	if priorities[0] != 2 {
		t.Fatalf("first retry: expected priority=2 (0+2), got %d", priorities[0])
	}
	if priorities[1] != 7 {
		t.Fatalf("second retry: expected priority=7 (5+2), got %d", priorities[1])
	}
}

// boostPriority unit tests.

func TestBoostPriority_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		current  int
		boost    int
		expected int
	}{
		{"zero_plus_one", 0, 1, 1},
		{"three_plus_two", 3, 2, 5},
		{"eight_plus_two_exact_max", 8, 2, 10},
		{"nine_plus_three_capped", 9, 3, 10},
		{"ten_plus_one_capped", 10, 1, 10},
		{"ten_plus_ten_capped", 10, 10, 10},
		{"zero_plus_ten_max", 0, 10, 10},
		{"five_plus_five_exact_max", 5, 5, 10},
		{"maxint_plus_one_overflow", math.MaxInt, 1, 10},
		{"maxint_plus_maxint_overflow", math.MaxInt, math.MaxInt, 10},
		{"large_current_plus_large_boost", 1000000, 1000000, 10},
		{"negative_current_plus_boost", -5, 3, -2},
		{"negative_current_large_boost", -5, 20, 10},
		{"zero_plus_zero", 0, 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := boostPriority(tc.current, tc.boost)
			if got != tc.expected {
				t.Fatalf("boostPriority(%d, %d) = %d, want %d", tc.current, tc.boost, got, tc.expected)
			}
		})
	}
}

func TestHandleFailure_NegativePriorityWithBoost(t *testing.T) {
	t.Parallel()

	// If a run somehow has a negative priority, boost should still work correctly.
	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1, Priority: -5}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 3, MaxAttempts: 3}
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

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
	// -5 + 3 = -2, which is < 10 so min returns -2.
	if gotPriority != -2 {
		t.Fatalf("expected priority=-2 (-5+3), got %d", gotPriority)
	}
}

func TestHandleFailure_BoostWithStoreError(t *testing.T) {
	t.Parallel()

	// Verify that a store error during retry doesn't panic or corrupt state.
	store := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return errors.New("database connection lost")
		},
	}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1, Priority: 3}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 2, MaxAttempts: 3}
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	// Should not panic even when store fails.
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

	// Verify the original run struct is not mutated despite error.
	if run.Priority != 3 {
		t.Fatalf("expected run.Priority to remain 3 after store error, got %d", run.Priority)
	}
}

func TestHandleTimeout_BoostWithStoreError(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return errors.New("database connection lost")
		},
	}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1, Priority: 3, Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 2, MaxAttempts: 3}
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	// Should not panic.
	exec.handleTimeout(context.Background(), run, job, policy, nil)

	if run.Priority != 3 {
		t.Fatalf("expected run.Priority to remain 3 after store error, got %d", run.Priority)
	}
}

func TestHandleFailure_BoostConsistencyBetweenFailureAndTimeout(t *testing.T) {
	t.Parallel()

	// Verify that the same inputs produce the same priority boost
	// whether the retry comes from failure or timeout.
	failureStore := &mockExecutorStore{}
	timeoutStore := &mockExecutorStore{}

	failureExec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: failureStore, PollInterval: time.Hour,
	})
	timeoutExec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: timeoutStore, PollInterval: time.Hour,
	})

	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 3, MaxAttempts: 5}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}

	failureRun := &domain.JobRun{ID: "run-f", JobID: "job-1", Attempt: 2, Priority: 4}
	timeoutRun := &domain.JobRun{ID: "run-t", JobID: "job-1", Attempt: 2, Priority: 4, Status: domain.StatusExecuting}

	failureExec.handleFailure(context.Background(), failureRun, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)
	timeoutExec.handleTimeout(context.Background(), timeoutRun, job, policy, nil)

	var failurePriority, timeoutPriority int
	for _, c := range failureStore.statusUpdates() {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			failurePriority = c.fields["priority"].(int)
			break
		}
	}
	for _, c := range timeoutStore.statusUpdates() {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			timeoutPriority = c.fields["priority"].(int)
			break
		}
	}

	if failurePriority != timeoutPriority {
		t.Fatalf("failure and timeout produced different priorities: failure=%d, timeout=%d", failurePriority, timeoutPriority)
	}
	if failurePriority != 7 {
		t.Fatalf("expected priority=7 (4+3), got %d", failurePriority)
	}
}

func TestHandleFailure_RapidSequentialRetriesNoDataRace(t *testing.T) {
	t.Parallel()

	// Run many retries concurrently to check for data races.
	// This test is meaningful when run with -race flag.
	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 2, MaxAttempts: 10}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}

	var wg conc.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			run := &domain.JobRun{
				ID: fmt.Sprintf("run-%d", i), JobID: "job-1",
				Attempt: 1, Priority: i % 10,
			}
			exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)
		})
	}
	wg.Wait()

	calls := store.statusUpdates()
	retryCount := 0
	for _, c := range calls {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			retryCount++
			priority := c.fields["priority"].(int)
			if priority > 10 {
				t.Fatalf("priority %d exceeds cap of 10", priority)
			}
		}
	}
	if retryCount != 20 {
		t.Fatalf("expected 20 retry transitions, got %d", retryCount)
	}
}

func TestShutdown_WaitsForCallbacks(t *testing.T) {
	t.Parallel()

	var callbackCalled atomic.Bool
	callback := &mockWorkflowCallback{
		onTerminalFn: func(_ context.Context, _ *domain.JobRun) error {
			time.Sleep(100 * time.Millisecond)
			callbackCalled.Store(true)
			return nil
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}

	var dequeued atomic.Bool
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			if dequeued.CompareAndSwap(false, true) {
				return []domain.JobRun{*testRun(1)}, nil
			}
			return nil, nil
		},
	}

	pool := NewPool(4)
	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             store,
		PollInterval:      50 * time.Millisecond,
		HeartbeatInterval: time.Hour,
		WorkflowCallback:  callback,
		HTTPClient:        server.Client(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	go exec.Run(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if callbackCalled.Load() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := exec.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	_ = pool.Shutdown(context.Background())

	if !callbackCalled.Load() {
		t.Fatal("expected callback to complete before shutdown returned")
	}
}

func TestShutdown_NoCallbacksNoDelay(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	q := &mockExecQueue{}

	pool := NewPool(4)
	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             store,
		PollInterval:      time.Hour,
		HeartbeatInterval: time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go exec.Run(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()

	start := time.Now()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := exec.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	_ = pool.Shutdown(context.Background())

	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("shutdown took %v, expected near-instant with no callbacks", elapsed)
	}
}

func TestBulkhead_DefaultAppliedWhenJobHasNoLimit(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(ExecutorConfig{
		Pool:                     NewPool(10),
		Queue:                    &mockExecQueue{},
		Store:                    &mockExecutorStore{},
		PollInterval:             time.Hour,
		DefaultJobMaxConcurrency: 3,
	})

	// Should acquire first 3 slots
	for i := range 3 {
		if !exec.tryAcquireBulkheadSlot("job-1", 0) {
			t.Fatalf("slot %d should be acquired", i+1)
		}
	}
	// 4th should be rejected
	if exec.tryAcquireBulkheadSlot("job-1", 0) {
		t.Fatal("4th slot should be rejected with default concurrency 3")
	}
}

func TestBulkhead_ExplicitOverridesDefault(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(ExecutorConfig{
		Pool:                     NewPool(10),
		Queue:                    &mockExecQueue{},
		Store:                    &mockExecutorStore{},
		PollInterval:             time.Hour,
		DefaultJobMaxConcurrency: 3,
	})

	// Explicit limit of 5 should override default 3
	for i := range 5 {
		if !exec.tryAcquireBulkheadSlot("job-1", 5) {
			t.Fatalf("slot %d should be acquired with explicit limit 5", i+1)
		}
	}
	if exec.tryAcquireBulkheadSlot("job-1", 5) {
		t.Fatal("6th slot should be rejected with explicit limit 5")
	}
}

func TestBulkhead_DefaultZeroDisabled(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(ExecutorConfig{
		Pool:                     NewPool(10),
		Queue:                    &mockExecQueue{},
		Store:                    &mockExecutorStore{},
		PollInterval:             time.Hour,
		DefaultJobMaxConcurrency: 0, // disabled
	})

	// All slots should be acquired (no limit)
	for i := range 100 {
		if !exec.tryAcquireBulkheadSlot("job-1", 0) {
			t.Fatalf("slot %d should be acquired with no limit", i+1)
		}
	}
}

func TestPoll_MemoryPressure_SkipsDequeue(t *testing.T) {
	t.Parallel()

	var dequeueCalled atomic.Bool
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			dequeueCalled.Store(true)
			return nil, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:                       NewPool(10),
		Queue:                      q,
		Store:                      &mockExecutorStore{},
		PollInterval:               time.Hour,
		MemoryPressureThresholdPct: 1, // 1% — will always be exceeded
	})

	exec.poll(context.Background())

	if dequeueCalled.Load() {
		t.Fatal("dequeue should not be called when memory pressure exceeds threshold")
	}
}

func TestPoll_MemoryPressure_DisabledByDefault(t *testing.T) {
	t.Parallel()

	var dequeueCalled atomic.Bool
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			dequeueCalled.Store(true)
			return nil, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:                       NewPool(10),
		Queue:                      q,
		Store:                      &mockExecutorStore{},
		PollInterval:               time.Hour,
		MemoryPressureThresholdPct: 0, // disabled
	})

	exec.poll(context.Background())

	if !dequeueCalled.Load() {
		t.Fatal("dequeue should be called when memory pressure is disabled (threshold=0)")
	}
}

func TestHandleFailure_PoisonPillDetected(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})
	threshold := 3
	errBody := "fail"
	endpointErr := &domain.EndpointError{StatusCode: 500, Body: errBody}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 3, Metadata: map[string]string{
		"_error_hash":       errorHashForError(endpointErr),
		"_error_hash_count": "2",
	}}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: &threshold}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, endpointErr, nil)

	calls := store.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected at least one status update")
	}
	last := calls[len(calls)-1]
	if last.to != domain.StatusDeadLetter {
		t.Errorf("expected dead_letter, got %s", last.to)
	}
}

func TestHandleFailure_PoisonPillNotTriggeredOnDifferentError(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})
	threshold := 3
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 3, Metadata: map[string]string{
		"_error_hash":       errorHash("different error"),
		"_error_hash_count": "2",
	}}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: &threshold}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

	calls := store.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected at least one status update")
	}
	last := calls[len(calls)-1]
	if last.to != domain.StatusQueued {
		t.Errorf("expected queued (retry), got %s", last.to)
	}
}

func TestHandleFailure_PoisonPillNotTriggeredWhenDisabled(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})
	errMsg := "fail"
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 3, Metadata: map[string]string{
		"_error_hash":       errorHash(errMsg),
		"_error_hash_count": "2",
	}}
	// nil threshold = disabled
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com"}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: errMsg}, nil)

	calls := store.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected at least one status update")
	}
	last := calls[len(calls)-1]
	if last.to != domain.StatusQueued {
		t.Errorf("expected queued (retry), got %s", last.to)
	}
}

func TestResolveJob_CacheHit(t *testing.T) {
	t.Parallel()

	var getJobCalls atomic.Int32
	store := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			getJobCalls.Add(1)
			return &domain.Job{ID: "job-1", Version: 1, EndpointURL: "http://example.com"}, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
		JobCacheTTL:  5 * time.Minute,
	})
	t.Cleanup(exec.CloseCache)

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	// First call — cache miss, hits store
	job1, err := exec.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	if job1 == nil {
		t.Fatal("expected job, got nil")
	}

	// Second call — cache hit, should not hit store again
	job2, err := exec.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if job2 == nil {
		t.Fatal("expected job, got nil")
	}

	if getJobCalls.Load() != 1 {
		t.Errorf("expected 1 GetJob call (cache hit), got %d", getJobCalls.Load())
	}
}

func TestDeepSecResolveJob_ClonesCachedJobBeforeEnvironmentOverrideMutation(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        &mockExecutorStore{},
		PollInterval: time.Hour,
		JobCacheTTL:  5 * time.Minute,
	})
	t.Cleanup(exec.CloseCache)

	cached := &domain.Job{ID: "job-1", ProjectID: "proj-1", Version: 1, EndpointURL: "https://original.example/run"}
	if err := exec.jobCache.Set(context.Background(), "job-1", cached); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	resolved, err := exec.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("resolveJobForRun: %v", err)
	}
	resolved.EndpointURL = "https://override.example/run"

	again, err := exec.jobCache.Get(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	if again.EndpointURL != "https://original.example/run" {
		t.Fatalf("cached endpoint mutated to %q", again.EndpointURL)
	}
}

func TestDeepSecResolveJob_RefreshesLatestPolicyCacheHit(t *testing.T) {
	t.Parallel()

	var getJobCalls atomic.Int32
	store := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			getJobCalls.Add(1)
			return &domain.Job{
				ID:            "job-1",
				ProjectID:     "proj-1",
				Version:       2,
				VersionID:     "v2",
				VersionPolicy: domain.VersionPolicyLatest,
				EndpointURL:   "https://fresh.example/run",
			}, nil
		},
	}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
		JobCacheTTL:  5 * time.Minute,
	})
	t.Cleanup(exec.CloseCache)
	if err := exec.jobCache.Set(context.Background(), "job-1", &domain.Job{
		ID:            "job-1",
		ProjectID:     "proj-1",
		Version:       1,
		VersionPolicy: domain.VersionPolicyLatest,
		EndpointURL:   "https://stale.example/run",
	}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}
	resolved, err := exec.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("resolveJobForRun: %v", err)
	}
	if getJobCalls.Load() != 1 {
		t.Fatalf("GetJob calls = %d, want 1", getJobCalls.Load())
	}
	if resolved.Version != 2 || resolved.EndpointURL != "https://fresh.example/run" {
		t.Fatalf("resolved stale job: version=%d endpoint=%q", resolved.Version, resolved.EndpointURL)
	}
}

func TestDeepSecEndpointStateKeyScopesByProject(t *testing.T) {
	t.Parallel()

	endpoint := "https://shared.example/run"
	a := endpointStateKey("proj-a", endpoint)
	b := endpointStateKey("proj-b", endpoint)
	if a == b {
		t.Fatal("endpoint state keys for different projects must differ")
	}
	if strings.Contains(a, "\x00") || strings.Contains(b, "\x00") {
		t.Fatalf("endpoint state keys must be valid Postgres text: %q %q", a, b)
	}
	if strings.Contains(a, endpoint) || strings.Contains(b, endpoint) {
		t.Fatalf("project-scoped endpoint state keys must not store raw endpoint URL: %q %q", a, b)
	}
	if endpointStateKey("", endpoint) != endpoint {
		t.Fatal("empty project should preserve legacy endpoint key")
	}
}

func TestResolveJob_CacheExpiry(t *testing.T) {
	t.Parallel()

	var getJobCalls atomic.Int32
	store := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			getJobCalls.Add(1)
			return &domain.Job{ID: "job-1", Version: 1, EndpointURL: "http://example.com"}, nil
		},
	}

	// Otter uses a timer wheel with ~1s granularity for expiration.
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
		JobCacheTTL:  1 * time.Second,
	})
	t.Cleanup(exec.CloseCache)

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	_, _ = exec.resolveJobForRun(context.Background(), run)
	time.Sleep(3 * time.Second) // wait for expiry
	_, _ = exec.resolveJobForRun(context.Background(), run)

	if getJobCalls.Load() != 2 {
		t.Errorf("expected 2 GetJob calls after expiry, got %d", getJobCalls.Load())
	}
}

func TestResolveJob_CacheDisabledWhenTTLZero(t *testing.T) {
	t.Parallel()

	var getJobCalls atomic.Int32
	store := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			getJobCalls.Add(1)
			return &domain.Job{ID: "job-1", Version: 1, EndpointURL: "http://example.com"}, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
		JobCacheTTL:  0, // disabled
	})

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	_, _ = exec.resolveJobForRun(context.Background(), run)
	_, _ = exec.resolveJobForRun(context.Background(), run)

	if getJobCalls.Load() != 2 {
		t.Errorf("expected 2 GetJob calls (cache disabled), got %d", getJobCalls.Load())
	}
}

func TestHandleSuccess_LatencyAnomalyDetected(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			return &orcstore.JobHealthStats{P95DurationSecs: 1.0}, nil // P95 = 1s
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	startedAt := time.Now().Add(-3 * time.Second) // 3s ago — exceeds 2 * 1s P95
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", StartedAt: &startedAt}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com"}

	// Should not panic — just verify no error
	exec.handleSuccess(context.Background(), run, job, nil)

	// Verify the run was completed
	calls := store.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected status update")
	}
	if calls[0].to != domain.StatusCompleted {
		t.Errorf("expected completed, got %s", calls[0].to)
	}
}

func TestHandleSuccess_LatencyNormal(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			return &orcstore.JobHealthStats{P95DurationSecs: 10.0}, nil // P95 = 10s
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	startedAt := time.Now().Add(-500 * time.Millisecond) // 0.5s — well within 2 * 10s P95
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", StartedAt: &startedAt}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com"}

	exec.handleSuccess(context.Background(), run, job, nil)

	calls := store.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected status update")
	}
	if calls[0].to != domain.StatusCompleted {
		t.Errorf("expected completed, got %s", calls[0].to)
	}
}

func TestHandleSuccess_NoStatsAvailable(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			return nil, nil // no stats
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	startedAt := time.Now().Add(-3 * time.Second)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", StartedAt: &startedAt}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com"}

	exec.handleSuccess(context.Background(), run, job, nil)

	calls := store.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected status update")
	}
	if calls[0].to != domain.StatusCompleted {
		t.Errorf("expected completed, got %s", calls[0].to)
	}
}

func TestNewExecutor_DefaultHTTPClientBlocksPrivateDNSAtDispatch(t *testing.T) {
	restore := httputil.SetLookupHostForTest(func(host string) ([]string, error) {
		if host != "rebind.test" {
			return nil, fmt.Errorf("unexpected host lookup: %s", host)
		}
		return []string{"127.0.0.1"}, nil
	})
	t.Cleanup(restore)

	exec := NewExecutor(ExecutorConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := exec.dispatchToEndpoint(ctx, "http://rebind.test/hook", &domain.JobRun{
		ID:      "run-ssrf",
		JobID:   "job-ssrf",
		Attempt: 1,
		Payload: json.RawMessage(`{"ok":true}`),
	}, nil)
	if err == nil {
		t.Fatal("expected SSRF-safe executor client to reject private DNS answer")
	}
	if !strings.Contains(err.Error(), "blocked private") && !strings.Contains(err.Error(), "resolves to private") {
		t.Fatalf("expected private-address rejection, got %v", err)
	}
}
