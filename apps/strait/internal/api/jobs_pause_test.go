package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// Pause endpoint tests.

func TestHandlePauseJob_Success(t *testing.T) {
	t.Parallel()
	pausedJob := &domain.Job{ID: "job-1", ProjectID: "proj-1", Enabled: true, Paused: true}
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, Paused: false}, nil
			}
			return pausedJob, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{"reason":"investigating timeout spike"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["id"] != "job-1" {
		t.Fatalf("expected id=job-1, got %v", body["id"])
	}
	if body["paused"] != true {
		t.Fatalf("expected paused=true in response, got %v", body["paused"])
	}
}

func TestHandlePauseJob_ReturnsFullJobWithEnabledField(t *testing.T) {
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, Paused: false}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, Paused: true, PauseReason: "test"}, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{"reason":"test"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// The response must include both enabled and paused so the user can see the full state.
	if _, ok := body["enabled"]; !ok {
		t.Fatal("expected 'enabled' field in pause response")
	}
	if _, ok := body["paused"]; !ok {
		t.Fatal("expected 'paused' field in pause response")
	}
}

func TestHandlePauseJob_WithReason(t *testing.T) {
	t.Parallel()
	var capturedReason string
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false}, nil
		},
		PauseJobFunc: func(_ context.Context, _ string, reason string) error {
			capturedReason = reason
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{"reason":"deploy in progress"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedReason != "deploy in progress" {
		t.Fatalf("expected reason 'deploy in progress', got %q", capturedReason)
	}
}

func TestHandlePauseJob_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/nonexistent/pause", `{}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePauseJob_AlreadyPaused(t *testing.T) {
	t.Parallel()
	var pauseCalled bool
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true, Enabled: false}, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			pauseCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (idempotent), got %d: %s", w.Code, w.Body.String())
	}
	if pauseCalled {
		t.Fatal("PauseJob should not be called when already paused")
	}

	// Even when idempotent, the response should return the full job
	// so the caller can see that enabled=false.
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["enabled"] != false {
		t.Fatalf("expected enabled=false in idempotent response, got %v", body["enabled"])
	}
}

func TestHandlePauseJob_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false}, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return fmt.Errorf("db connection lost")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{}`))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePauseJob_EmitsAuditEvent(t *testing.T) {
	t.Parallel()
	var capturedEvent *domain.AuditEvent
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false}, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			capturedEvent = ev
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{"reason":"incident"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedEvent == nil {
		t.Fatal("expected audit event to be created")
	}
	if capturedEvent.Action != "job.paused" {
		t.Fatalf("expected action job.paused, got %s", capturedEvent.Action)
	}
	if capturedEvent.ResourceType != "job" {
		t.Fatalf("expected resource_type job, got %s", capturedEvent.ResourceType)
	}
	if capturedEvent.ResourceID != "job-1" {
		t.Fatalf("expected resource_id job-1, got %s", capturedEvent.ResourceID)
	}
	var details map[string]any
	if err := json.Unmarshal(capturedEvent.Details, &details); err != nil {
		t.Fatalf("unmarshal details: %v", err)
	}
	if details["reason"] != "incident" {
		t.Fatalf("expected reason=incident in details, got %v", details["reason"])
	}
}

// Resume endpoint tests.

func TestHandleResumeJob_Success(t *testing.T) {
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true, Enabled: true}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false, Enabled: true}, nil
		},
		ResumeJobFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["id"] != "job-1" {
		t.Fatalf("expected id=job-1, got %v", body["id"])
	}
	if body["paused"] != false {
		t.Fatalf("expected paused=false in response, got %v", body["paused"])
	}
}

func TestHandleResumeJob_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/nonexistent/resume", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleResumeJob_NotPaused(t *testing.T) {
	t.Parallel()
	var resumeCalled bool
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false, Enabled: true}, nil
		},
		ResumeJobFunc: func(_ context.Context, _ string) error {
			resumeCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (idempotent), got %d: %s", w.Code, w.Body.String())
	}
	if resumeCalled {
		t.Fatal("ResumeJob should not be called when not paused")
	}
}

func TestHandleResumeJob_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true}, nil
		},
		ResumeJobFunc: func(_ context.Context, _ string) error {
			return fmt.Errorf("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleResumeJob_EmitsAuditEvent(t *testing.T) {
	t.Parallel()
	var capturedEvent *domain.AuditEvent
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true}, nil
		},
		ResumeJobFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			capturedEvent = ev
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedEvent == nil {
		t.Fatal("expected audit event to be created")
	}
	if capturedEvent.Action != "job.resumed" {
		t.Fatalf("expected action job.resumed, got %s", capturedEvent.Action)
	}
	if capturedEvent.ResourceID != "job-1" {
		t.Fatalf("expected resource_id job-1, got %s", capturedEvent.ResourceID)
	}
}

// GET includes pause state.

func TestGetJob_IncludesPauseState(t *testing.T) {
	t.Parallel()
	pausedAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Name: "test-job", Slug: "test-job",
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
				Enabled: true, Paused: true, PausedAt: &pausedAt, PauseReason: "incident investigation",
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["paused"] != true {
		t.Fatalf("expected paused=true, got %v", body["paused"])
	}
	if body["paused_at"] == nil {
		t.Fatal("expected paused_at to be present")
	}
	if body["pause_reason"] != "incident investigation" {
		t.Fatalf("expected pause_reason='incident investigation', got %v", body["pause_reason"])
	}
}

func TestGetJob_UnpausedOmitsPauseFields(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Name: "test-job", Slug: "test-job",
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
				Enabled: true, Paused: false,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["paused"] != false {
		t.Fatalf("expected paused=false, got %v", body["paused"])
	}
	if _, ok := body["paused_at"]; ok {
		t.Fatal("expected paused_at to be omitted for unpaused job")
	}
	if _, ok := body["pause_reason"]; ok {
		t.Fatal("expected pause_reason to be omitted for unpaused job")
	}
}

func TestListJobs_IncludesPauseState(t *testing.T) {
	t.Parallel()
	pausedAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	ms := &APIStoreMock{
		ListJobsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.Job, error) {
			return []domain.Job{{
				ID: "job-1", ProjectID: "proj-1", Name: "paused-job", Slug: "paused-job",
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
				Enabled: true, Paused: true, PausedAt: &pausedAt, PauseReason: "deploy",
			}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/jobs", "", "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) == 0 {
		t.Fatal("expected at least one job in response")
	}
	if resp.Data[0]["paused"] != true {
		t.Fatalf("expected paused=true in list response, got %v", resp.Data[0]["paused"])
	}
	if resp.Data[0]["pause_reason"] != "deploy" {
		t.Fatalf("expected pause_reason=deploy, got %v", resp.Data[0]["pause_reason"])
	}
}

// Edge case: trigger rejection for paused jobs.

func TestTriggerJob_RejectedWhenPaused(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, Paused: true,
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTriggerJob_AllowedWhenNotPaused(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, Paused: false,
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{MaxQueuedRuns: 1000}, nil
		},
		CountProjectQueuedRunsFunc: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		FindRecentRunByPayloadFunc: func(_ context.Context, _ string, _ json.RawMessage, _ time.Time) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
		CountRunsForJobSinceFunc: func(_ context.Context, _ string, _ time.Time) (int, error) {
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`))

	// Should succeed (not 409) since the job is not paused.
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBulkTriggerJob_RejectedWhenPaused(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, Paused: true,
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", `{"items":[{}]}`))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

// Edge case: enabled + paused double gate.

func TestResumeJob_ShowsDisabledState(t *testing.T) {
	// A job that was paused AND disabled. Resuming clears pause but
	// the response must show enabled=false so the user knows it's still disabled.
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true, Enabled: false}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false, Enabled: false}, nil
		},
		ResumeJobFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["paused"] != false {
		t.Fatalf("expected paused=false after resume, got %v", body["paused"])
	}
	if body["enabled"] != false {
		t.Fatalf("expected enabled=false to be visible in resume response, got %v", body["enabled"])
	}
}

// Edge case: group pause + individual resume.

func TestGroupPause_IndividualResume(t *testing.T) {
	t.Parallel()

	// After group pause, individual resume should work and return the job.
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				// First call: job is group-paused.
				return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true, PauseReason: "group pause", Enabled: true}, nil
			}
			// After resume:
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false, Enabled: true}, nil
		},
		ResumeJobFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["paused"] != false {
		t.Fatalf("expected paused=false after individual resume, got %v", body["paused"])
	}
}

// Adversarial tests.

func TestTriggerJob_DryRunRejectedWhenPaused(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, Paused: true,
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"dry_run":true}`))

	// Dry-run must also reject paused jobs. If it returned 200, the user
	// would think triggering is possible when it isn't.
	if w.Code == http.StatusOK {
		t.Fatal("dry-run on paused job should NOT return 200 — must reject like real trigger")
	}
}

func TestTriggerJob_DryRunAllowedWhenNotPaused(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, Paused: false,
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{MaxQueuedRuns: 1000}, nil
		},
		CountProjectQueuedRunsFunc: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		FindRecentRunByPayloadFunc: func(_ context.Context, _ string, _ json.RawMessage, _ time.Time) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
		CountRunsForJobSinceFunc: func(_ context.Context, _ string, _ time.Time) (int, error) {
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"dry_run":true}`))

	if w.Code != http.StatusOK {
		t.Fatalf("dry-run on active job should return 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPauseJob_IdempotentAlwaysReturnsFreshState(t *testing.T) {
	// When pausing an already-paused job, the response must come from a
	// fresh re-fetch, not from the initial check. This ensures consistent
	// state even under concurrent modifications.
	t.Parallel()
	freshPausedAt := time.Date(2026, 3, 24, 15, 0, 0, 0, time.UTC)
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			// Both calls return a paused job, but the second call returns
			// an updated timestamp (simulating concurrent modification).
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Paused: true,
				PausedAt: &freshPausedAt, PauseReason: "latest reason",
				Enabled: true,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// Verify the response has the fresh state.
	if body["pause_reason"] != "latest reason" {
		t.Fatalf("expected fresh pause_reason, got %v", body["pause_reason"])
	}
}

func TestResumeJob_IdempotentAlwaysReturnsFreshState(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Paused: false, Enabled: true,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["paused"] != false {
		t.Fatalf("expected paused=false, got %v", body["paused"])
	}
	if body["enabled"] != true {
		t.Fatalf("expected enabled=true, got %v", body["enabled"])
	}
}

func TestPauseJob_NoAuditEventWhenAlreadyPaused(t *testing.T) {
	t.Parallel()
	var auditCalled bool
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true, Enabled: true}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			auditCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{"reason":"duplicate"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if auditCalled {
		t.Fatal("audit event should NOT be emitted when job is already paused")
	}
}

func TestResumeJob_NoAuditEventWhenNotPaused(t *testing.T) {
	t.Parallel()
	var auditCalled bool
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false, Enabled: true}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			auditCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if auditCalled {
		t.Fatal("audit event should NOT be emitted when job is not paused")
	}
}

func TestPauseJob_DisabledJobCanBePaused(t *testing.T) {
	// A disabled job should still be pausable. These are independent states.
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: false, Paused: false}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: false, Paused: true, PauseReason: "maintenance"}, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{"reason":"maintenance"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["enabled"] != false {
		t.Fatalf("expected enabled=false, got %v", body["enabled"])
	}
	if body["paused"] != true {
		t.Fatalf("expected paused=true, got %v", body["paused"])
	}
}

func TestPauseDisableResume_JobStillDisabled(t *testing.T) {
	// Lifecycle: pause -> disable -> resume. After resume, job should
	// be enabled=false, paused=false. Runs still won't dequeue because
	// enabled=false. The resume response makes this visible.
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: false, Paused: true}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: false, Paused: false}, nil
		},
		ResumeJobFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["paused"] != false {
		t.Fatalf("expected paused=false after resume, got %v", body["paused"])
	}
	if body["enabled"] != false {
		t.Fatalf("expected enabled=false (still disabled), got %v", body["enabled"])
	}
}

func TestTriggerJob_DisabledCheckBeforePausedCheck(t *testing.T) {
	// When a job is both disabled and paused, the disabled check should
	// fire first (400) rather than the paused check (409).
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: false, Paused: true,
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (disabled takes precedence), got %d: %s", w.Code, w.Body.String())
	}
}

func TestPauseJob_EmptyReasonIsAccepted(t *testing.T) {
	t.Parallel()
	var capturedReason string
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true}, nil
		},
		PauseJobFunc: func(_ context.Context, _ string, reason string) error {
			capturedReason = reason
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedReason != "" {
		t.Fatalf("expected empty reason, got %q", capturedReason)
	}
}

func TestPauseJob_EmptyBodyIsAccepted(t *testing.T) {
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true}, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with empty body, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPauseJob_GetJobFailsAfterPause(t *testing.T) {
	// If the re-fetch after PauseJob fails, we should get 500.
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false}, nil
			}
			return nil, fmt.Errorf("db gone")
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{}`))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when re-fetch fails, got %d: %s", w.Code, w.Body.String())
	}
}
