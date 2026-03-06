package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"orchestrator/internal/config"
	"orchestrator/internal/domain"
	"orchestrator/internal/pubsub"
	"orchestrator/internal/store"
)

func newTestServer(t *testing.T, s APIStore, q *mockQueue, pub *mockPublisher) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret: "test-secret",
		JWTSigningKey:  "01234567890123456789012345678901",
	}
	var p *mockPublisher
	if pub != nil {
		p = pub
	}
	return NewServer(cfg, s, q, p, nil, nil, nil, nil)
}

func newTestServerWithPinger(t *testing.T, s APIStore, q *mockQueue, pub *mockPublisher, pinger Pinger) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret: "test-secret",
		JWTSigningKey:  "test-jwt-key-must-be-32-chars-long",
	}
	var p pubsub.Publisher
	if pub != nil {
		p = pub
	}
	return NewServer(cfg, s, q, p, nil, pinger, nil, nil)
}

func authedRequest(method, path string, body string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Header.Set("X-Internal-Secret", "test-secret")
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestHandleHealth(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("expected status=ok, got %q", resp["status"])
	}
}

func TestHandleAuth_MissingSecret(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/jobs/", nil)
	// No X-Internal-Secret header

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandleCreateJob_Success(t *testing.T) {
	var created atomic.Bool
	ms := &mockAPIStore{
		createJobFn: func(_ context.Context, job *domain.Job) error {
			created.Store(true)
			job.ID = "job-123"
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "Test Job",
		"slug": "test-job",
		"endpoint_url": "https://example.com/callback"
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !created.Load() {
		t.Fatal("CreateJob was not called")
	}
}

func TestHandleCreateJob_MissingFields(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", `{}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateJob_TagsFeatureDisabled(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	body := `{
		"project_id": "proj-1",
		"name": "Tagged Job",
		"slug": "tagged-job",
		"endpoint_url": "https://example.com/callback",
		"tags": {"team": "core"}
	}`

	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "job tags feature is not enabled") {
		t.Fatalf("expected disabled-tags error, got %s", w.Body.String())
	}
}

func TestHandleCreateJob_ValidateTagsTooMany(t *testing.T) {
	tags := make(map[string]string)
	for i := range 21 {
		tags[strings.Repeat("k", i+1)] = "v"
	}

	req := map[string]any{
		"project_id":   "proj-1",
		"name":         "Tagged Job",
		"slug":         "tagged-job",
		"endpoint_url": "https://example.com/callback",
		"tags":         tags,
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	srv.config.FFJobTags = true
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", string(body)))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "too many tags (max 20)") {
		t.Fatalf("expected too-many-tags error, got %s", w.Body.String())
	}
}

func TestHandleCreateJob_ValidateTagsKeyTooLong(t *testing.T) {
	req := map[string]any{
		"project_id":   "proj-1",
		"name":         "Tagged Job",
		"slug":         "tagged-job",
		"endpoint_url": "https://example.com/callback",
		"tags": map[string]string{
			strings.Repeat("k", 65): "core",
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	srv.config.FFJobTags = true
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", string(body)))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "tag key too long (max 64 characters)") {
		t.Fatalf("expected key-too-long error, got %s", w.Body.String())
	}
}

func TestHandleGetJob_Success(t *testing.T) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Test",
				Slug:        "test",
				EndpointURL: "https://example.com",
				Enabled:     true,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-123", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["id"] != "job-123" {
		t.Fatalf("expected id=job-123, got %v", resp["id"])
	}
}

func TestHandleGetJob_NotFound(t *testing.T) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/nonexistent", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleListJobs_Success(t *testing.T) {
	ms := &mockAPIStore{
		listJobsFn: func(_ context.Context, projectID string) ([]domain.Job, error) {
			return []domain.Job{
				{ID: "job-1", ProjectID: projectID, Name: "Job 1"},
				{ID: "job-2", ProjectID: projectID, Name: "Job 2"},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/?project_id=proj-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(resp))
	}
}

func TestHandleListJobs_MissingProjectID(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleListJobs_FilterByTag(t *testing.T) {
	ms := &mockAPIStore{
		listJobsByTagFn: func(_ context.Context, projectID, tagKey, tagValue string) ([]domain.Job, error) {
			if projectID != "proj-1" || tagKey != "team" || tagValue != "core" {
				t.Fatalf("unexpected list by tag args: %q %q %q", projectID, tagKey, tagValue)
			}
			return []domain.Job{{ID: "job-1", ProjectID: projectID, Name: "Job 1"}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobTags = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/?project_id=proj-1&tag_key=team&tag_value=core", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 job, got %d", len(resp))
	}
}

func TestHandleListJobs_FilterByTag_FeatureDisabled(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/?project_id=proj-1&tag_key=team&tag_value=core", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "job tags feature is not enabled") {
		t.Fatalf("expected disabled-tags error, got %s", w.Body.String())
	}
}

func TestHandleTriggerJob_Success(t *testing.T) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Test",
				Slug:        "test",
				EndpointURL: "https://example.com/callback",
				Enabled:     true,
				TimeoutSecs: 300,
				MaxAttempts: 3,
			}, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Fatal("expected non-empty run id")
	}
	if resp["run_token"] == nil || resp["run_token"] == "" {
		t.Fatal("expected non-empty run_token")
	}
	if resp["status"] != "queued" {
		t.Fatalf("expected status=queued, got %v", resp["status"])
	}
}

func TestHandleTriggerJob_DisabledJob(t *testing.T) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Disabled",
				Enabled:     false,
				EndpointURL: "https://example.com",
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleStats_Success(t *testing.T) {
	ms := &mockAPIStore{
		queueStatsFn: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 5, Executing: 2, Delayed: 1}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/stats", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["queued"] != float64(5) {
		t.Fatalf("expected queued=5, got %v", resp["queued"])
	}
}

func TestHandleCancelRun_Success(t *testing.T) {
	callCount := 0
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			callCount++
			if callCount == 1 {
				return &domain.JobRun{
					ID:        id,
					JobID:     "job-1",
					ProjectID: "proj-1",
					Status:    domain.StatusExecuting,
				}, nil
			}
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusCanceled,
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _ domain.RunStatus, to domain.RunStatus, _ map[string]any) error {
			if to != domain.StatusCanceled {
				t.Errorf("expected transition to canceled, got %s", to)
			}
			return nil
		},
		listChildRunsFn: func(_ context.Context, _ string) ([]domain.JobRun, error) {
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-123", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteJob_SoftDelete(t *testing.T) {
	var updatedJob *domain.Job
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Test",
				Slug:        "test",
				EndpointURL: "https://example.com",
				Enabled:     true,
			}, nil
		},
		updateJobFn: func(_ context.Context, job *domain.Job) error {
			updatedJob = job
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/jobs/job-123", ""))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if updatedJob == nil {
		t.Fatal("UpdateJob was not called")
	}
	if updatedJob.Enabled {
		t.Fatal("expected job to be disabled after soft delete")
	}
}

func TestHandleUpdateJob_ValidateTagsValueTooLong(t *testing.T) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Name: "Test", Slug: "test", EndpointURL: "https://example.com", Enabled: true}, nil
		},
		updateJobFn: func(_ context.Context, _ *domain.Job) error {
			t.Fatal("UpdateJob should not be called for invalid tags")
			return nil
		},
	}

	req := map[string]any{
		"tags": map[string]string{
			"team": strings.Repeat("v", 257),
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobTags = true
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", string(body)))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "tag value too long (max 256 characters)") {
		t.Fatalf("expected value-too-long error, got %s", w.Body.String())
	}
}

func TestHandleUpdateJob_ValidateTagsEmptyKey(t *testing.T) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Name: "Test", Slug: "test", EndpointURL: "https://example.com", Enabled: true}, nil
		},
		updateJobFn: func(_ context.Context, _ *domain.Job) error {
			t.Fatal("UpdateJob should not be called for invalid tags")
			return nil
		},
	}

	req := map[string]any{
		"tags": map[string]string{
			"": "core",
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobTags = true
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", string(body)))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "tag keys must be non-empty") {
		t.Fatalf("expected empty-key error, got %s", w.Body.String())
	}
}

func TestHandleReplayRun_Success(t *testing.T) {
	originalPayload := json.RawMessage(`{"k":"v"}`)
	originalRun := &domain.JobRun{
		ID:             "run-123",
		JobID:          "job-1",
		ProjectID:      "proj-1",
		Status:         domain.StatusFailed,
		Attempt:        3,
		Payload:        originalPayload,
		IdempotencyKey: "idem-123",
		JobVersion:     5,
		Priority:       7,
	}

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			if id != "run-123" {
				t.Fatalf("unexpected run id: %s", id)
			}
			return originalRun, nil
		},
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			if id != "job-1" {
				t.Fatalf("unexpected job id: %s", id)
			}
			return &domain.Job{ID: id, TimeoutSecs: 60, RunTTLSecs: 0, Enabled: true}, nil
		},
	}

	var enqueued *domain.JobRun
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = run
			return nil
		},
	}

	srv := newTestServer(t, ms, mq, nil)
	srv.config.FFRunReplay = true
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-123/replay", ""))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if enqueued == nil {
		t.Fatal("expected run to be enqueued")
	}
	if enqueued.JobID != originalRun.JobID {
		t.Fatalf("expected JobID %q, got %q", originalRun.JobID, enqueued.JobID)
	}
	if enqueued.ProjectID != originalRun.ProjectID {
		t.Fatalf("expected ProjectID %q, got %q", originalRun.ProjectID, enqueued.ProjectID)
	}
	if enqueued.Attempt != 1 {
		t.Fatalf("expected attempt 1, got %d", enqueued.Attempt)
	}
	if string(enqueued.Payload) != string(originalRun.Payload) {
		t.Fatalf("expected payload %s, got %s", string(originalRun.Payload), string(enqueued.Payload))
	}
	if enqueued.IdempotencyKey != originalRun.IdempotencyKey {
		t.Fatalf("expected idempotency key %q, got %q", originalRun.IdempotencyKey, enqueued.IdempotencyKey)
	}
}

func TestHandleReplayRun_FeatureDisabled(t *testing.T) {
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			t.Fatal("GetRun should not be called when replay feature is disabled")
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-123/replay", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "run replay is not enabled") {
		t.Fatalf("expected replay-disabled error, got %s", w.Body.String())
	}
}

func TestHandleReplayRun_DisabledJob(t *testing.T) {
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-123", JobID: "job-1", Status: domain.StatusFailed}, nil
		},
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: false, TimeoutSecs: 60}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFRunReplay = true
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-123/replay", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "job is disabled") {
		t.Fatalf("expected disabled-job error, got %s", w.Body.String())
	}
}

func TestHandleReplayRun_NonReplayableStatus(t *testing.T) {
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-123", JobID: "job-1", Status: domain.StatusCompleted}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-123/replay", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTriggerJob_DryRunMode(t *testing.T) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Test",
				Slug:        "test",
				EndpointURL: "https://example.com/callback",
				Enabled:     true,
				TimeoutSecs: 300,
				MaxAttempts: 3,
			}, nil
		},
		getProjectQuotaFn: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID}, nil
		},
		countProjectQueuedRunsFn: func(_ context.Context, projectID string) (int, error) {
			return 5, nil
		},
	}
	mq := &mockQueue{}
	srv := newTestServer(t, ms, mq, nil)
	// Enable dry-run feature
	srv.config.FFDryRun = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{"dry_run": true}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp DryRunValidationResult
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp.Job == nil || resp.Job.ID != "job-123" {
		t.Fatal("expected non-nil job with id=job-123")
	}
	if resp.PayloadHash == "" {
		t.Fatal("expected non-empty payload_hash")
	}
	if resp.ExpiresAt.IsZero() {
		t.Fatal("expected non-zero expires_at")
	}
}

func TestHandleCloneJob_Success(t *testing.T) {
	sourceJob := &domain.Job{
		ID:          "job-source",
		ProjectID:   "proj-1",
		Name:        "Original Job",
		Slug:        "original-job",
		Description: "A test job",
		EndpointURL: "https://example.com/hook",
		MaxAttempts: 5,
		TimeoutSecs: 120,
		RunTTLSecs:  3600,
		Enabled:     true,
	}
	ms := &mockAPIStore{}
	ms.getJobFn = func(_ context.Context, id string) (*domain.Job, error) {
		if id == "job-source" {
			return sourceJob, nil
		}
		return nil, store.ErrJobNotFound
	}
	ms.createJobFn = func(_ context.Context, job *domain.Job) error {
		job.ID = "job-cloned"
		return nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-source/clone", `{"name":"Cloned Job","slug":"cloned-job"}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
	var cloned domain.Job
	if err := json.Unmarshal(w.Body.Bytes(), &cloned); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cloned.Name != "Cloned Job" {
		t.Fatalf("name = %q, want %q", cloned.Name, "Cloned Job")
	}
	if cloned.Slug != "cloned-job" {
		t.Fatalf("slug = %q, want %q", cloned.Slug, "cloned-job")
	}
	if cloned.EndpointURL != sourceJob.EndpointURL {
		t.Fatalf("endpoint_url = %q, want %q", cloned.EndpointURL, sourceJob.EndpointURL)
	}
	if cloned.MaxAttempts != sourceJob.MaxAttempts {
		t.Fatalf("max_attempts = %d, want %d", cloned.MaxAttempts, sourceJob.MaxAttempts)
	}
	if cloned.TimeoutSecs != sourceJob.TimeoutSecs {
		t.Fatalf("timeout_secs = %d, want %d", cloned.TimeoutSecs, sourceJob.TimeoutSecs)
	}
	if cloned.RunTTLSecs != sourceJob.RunTTLSecs {
		t.Fatalf("run_ttl_secs = %d, want %d", cloned.RunTTLSecs, sourceJob.RunTTLSecs)
	}
	if cloned.ProjectID != sourceJob.ProjectID {
		t.Fatalf("project_id = %q, want %q", cloned.ProjectID, sourceJob.ProjectID)
	}
}

func TestHandleCloneJob_NotFound(t *testing.T) {
	ms := &mockAPIStore{}
	ms.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return nil, store.ErrJobNotFound
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/nonexistent/clone", `{"name":"Clone","slug":"clone"}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleCloneJob_MissingFields(t *testing.T) {
	ms := &mockAPIStore{}
	ms.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return &domain.Job{ID: "job-1", ProjectID: "proj-1", EndpointURL: "https://example.com"}, nil
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/clone", `{"name":""}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
