package api

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestHandleCreateJobGroup_Success(t *testing.T) {
	var created atomic.Bool
	ms := &mockAPIStore{
		createJobGroupFn: func(_ context.Context, group *domain.JobGroup) error {
			created.Store(true)
			group.ID = "group-123"
			group.CreatedAt = time.Now()
			group.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobGroups = true

	body := `{
		"project_id": "proj-1",
		"name": "Core Jobs",
		"slug": "core-jobs"
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/job-groups/", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !created.Load() {
		t.Fatal("CreateJobGroup was not called")
	}
}

func TestHandleCreateJobGroup_MissingFields(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	srv.config.FFJobGroups = true
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/job-groups/", `{}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateJobGroup_FeatureDisabled(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/job-groups/", `{"project_id":"proj-1","name":"Core","slug":"core"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetJobGroup_Success(t *testing.T) {
	ms := &mockAPIStore{
		getJobGroupFn: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "proj-1", Name: "Core", Slug: "core"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobGroups = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/job-groups/group-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetJobGroup_NotFound(t *testing.T) {
	ms := &mockAPIStore{
		getJobGroupFn: func(_ context.Context, _ string) (*domain.JobGroup, error) {
			return nil, store.ErrJobGroupNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobGroups = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/job-groups/missing", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleListJobGroups_Success(t *testing.T) {
	ms := &mockAPIStore{
		listJobGroupsFn: func(_ context.Context, projectID string) ([]domain.JobGroup, error) {
			return []domain.JobGroup{
				{ID: "group-1", ProjectID: projectID, Name: "Core", Slug: "core"},
				{ID: "group-2", ProjectID: projectID, Name: "Ops", Slug: "ops"},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobGroups = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/job-groups/?project_id=proj-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(resp))
	}
}

func TestHandleDeleteJobGroup_Success(t *testing.T) {
	var deletedID string
	ms := &mockAPIStore{
		deleteJobGroupFn: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobGroups = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/job-groups/group-123", ""))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if deletedID != "group-123" {
		t.Fatalf("expected group id group-123, got %q", deletedID)
	}
}

func TestHandleListJobsByGroup_Success(t *testing.T) {
	ms := &mockAPIStore{
		listJobsByGroupFn: func(_ context.Context, groupID string) ([]domain.Job, error) {
			return []domain.Job{{ID: "job-1", GroupID: groupID, ProjectID: "proj-1", Name: "Job 1"}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobGroups = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/job-groups/group-1/jobs", ""))

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

func TestHandleCreateJobDependency_Success(t *testing.T) {
	ms := &mockAPIStore{}
	ms.getJobFn = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true}, nil
	}
	ms.createJobDependencyFn = func(_ context.Context, dep *domain.JobDependency) error {
		if dep.JobID != "job-1" {
			t.Fatalf("job_id = %q, want %q", dep.JobID, "job-1")
		}
		if dep.DependsOnJobID != "job-2" {
			t.Fatalf("depends_on_job_id = %q, want %q", dep.DependsOnJobID, "job-2")
		}
		if dep.Condition != "completed" {
			t.Fatalf("condition = %q, want %q", dep.Condition, "completed")
		}
		dep.ID = "dep-1"
		dep.CreatedAt = time.Now()
		return nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobDependencies = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/dependencies", `{"depends_on_job_id":"job-2"}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp domain.JobDependency
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.ID != "dep-1" {
		t.Fatalf("id = %q, want %q", resp.ID, "dep-1")
	}
}

func TestHandleCreateJobDependency_SelfReference(t *testing.T) {
	ms := &mockAPIStore{}
	ms.getJobFn = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true}, nil
	}
	ms.createJobDependencyFn = func(_ context.Context, _ *domain.JobDependency) error {
		t.Fatal("CreateJobDependency should not be called")
		return nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobDependencies = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/dependencies", `{"depends_on_job_id":"job-1"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateJobDependency_MissingFields(t *testing.T) {
	ms := &mockAPIStore{}
	ms.getJobFn = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true}, nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobDependencies = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/dependencies", `{}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateJobDependency_FeatureDisabled(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/dependencies", `{"depends_on_job_id":"job-2"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleListJobDependencies_Success(t *testing.T) {
	ms := &mockAPIStore{}
	ms.listJobDependenciesFn = func(_ context.Context, jobID string) ([]domain.JobDependency, error) {
		return []domain.JobDependency{{ID: "dep-1", JobID: jobID, DependsOnJobID: "job-2", Condition: "completed", CreatedAt: time.Now()}}, nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobDependencies = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-1/dependencies", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []domain.JobDependency
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(resp))
	}
}

func TestHandleDeleteJobDependency_Success(t *testing.T) {
	ms := &mockAPIStore{}
	ms.getJobFn = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true}, nil
	}
	deletedID := ""
	ms.deleteJobDependencyFn = func(_ context.Context, id string) error {
		deletedID = id
		return nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobDependencies = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/jobs/job-1/dependencies/dep-9", ""))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if deletedID != "dep-9" {
		t.Fatalf("deleted id = %q, want %q", deletedID, "dep-9")
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

// Batch Job Definition Ops (2.38).

func TestHandleBatchCreateJobs_Success(t *testing.T) {
	createdCount := 0
	ms := &mockAPIStore{
		createJobFn: func(_ context.Context, job *domain.Job) error {
			createdCount++
			job.ID = fmt.Sprintf("job-%d", createdCount)
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFBatchJobOps = true

	body := `{"jobs":[
		{"project_id":"proj-1","name":"Job A","slug":"job-a","endpoint_url":"https://example.com/a"},
		{"project_id":"proj-1","name":"Job B","slug":"job-b","endpoint_url":"https://example.com/b"}
	]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
	if createdCount != 2 {
		t.Fatalf("expected 2 jobs created, got %d", createdCount)
	}

	var resp BatchCreateJobsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Created) != 2 {
		t.Fatalf("expected 2 created, got %d", len(resp.Created))
	}
	if len(resp.Errors) != 0 {
		t.Fatalf("expected 0 errors, got %d", len(resp.Errors))
	}
}

func TestHandleBatchCreateJobs_PartialFailure(t *testing.T) {
	ms := &mockAPIStore{
		createJobFn: func(_ context.Context, job *domain.Job) error {
			job.ID = "job-ok"
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFBatchJobOps = true

	// First job valid, second missing required fields
	body := `{"jobs":[
		{"project_id":"proj-1","name":"Job A","slug":"job-a","endpoint_url":"https://example.com/a"},
		{"project_id":"","name":"","slug":"","endpoint_url":""}
	]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp BatchCreateJobsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Created) != 1 {
		t.Fatalf("expected 1 created, got %d", len(resp.Created))
	}
	if len(resp.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(resp.Errors))
	}
	if resp.Errors[0].Index != 1 {
		t.Fatalf("expected error at index 1, got %d", resp.Errors[0].Index)
	}
}

func TestHandleBatchCreateJobs_AllFail(t *testing.T) {
	ms := &mockAPIStore{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFBatchJobOps = true

	body := `{"jobs":[
		{"project_id":"","name":"","slug":"","endpoint_url":""},
		{"project_id":"","name":"","slug":"","endpoint_url":""}
	]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleBatchCreateJobs_EmptyArray(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	srv.config.FFBatchJobOps = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch", `{"jobs":[]}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleBatchCreateJobs_FeatureDisabled(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	// FFBatchJobOps defaults to false

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch", `{"jobs":[{"project_id":"p","name":"n","slug":"s","endpoint_url":"https://example.com"}]}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "batch job operations feature is not enabled") {
		t.Fatalf("expected feature-disabled error, got %s", w.Body.String())
	}
}

func TestHandleBatchCreateJobs_TooManyJobs(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	srv.config.FFBatchJobOps = true

	jobs := make([]map[string]string, 51)
	for i := range jobs {
		jobs[i] = map[string]string{
			"project_id":   "proj-1",
			"name":         fmt.Sprintf("Job %d", i),
			"slug":         fmt.Sprintf("job-%d", i),
			"endpoint_url": "https://example.com/hook",
		}
	}
	body, _ := json.Marshal(map[string]any{"jobs": jobs})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch", string(body)))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "too many jobs in batch") {
		t.Fatalf("expected too-many error, got %s", w.Body.String())
	}
}

func TestHandleBatchEnableJobs_Success(t *testing.T) {
	var capturedEnabled bool
	var capturedIDs []string
	ms := &mockAPIStore{
		batchUpdateJobsEnabledFn: func(_ context.Context, ids []string, enabled bool) (int64, error) {
			capturedIDs = ids
			capturedEnabled = enabled
			return int64(len(ids)), nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFBatchJobOps = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch-enable", `{"ids":["job-1","job-2","job-3"]}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !capturedEnabled {
		t.Fatal("expected enabled=true")
	}
	if len(capturedIDs) != 3 {
		t.Fatalf("expected 3 ids, got %d", len(capturedIDs))
	}

	var resp BatchUpdateResult
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Updated != 3 {
		t.Fatalf("expected updated=3, got %d", resp.Updated)
	}
}

func TestHandleBatchDisableJobs_Success(t *testing.T) {
	var capturedEnabled bool
	ms := &mockAPIStore{
		batchUpdateJobsEnabledFn: func(_ context.Context, ids []string, enabled bool) (int64, error) {
			capturedEnabled = enabled
			return int64(len(ids)), nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFBatchJobOps = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch-disable", `{"ids":["job-1","job-2"]}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if capturedEnabled {
		t.Fatal("expected enabled=false")
	}

	var resp BatchUpdateResult
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Updated != 2 {
		t.Fatalf("expected updated=2, got %d", resp.Updated)
	}
}

func TestHandleBatchEnableJobs_EmptyIDs(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	srv.config.FFBatchJobOps = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch-enable", `{"ids":[]}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "ids array is required") {
		t.Fatalf("expected empty-ids error, got %s", w.Body.String())
	}
}

func TestHandleBatchEnableJobs_FeatureDisabled(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch-enable", `{"ids":["job-1"]}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "batch job operations feature is not enabled") {
		t.Fatalf("expected feature-disabled error, got %s", w.Body.String())
	}
}

func TestHandleBatchDisableJobs_TooManyIDs(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	srv.config.FFBatchJobOps = true

	ids := make([]string, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("job-%d", i)
	}
	body, _ := json.Marshal(map[string]any{"ids": ids})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch-disable", string(body)))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "too many ids in batch") {
		t.Fatalf("expected too-many error, got %s", w.Body.String())
	}
}

// Job Health Scoring (2.41).

func TestHandleGetJobHealth_Success(t *testing.T) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Name: "Test"}, nil
		},
		getJobHealthStatsFn: func(_ context.Context, jobID string, since time.Time) (*store.JobHealthStats, error) {
			return &store.JobHealthStats{
				TotalRuns:       100,
				CompletedRuns:   85,
				FailedRuns:      10,
				TimedOutRuns:    3,
				CrashedRuns:     2,
				CanceledRuns:    0,
				SuccessRate:     85.0,
				AvgDurationSecs: 5.5,
				P95DurationSecs: 12.3,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobHealthScoring = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-123/health?window=7d", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp JobHealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.JobID != "job-123" {
		t.Fatalf("job_id = %q, want %q", resp.JobID, "job-123")
	}
	if resp.Window != "7d" {
		t.Fatalf("window = %q, want %q", resp.Window, "7d")
	}
	if resp.TotalRuns != 100 {
		t.Fatalf("total_runs = %d, want %d", resp.TotalRuns, 100)
	}
	if resp.SuccessRate != 85.0 {
		t.Fatalf("success_rate = %f, want %f", resp.SuccessRate, 85.0)
	}
}

func TestHandleGetJobHealth_DefaultWindow(t *testing.T) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*store.JobHealthStats, error) {
			return &store.JobHealthStats{TotalRuns: 10, CompletedRuns: 10, SuccessRate: 100.0}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobHealthScoring = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-123/health", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp JobHealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Window != "7d" {
		t.Fatalf("window = %q, want %q (default)", resp.Window, "7d")
	}
}

func TestHandleGetJobHealth_InvalidWindow(t *testing.T) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobHealthScoring = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-123/health?window=2w", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid window") {
		t.Fatalf("expected invalid-window error, got %s", w.Body.String())
	}
}

func TestHandleGetJobHealth_NotFound(t *testing.T) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobHealthScoring = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/missing/health", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleGetJobHealth_FeatureDisabled(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-123/health", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "job health scoring feature is not enabled") {
		t.Fatalf("expected feature-disabled error, got %s", w.Body.String())
	}
}
