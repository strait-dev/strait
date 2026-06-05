package worker

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	orcstore "strait/internal/store"

	"github.com/stretchr/testify/require"
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

func requireRetryTransition(t *testing.T, calls []statusUpdateCall) statusUpdateCall {
	t.Helper()

	return requireStatusTransition(t, calls, domain.StatusExecuting, domain.StatusQueued)
}

func requireStatusTransition(t *testing.T, calls []statusUpdateCall, from, to domain.RunStatus) statusUpdateCall {
	t.Helper()

	for _, call := range calls {
		if call.from == from && call.to == to {
			return call
		}
	}
	require.Failf(t, "test failure",

		"expected status transition %s -> %s; got %+v", from, to, calls)
	return statusUpdateCall{}
}

func requireOnlyStatusTransition(t *testing.T, calls []statusUpdateCall, from, to domain.RunStatus) statusUpdateCall {
	t.Helper()
	require.Len(t, calls,
		1)

	return requireStatusTransition(t, calls, from, to)
}

func requireRetryPriority(t *testing.T, calls []statusUpdateCall) int {
	t.Helper()

	retryCall := requireRetryTransition(t, calls)
	priority, ok := retryCall.fields["priority"].(int)
	require.True(t,
		ok)

	return priority
}

func requireRetryWithoutPriority(t *testing.T, calls []statusUpdateCall) {
	t.Helper()

	retryCall := requireRetryTransition(t, calls)
	if _, ok := retryCall.fields["priority"]; ok {
		require.Failf(t, "test failure",

			"expected retry transition without priority field, got %+v", retryCall.fields)
	}
}

func requireStatusUpdateTo(t *testing.T, calls []statusUpdateCall, to domain.RunStatus) statusUpdateCall {
	t.Helper()

	for _, call := range calls {
		if call.to == to {
			return call
		}
	}
	require.Failf(t, "test failure",

		"expected status update to %s; got %+v", to, calls)
	return statusUpdateCall{}
}

func requireNoStatusUpdateTo(t *testing.T, calls []statusUpdateCall, to domain.RunStatus) {
	t.Helper()

	for _, call := range calls {
		require.NotEqual(t, to, call.to)
	}
}

func requireLastStatusUpdateTo(t *testing.T, calls []statusUpdateCall, to domain.RunStatus) statusUpdateCall {
	t.Helper()
	require.NotEmpty(t, calls)

	last := calls[len(calls)-1]
	require.Equal(t,
		to, last.to)

	return last
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
		require.Fail(t, msg)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
