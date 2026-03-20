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
		listRunsByProjectFn: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, triggeredBy, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedTriggeredBy = triggeredBy
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?triggered_by=api", "", "proj-1"))
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
		listRunsByProjectFn: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, em *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedMode = em
			return []domain.JobRun{
				{ID: "run-managed", ExecutionMode: domain.ExecutionModeManaged, CreatedAt: time.Now()},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?execution_mode=managed", "", "proj-1"))
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
		listRunsByProjectFn: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, em *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedMode = em
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?execution_mode=http", "", "proj-1"))
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
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?execution_mode=invalid", "", "proj-1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid execution_mode, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRuns_ExecutionModeFilter_NoFilter(t *testing.T) {
	t.Parallel()

	var capturedMode *domain.ExecutionMode
	ms := &mockAPIStore{
		listRunsByProjectFn: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, em *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedMode = em
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs", "", "proj-1"))
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
		listRunsByProjectFn: func(_ context.Context, _ string, status *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, em *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedStatus = status
			capturedMode = em
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?status=completed&execution_mode=managed", "", "proj-1"))
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
	startFn   func(ctx context.Context, machineID string, env map[string]string) error
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
func (m *mockContainerRuntime) Start(ctx context.Context, machineID string, env map[string]string) error {
	if m.startFn != nil {
		return m.startFn(ctx, machineID, env)
	}
	return compute.ErrMachineGone
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
		InternalSecret:           "test-secret",
		MaxBulkTriggerItems:      500,
		JWTSigningKey:            "01234567890123456789012345678901",
		TriggerRateLimitRequests: 10000,
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

func TestHandlePauseRun_ManagedStopsContainer(t *testing.T) {
	t.Parallel()

	stopCalled := false
	rt := &mockContainerRuntime{
		stopFn: func(_ context.Context, machineID string) error {
			if machineID != "m-1" {
				t.Fatalf("expected m-1, got %s", machineID)
			}
			stopCalled = true
			return nil
		},
	}

	getCalls := 0
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting, ExecutionMode: domain.ExecutionModeManaged, MachineID: "m-1"}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusPaused, ExecutionMode: domain.ExecutionModeManaged}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			if from != domain.StatusExecuting || to != domain.StatusPaused {
				t.Fatalf("unexpected transition %s -> %s", from, to)
			}
			return nil
		},
	}

	srv := newTestServerWithRuntime(t, ms, &mockQueue{}, rt)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/pause", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !stopCalled {
		t.Error("expected Stop to be called")
	}
}

func TestHandlePauseRun_HTTPRun_Rejected(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting, ExecutionMode: domain.ExecutionModeHTTP}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/pause", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePauseRun_AlreadyPaused(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusPaused, ExecutionMode: domain.ExecutionModeManaged}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/pause", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected idempotent 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePauseRun_TerminalRun(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusCompleted, ExecutionMode: domain.ExecutionModeManaged}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/pause", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleResumeRun_RequeuesRun(t *testing.T) {
	t.Parallel()

	getCalls := 0
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusPaused, ExecutionMode: domain.ExecutionModeManaged}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeManaged}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			if from != domain.StatusPaused || to != domain.StatusQueued {
				t.Fatalf("unexpected transition %s -> %s", from, to)
			}
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/resume", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleResumeRun_NotPaused(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting, ExecutionMode: domain.ExecutionModeManaged}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/resume", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRestartRun_WithPresetOverride(t *testing.T) {
	t.Parallel()

	var capturedMetadata map[string]any
	getCalls := 0
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting, ExecutionMode: domain.ExecutionModeManaged, MachineID: "m-1"}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeManaged}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, fields map[string]any) error {
			capturedMetadata = fields
			return nil
		},
	}

	rt := &mockContainerRuntime{}
	srv := newTestServerWithRuntime(t, ms, &mockQueue{}, rt)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/restart", `{"machine_preset":"small-1x"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedMetadata == nil {
		t.Fatal("expected metadata to be captured")
	}
	md, ok := capturedMetadata["metadata"].(map[string]string)
	if !ok {
		t.Fatalf("expected metadata to be map[string]string, got %T", capturedMetadata["metadata"])
	}
	if md["_preset_override"] != "small-1x" {
		t.Errorf("expected preset override small-1x, got %q", md["_preset_override"])
	}
}

func TestHandleRestartRun_InvalidPreset(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting, ExecutionMode: domain.ExecutionModeManaged}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/restart", `{"machine_preset":"invalid_preset"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// Phase 3 tests.

func TestHandleResumeRun_PreservesMachineID(t *testing.T) {
	t.Parallel()

	var capturedFields map[string]any
	getCalls := 0
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusPaused, MachineID: "m-paused", ExecutionMode: domain.ExecutionModeManaged}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusQueued, MachineID: "m-paused", ExecutionMode: domain.ExecutionModeManaged}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, fields map[string]any) error {
			capturedFields = fields
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/resume", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, hasMachineID := capturedFields["machine_id"]; hasMachineID {
		t.Error("resume should NOT clear machine_id (preserve for warm start)")
	}
}

func TestHandlePauseRun_ThenResume_EndToEnd(t *testing.T) {
	t.Parallel()

	runState := &domain.JobRun{
		ID:            "run-1",
		Status:        domain.StatusExecuting,
		ExecutionMode: domain.ExecutionModeManaged,
		MachineID:     "m-1",
	}
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return runState, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			if from == domain.StatusExecuting && to == domain.StatusPaused {
				runState.Status = domain.StatusPaused
			} else if from == domain.StatusPaused && to == domain.StatusQueued {
				runState.Status = domain.StatusQueued
			}
			return nil
		},
	}
	rt := &mockContainerRuntime{}

	srv := newTestServerWithRuntime(t, ms, &mockQueue{}, rt)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/pause", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("pause: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if runState.Status != domain.StatusPaused {
		t.Fatalf("expected paused, got %s", runState.Status)
	}

	w = httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/resume", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("resume: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if runState.Status != domain.StatusQueued {
		t.Fatalf("expected queued after resume, got %s", runState.Status)
	}
	if runState.MachineID != "m-1" {
		t.Errorf("expected machine_id preserved, got %q", runState.MachineID)
	}
}

func TestHandleResumeRun_NotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-gone/resume", ""))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleResumeRun_AlreadyQueued(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusQueued}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/resume", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for already-queued, got %d: %s", w.Code, w.Body.String())
	}
}

// Phase 4 tests.

func TestHandlePauseRun_ContainerStopFails_StillPauses(t *testing.T) {
	t.Parallel()

	var paused bool
	getCalls := 0
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting, ExecutionMode: domain.ExecutionModeManaged, MachineID: "m-1"}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusPaused, ExecutionMode: domain.ExecutionModeManaged, MachineID: "m-1"}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			if from == domain.StatusExecuting && to == domain.StatusPaused {
				paused = true
			}
			return nil
		},
	}
	rt := &mockContainerRuntime{
		stopFn: func(_ context.Context, _ string) error {
			return errors.New("stop failed")
		},
	}

	srv := newTestServerWithRuntime(t, ms, &mockQueue{}, rt)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/pause", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !paused {
		t.Error("expected run to be paused even when container stop fails")
	}
}

func TestHandlePauseRun_NotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-gone/pause", ""))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRestartRun_PausedState_StopsNotCalled(t *testing.T) {
	t.Parallel()

	var stopCalled atomic.Bool
	getCalls := 0
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusPaused, ExecutionMode: domain.ExecutionModeManaged, MachineID: "m-1"}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeManaged}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
	}
	rt := &mockContainerRuntime{
		stopFn: func(_ context.Context, _ string) error {
			stopCalled.Store(true)
			return nil
		},
	}

	srv := newTestServerWithRuntime(t, ms, &mockQueue{}, rt)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/restart", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if stopCalled.Load() {
		t.Error("Stop should not be called for paused runs (machine already stopped)")
	}
}

func TestHandleRestartRun_ExecutingState_StopsCalled(t *testing.T) {
	t.Parallel()

	var stopCalled atomic.Bool
	getCalls := 0
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting, ExecutionMode: domain.ExecutionModeManaged, MachineID: "m-1"}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeManaged}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
	}
	rt := &mockContainerRuntime{
		stopFn: func(_ context.Context, _ string) error {
			stopCalled.Store(true)
			return nil
		},
	}

	srv := newTestServerWithRuntime(t, ms, &mockQueue{}, rt)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/restart", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !stopCalled.Load() {
		t.Error("Stop should be called for executing runs on restart")
	}
}

func TestHandleListRuns_ErrorClassFilter(t *testing.T) {
	t.Parallel()
	var capturedErrorClass *string
	ms := &mockAPIStore{
		listRunsByProjectFn: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, errorClass *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedErrorClass = errorClass
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?error_class=timeout", "", "proj-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedErrorClass == nil || *capturedErrorClass != "timeout" {
		t.Fatalf("expected errorClass=timeout, got %v", capturedErrorClass)
	}
}

func TestHandleListRuns_ErrorClassFilterEmpty(t *testing.T) {
	t.Parallel()
	var capturedErrorClass *string
	ms := &mockAPIStore{
		listRunsByProjectFn: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, errorClass *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedErrorClass = errorClass
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs", "", "proj-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedErrorClass != nil {
		t.Fatalf("expected errorClass=nil, got %v", *capturedErrorClass)
	}
}

func TestHandleListRuns_ErrorClassFilterInvalid(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?error_class=invalid_class", "", "proj-1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
