package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/compute"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"
)

func TestHandleResetIdempotencyKey_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		resetRunIdempotencyKeyFn: func(_ context.Context, runID string) error {
			if runID != "run-abc" {
				t.Fatalf("unexpected runID: %s", runID)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-abc/idempotency-key", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"status":"reset"`) {
		t.Fatalf("expected reset status in body, got %s", w.Body.String())
	}
}

func TestHandleResetIdempotencyKey_NotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		resetRunIdempotencyKeyFn: func(_ context.Context, _ string) error {
			return store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-missing/idempotency-key", ""))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRescheduleRun_Success(t *testing.T) {
	t.Parallel()
	scheduledAt := time.Now().Add(1 * time.Hour).Truncate(time.Second)
	ms := &mockAPIStore{
		rescheduleRunFn: func(_ context.Context, runID string, at time.Time, _ json.RawMessage) error {
			if runID != "run-r1" {
				t.Fatalf("unexpected runID: %s", runID)
			}
			return nil
		},
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:          id,
				JobID:       "job-1",
				ProjectID:   "proj-1",
				Status:      domain.StatusDelayed,
				ScheduledAt: &scheduledAt,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	body := `{"scheduled_at":"` + scheduledAt.Format(time.RFC3339) + `"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-r1/reschedule", body))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "run-r1") {
		t.Fatalf("expected run ID in response, got %s", w.Body.String())
	}
}

func TestHandleRescheduleRun_NotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		rescheduleRunFn: func(_ context.Context, _ string, _ time.Time, _ json.RawMessage) error {
			return store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	body := `{"scheduled_at":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-gone/reschedule", body))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRescheduleRun_InvalidBody(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-x/reschedule", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleBulkTrigger_WithTTL(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var enqueuedRuns []*domain.JobRun

	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			mu.Lock()
			enqueuedRuns = append(enqueuedRuns, run)
			mu.Unlock()
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	body := `{"items":[{"ttl_secs":120}]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", body))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(enqueuedRuns) != 1 {
		t.Fatalf("expected 1 enqueued run, got %d", len(enqueuedRuns))
	}
	run := enqueuedRuns[0]
	if run.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	// TTL of 120s means ExpiresAt should be ~120s from now, give generous tolerance.
	diff := time.Until(*run.ExpiresAt)
	if diff < 100*time.Second || diff > 130*time.Second {
		t.Fatalf("expected ExpiresAt ~120s from now, got %v", diff)
	}
}

func TestHandleListRuns_TriggeredByFilter(t *testing.T) {
	t.Parallel()

	var capturedTriggeredBy *string
	ms := &mockAPIStore{
		listRunsByProjectFn: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, triggeredBy, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedTriggeredBy = triggeredBy
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs?project_id=proj-1&triggered_by=api", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedTriggeredBy == nil {
		t.Fatal("expected triggeredBy parameter to be passed to store")
	}
	if *capturedTriggeredBy != "api" {
		t.Fatalf("expected triggeredBy=api, got %q", *capturedTriggeredBy)
	}
}

func TestHandleListRuns_ExecutionModeFilter_Managed(t *testing.T) {
	t.Parallel()

	var capturedMode *domain.ExecutionMode
	ms := &mockAPIStore{
		listRunsByProjectFn: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, em *domain.ExecutionMode, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedMode = em
			return []domain.JobRun{
				{ID: "run-managed", ExecutionMode: domain.ExecutionModeManaged, CreatedAt: time.Now()},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs?project_id=proj-1&execution_mode=managed", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedMode == nil {
		t.Fatal("expected execution_mode to be passed to store")
	}
	if *capturedMode != domain.ExecutionModeManaged {
		t.Fatalf("expected managed, got %q", *capturedMode)
	}
}

func TestHandleListRuns_ExecutionModeFilter_HTTP(t *testing.T) {
	t.Parallel()

	var capturedMode *domain.ExecutionMode
	ms := &mockAPIStore{
		listRunsByProjectFn: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, em *domain.ExecutionMode, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedMode = em
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs?project_id=proj-1&execution_mode=http", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedMode == nil || *capturedMode != domain.ExecutionModeHTTP {
		t.Fatal("expected http execution mode")
	}
}

func TestHandleListRuns_ExecutionModeFilter_Invalid(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs?project_id=proj-1&execution_mode=invalid", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid execution_mode, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRuns_ExecutionModeFilter_NoFilter(t *testing.T) {
	t.Parallel()

	var capturedMode *domain.ExecutionMode
	ms := &mockAPIStore{
		listRunsByProjectFn: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, em *domain.ExecutionMode, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedMode = em
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs?project_id=proj-1", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedMode != nil {
		t.Fatal("expected nil execution_mode when no filter provided")
	}
}

func TestHandleListRuns_ExecutionModeFilter_CombinedWithStatus(t *testing.T) {
	t.Parallel()

	var capturedStatus *domain.RunStatus
	var capturedMode *domain.ExecutionMode
	ms := &mockAPIStore{
		listRunsByProjectFn: func(_ context.Context, _ string, status *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, em *domain.ExecutionMode, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedStatus = status
			capturedMode = em
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs?project_id=proj-1&status=completed&execution_mode=managed", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedStatus == nil || *capturedStatus != domain.StatusCompleted {
		t.Fatal("expected completed status")
	}
	if capturedMode == nil || *capturedMode != domain.ExecutionModeManaged {
		t.Fatal("expected managed execution mode")
	}
}

func TestHandleBulkTrigger_WithConcurrencyKey(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var enqueuedRuns []*domain.JobRun

	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			mu.Lock()
			enqueuedRuns = append(enqueuedRuns, run)
			mu.Unlock()
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	body := `{"items":[{"concurrency_key":"tenant-42"}]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", body))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(enqueuedRuns) != 1 {
		t.Fatalf("expected 1 enqueued run, got %d", len(enqueuedRuns))
	}
	if enqueuedRuns[0].ConcurrencyKey != "tenant-42" {
		t.Fatalf("expected ConcurrencyKey=tenant-42, got %q", enqueuedRuns[0].ConcurrencyKey)
	}
}

func TestHandleTrigger_DefaultRunMetadataMerge(t *testing.T) {
	t.Parallel()
	var enqueued *domain.JobRun
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:                 id,
				ProjectID:          "proj-1",
				Enabled:            true,
				TimeoutSecs:        60,
				DefaultRunMetadata: map[string]string{"env": "prod", "dependency_key": "default-dep"},
			}, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = run
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	// Payload includes dependency_key which becomes run metadata; it should win over the job default.
	body := `{"payload":{"dependency_key":"user-dep"}}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if enqueued == nil {
		t.Fatal("expected run to be enqueued")
	}
	if enqueued.Metadata["env"] != "prod" {
		t.Fatalf("expected env=prod from defaults, got %q", enqueued.Metadata["env"])
	}
	if enqueued.Metadata["dependency_key"] != "user-dep" {
		t.Fatalf("expected dependency_key=user-dep (user override), got %q", enqueued.Metadata["dependency_key"])
	}
}

func TestHandleBulkTrigger_BatchIDSet(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var enqueuedRuns []*domain.JobRun
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			mu.Lock()
			enqueuedRuns = append(enqueuedRuns, run)
			mu.Unlock()
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	body := `{"items":[{},{}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", body))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(enqueuedRuns) != 2 {
		t.Fatalf("expected 2 enqueued runs, got %d", len(enqueuedRuns))
	}
	if enqueuedRuns[0].BatchID == "" {
		t.Fatal("expected non-empty BatchID on first run")
	}
	if enqueuedRuns[0].BatchID != enqueuedRuns[1].BatchID {
		t.Fatalf("expected same BatchID on both runs, got %q and %q", enqueuedRuns[0].BatchID, enqueuedRuns[1].BatchID)
	}
}

func TestHandleCreateJob_MaxConcurrencyPerKey(t *testing.T) {
	t.Parallel()
	var created *domain.Job
	ms := &mockAPIStore{
		createJobFn: func(_ context.Context, job *domain.Job) error {
			job.ID = "job-new"
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			created = job
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{
		"project_id": "proj-1",
		"name": "Job with PerKey",
		"slug": "job-per-key",
		"endpoint_url": "https://example.com/callback",
		"max_concurrency_per_key": 5
	}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if created == nil {
		t.Fatal("expected CreateJob to be called")
	}
	if created.MaxConcurrencyPerKey != 5 {
		t.Fatalf("expected MaxConcurrencyPerKey=5, got %d", created.MaxConcurrencyPerKey)
	}
}

// mockContainerRuntime implements compute.ContainerRuntime for API handler tests.
type mockContainerRuntime struct {
	runFn     func(ctx context.Context, req compute.RunRequest) (*compute.RunResult, error)
	createFn  func(ctx context.Context, req compute.RunRequest) (string, error)
	waitFn    func(ctx context.Context, machineID string, timeoutSecs int) (*compute.RunResult, error)
	stopFn    func(ctx context.Context, machineID string) error
	destroyFn func(ctx context.Context, machineID string) error
	statusFn  func(ctx context.Context, machineID string) (compute.MachineStatus, error)
	getLogsFn func(ctx context.Context, machineID string, lines int) (string, error)
}

func (m *mockContainerRuntime) Run(ctx context.Context, req compute.RunRequest) (*compute.RunResult, error) {
	if m.runFn != nil {
		return m.runFn(ctx, req)
	}
	return &compute.RunResult{ExitCode: 0}, nil
}
func (m *mockContainerRuntime) Create(ctx context.Context, req compute.RunRequest) (string, error) {
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return "mock-machine-id", nil
}
func (m *mockContainerRuntime) Wait(ctx context.Context, machineID string, timeoutSecs int) (*compute.RunResult, error) {
	if m.waitFn != nil {
		return m.waitFn(ctx, machineID, timeoutSecs)
	}
	return &compute.RunResult{MachineID: machineID, ExitCode: 0}, nil
}
func (m *mockContainerRuntime) Stop(ctx context.Context, machineID string) error {
	if m.stopFn != nil {
		return m.stopFn(ctx, machineID)
	}
	return nil
}
func (m *mockContainerRuntime) Destroy(ctx context.Context, machineID string) error {
	if m.destroyFn != nil {
		return m.destroyFn(ctx, machineID)
	}
	return nil
}
func (m *mockContainerRuntime) Status(ctx context.Context, machineID string) (compute.MachineStatus, error) {
	if m.statusFn != nil {
		return m.statusFn(ctx, machineID)
	}
	return compute.MachineStatusStopped, nil
}
func (m *mockContainerRuntime) GetLogs(ctx context.Context, machineID string, lines int) (string, error) {
	if m.getLogsFn != nil {
		return m.getLogsFn(ctx, machineID, lines)
	}
	return "", nil
}

// newTestServerWithRuntime creates a test server with an optional container runtime.
func newTestServerWithRuntime(t *testing.T, s APIStore, q *mockQueue, rt compute.ContainerRuntime) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       "01234567890123456789012345678901",
	}
	srv := NewServer(ServerDeps{
		Config:           cfg,
		Store:            s,
		Queue:            q,
		ContainerRuntime: rt,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestHandleCancelRun_ManagedStopsContainer(t *testing.T) {
	t.Parallel()

	var stoppedMachine atomic.Value
	rt := &mockContainerRuntime{
		stopFn: func(_ context.Context, machineID string) error {
			stoppedMachine.Store(machineID)
			return nil
		},
	}

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:            id,
				JobID:         "job-1",
				ProjectID:     "proj-1",
				Status:        domain.StatusExecuting,
				ExecutionMode: domain.ExecutionModeManaged,
				MachineID:     "m-abc-123",
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		cancelChildRunsByParentIDsFn: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
	}

	srv := newTestServerWithRuntime(t, ms, &mockQueue{}, rt)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-1", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	got, ok := stoppedMachine.Load().(string)
	if !ok || got != "m-abc-123" {
		t.Fatalf("expected Stop(m-abc-123) to be called, got %q", got)
	}
}

func TestHandleCancelRun_StopError(t *testing.T) {
	t.Parallel()

	rt := &mockContainerRuntime{
		stopFn: func(_ context.Context, _ string) error {
			return errors.New("fly API down")
		},
	}

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:            id,
				JobID:         "job-1",
				ProjectID:     "proj-1",
				Status:        domain.StatusExecuting,
				ExecutionMode: domain.ExecutionModeManaged,
				MachineID:     "m-fail",
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		cancelChildRunsByParentIDsFn: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
	}

	srv := newTestServerWithRuntime(t, ms, &mockQueue{}, rt)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-1", ""))
	// Cancel should still succeed even if Stop fails.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even when Stop fails, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCancelRun_HTTPRun_NoStopCall(t *testing.T) {
	t.Parallel()

	var stopCalled atomic.Bool
	rt := &mockContainerRuntime{
		stopFn: func(_ context.Context, _ string) error {
			stopCalled.Store(true)
			return nil
		},
	}

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:            id,
				JobID:         "job-1",
				ProjectID:     "proj-1",
				Status:        domain.StatusExecuting,
				ExecutionMode: domain.ExecutionModeHTTP,
				MachineID:     "m-http",
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		cancelChildRunsByParentIDsFn: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
	}

	srv := newTestServerWithRuntime(t, ms, &mockQueue{}, rt)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-1", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if stopCalled.Load() {
		t.Fatal("Stop should not be called for HTTP execution mode runs")
	}
}

func TestHandleCancelRun_NoMachineID(t *testing.T) {
	t.Parallel()

	var stopCalled atomic.Bool
	rt := &mockContainerRuntime{
		stopFn: func(_ context.Context, _ string) error {
			stopCalled.Store(true)
			return nil
		},
	}

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:            id,
				JobID:         "job-1",
				ProjectID:     "proj-1",
				Status:        domain.StatusExecuting,
				ExecutionMode: domain.ExecutionModeManaged,
				MachineID:     "", // Empty machine ID.
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		cancelChildRunsByParentIDsFn: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
	}

	srv := newTestServerWithRuntime(t, ms, &mockQueue{}, rt)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-1", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if stopCalled.Load() {
		t.Fatal("Stop should not be called when MachineID is empty")
	}
}

func TestParseBracketParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		param  string
		prefix string
		wantK  string
		wantOK bool
	}{
		{"metadata[env]", "metadata", "env", true},
		{"metadata[customer_id]", "metadata", "customer_id", true},
		{"tags[team]", "tags", "team", true},
		{"metadata[]", "metadata", "", false},
		{"metadata", "metadata", "", false},
		{"other[key]", "metadata", "", false},
		{"metadata[key", "metadata", "", false},
		{"status", "metadata", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.param, func(t *testing.T) {
			t.Parallel()
			k, ok := parseBracketParam(tt.param, tt.prefix)
			if ok != tt.wantOK || k != tt.wantK {
				t.Errorf("parseBracketParam(%q, %q) = (%q, %v), want (%q, %v)", tt.param, tt.prefix, k, ok, tt.wantK, tt.wantOK)
			}
		})
	}
}
