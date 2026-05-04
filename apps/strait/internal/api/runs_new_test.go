package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestHandleResetIdempotencyKey_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ResetRunIdempotencyKeyFunc: func(_ context.Context, runID string) error {
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
	ms := &APIStoreMock{
		ResetRunIdempotencyKeyFunc: func(_ context.Context, _ string) error {
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
	ms := &APIStoreMock{
		RescheduleRunFunc: func(_ context.Context, runID string, at time.Time, _ json.RawMessage) error {
			if runID != "run-r1" {
				t.Fatalf("unexpected runID: %s", runID)
			}
			return nil
		},
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
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
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
		RescheduleRunFunc: func(_ context.Context, _ string, _ time.Time, _ json.RawMessage) error {
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
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-x/reschedule", ""))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleBulkTrigger_WithTTL(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var enqueuedRuns []*domain.JobRun

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
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
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, triggeredBy, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
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

func TestHandleListRuns_ExecutionModeFilter_HTTP(t *testing.T) {
	t.Parallel()

	var capturedMode *domain.ExecutionMode
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, em *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
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

	ms := &APIStoreMock{}
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
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, em *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
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

func TestHandleBulkTrigger_WithConcurrencyKey(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var enqueuedRuns []*domain.JobRun

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:                 id,
				ProjectID:          "proj-1",
				Enabled:            true,
				TimeoutSecs:        60,
				DefaultRunMetadata: map[string]string{"env": "prod", "dependency_key": "default-dep"},
			}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
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
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
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

func TestHandlePauseRun_HTTPRun_CanBePaused(t *testing.T) {
	t.Parallel()

	getCalls := 0
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting, ExecutionMode: domain.ExecutionModeHTTP}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusPaused, ExecutionMode: domain.ExecutionModeHTTP}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/pause", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for HTTP run pause, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePauseRun_AlreadyPaused(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusPaused, ExecutionMode: domain.ExecutionModeHTTP}, nil
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

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusCompleted, ExecutionMode: domain.ExecutionModeHTTP}, nil
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
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusPaused, ExecutionMode: domain.ExecutionModeHTTP}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeHTTP}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
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

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting, ExecutionMode: domain.ExecutionModeHTTP}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/resume", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRestartRun_WrongStatus_Rejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusCompleted, ExecutionMode: domain.ExecutionModeHTTP}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/restart", `{}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for completed run restart, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleResumeRun_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
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
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
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

func TestHandlePauseRun_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
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

func TestHandleListRuns_ErrorClassFilter(t *testing.T) {
	t.Parallel()
	var capturedErrorClass *string
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, errorClass *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
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
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, errorClass *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
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
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?error_class=invalid_class", "", "proj-1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
