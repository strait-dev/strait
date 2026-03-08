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
	"orchestrator/internal/testutil"
)

// decodePaginatedList decodes a PaginatedResponse body into the given slice pointer.
func decodePaginatedList(t testing.TB, body []byte, out any) {
	t.Helper()
	var envelope struct {
		Data       json.RawMessage `json:"data"`
		HasMore    bool            `json:"has_more"`
		NextCursor *string         `json:"next_cursor,omitempty"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("invalid paginated JSON: %v\nbody: %s", err, string(body))
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		t.Fatalf("invalid data array JSON: %v", err)
	}
}

func newTestServer(t *testing.T, s APIStore, q *mockQueue, pub *mockPublisher) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret: "test-secret",
		JWTSigningKey:  "01234567890123456789012345678901",
	}
	var p pubsub.Publisher
	if pub != nil {
		p = pub
	}
	return NewServer(ServerDeps{
		Config: cfg,
		Store:  s,
		Queue:  q,
		PubSub: p,
	})
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
	return NewServer(ServerDeps{
		Config: cfg,
		Store:  s,
		Queue:  q,
		PubSub: p,
		Pinger: pinger,
	})
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", `{}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateJob_TagsFeatureDisabled(t *testing.T) {
	t.Parallel()
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

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "job tags feature is not enabled") {
		t.Fatalf("expected disabled-tags error, got %s", w.Body.String())
	}
}

func TestHandleCreateJob_ValidateTagsTooMany(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	ms := &mockAPIStore{
		listJobsFn: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.Job, error) {
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
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	if len(resp) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(resp))
	}
}

func TestHandleListJobs_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateJobGroup_Success(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	srv.config.FFJobGroups = true
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/job-groups/", `{}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateJobGroup_FeatureDisabled(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/job-groups/", `{"project_id":"proj-1","name":"Core","slug":"core"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetJobGroup_Success(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	ms := &mockAPIStore{
		listJobGroupsFn: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.JobGroup, error) {
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
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	if len(resp) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(resp))
	}
}

func TestHandleDeleteJobGroup_Success(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	ms := &mockAPIStore{
		listJobsByGroupFn: func(_ context.Context, groupID string, _ int, _ *time.Time) ([]domain.Job, error) {
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
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	if len(resp) != 1 {
		t.Fatalf("expected 1 job, got %d", len(resp))
	}
}

func TestHandleListJobs_FilterByTag(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		listJobsByTagFn: func(_ context.Context, projectID, tagKey, tagValue string, _ int, _ *time.Time) ([]domain.Job, error) {
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
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	if len(resp) != 1 {
		t.Fatalf("expected 1 job, got %d", len(resp))
	}
}

func TestHandleListJobs_FilterByTag_FeatureDisabled(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/?project_id=proj-1&tag_key=team&tag_value=core", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "job tags feature is not enabled") {
		t.Fatalf("expected disabled-tags error, got %s", w.Body.String())
	}
}

func TestHandleCreateJobDependency_Success(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/dependencies", `{"depends_on_job_id":"job-2"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleListJobDependencies_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{}
	ms.listJobDependenciesFn = func(_ context.Context, jobID string, _ int, _ *time.Time) ([]domain.JobDependency, error) {
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
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	if len(resp) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(resp))
	}
}

func TestHandleDeleteJobDependency_Success(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
		listChildRunsFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
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
	t.Parallel()
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

func TestHandleUpdateJob_ValidEndpointURL(t *testing.T) {
	t.Parallel()
	var updated bool
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
		updateJobFn: func(_ context.Context, _ *domain.Job) error {
			updated = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", `{"endpoint_url": "https://new.example.com/callback"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !updated {
		t.Fatal("UpdateJob was not called")
	}
}

func TestHandleUpdateJob_SSRFBlocksPrivateIP(t *testing.T) {
	t.Parallel()
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

	tests := []struct {
		name string
		url  string
	}{
		{"loopback IPv4", "http://127.0.0.1/callback"},
		{"private 10.x", "http://10.0.0.1/callback"},
		{"private 192.168.x", "http://192.168.1.1/callback"},
		{"private 172.16.x", "http://172.16.0.1/callback"},
		{"loopback IPv6", "http://[::1]/callback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			body := `{"endpoint_url": "` + tt.url + `"}`
			srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", body))

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d: %s", tt.url, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleUpdateJob_SSRFBlocksLocalhost(t *testing.T) {
	t.Parallel()
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

	// Note: localhost resolves to IP at validation time, which may or may not
	// be blocked depending on the validateURL implementation. The function
	// only blocks when net.ParseIP succeeds, so hostname "localhost" passes.
	// This test verifies the direct IP blocking works.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", `{"endpoint_url": "http://127.0.0.1:8080/callback"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for loopback IP, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateJob_InvalidURL(t *testing.T) {
	t.Parallel()
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

	tests := []struct {
		name string
		url  string
	}{
		{"no scheme", "example.com/callback"},
		{"ftp scheme", "ftp://example.com/callback"},
		{"empty string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			body := `{"endpoint_url": "` + tt.url + `"}`
			srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", body))

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %q, got %d: %s", tt.url, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleUpdateJob_NilEndpointURL_SkipsValidation(t *testing.T) {
	t.Parallel()
	var updated bool
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
		updateJobFn: func(_ context.Context, _ *domain.Job) error {
			updated = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// PATCH without endpoint_url should skip URL validation entirely
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", `{"name": "Updated Name"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !updated {
		t.Fatal("UpdateJob was not called")
	}
}

func TestHandleUpdateJob_ValidateTagsValueTooLong(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			t.Fatal("GetRun should not be called when replay feature is disabled")
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-123/replay", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "run replay is not enabled") {
		t.Fatalf("expected replay-disabled error, got %s", w.Body.String())
	}
}

func TestHandleReplayRun_DisabledJob(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-123", JobID: "job-1", Status: domain.StatusCompleted}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFRunReplay = true
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-123/replay", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListDeadLetterRuns_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		listDeadLetterRunsFn: func(_ context.Context, projectID string, limit int, _ *time.Time) ([]domain.JobRun, error) {
			if projectID != "proj-1" {
				t.Fatalf("unexpected project_id: %s", projectID)
			}
			if limit != 26 { // handler passes limit+1 for has_more detection
				t.Fatalf("unexpected limit: %d", limit)
			}
			return []domain.JobRun{{ID: "run-dlq-1", ProjectID: "proj-1", Status: domain.StatusDeadLetter}}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFRunDLQ = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/dlq?project_id=proj-1&limit=25", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var runs []domain.JobRun
	decodePaginatedList(t, w.Body.Bytes(), &runs)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Status != domain.StatusDeadLetter {
		t.Fatalf("expected dead_letter, got %s", runs[0].Status)
	}
}

func TestHandleListDeadLetterRuns_FeatureDisabled(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		listDeadLetterRunsFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			t.Fatal("ListDeadLetterRuns should not be called when FFRunDLQ is disabled")
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/dlq?project_id=proj-1", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReplayDeadLetterRun_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		replayDeadLetterRunFn: func(_ context.Context, runID string) (*domain.JobRun, error) {
			if runID != "run-123" {
				t.Fatalf("unexpected runID: %s", runID)
			}
			return &domain.JobRun{ID: runID, Status: domain.StatusQueued, Attempt: 1}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFRunDLQ = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-123/dlq-replay", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var run domain.JobRun
	if err := json.Unmarshal(w.Body.Bytes(), &run); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if run.Status != domain.StatusQueued {
		t.Fatalf("expected queued status, got %s", run.Status)
	}
}

func TestHandleReplayDeadLetterRun_NotDeadLetter(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		replayDeadLetterRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, fmt.Errorf("run run-123 is not dead_letter")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFRunDLQ = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-123/dlq-replay", ""))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTriggerJob_DryRunMode(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	testutil.AssertEqual(t, domain.Job{
		Name:        cloned.Name,
		Slug:        cloned.Slug,
		EndpointURL: cloned.EndpointURL,
		MaxAttempts: cloned.MaxAttempts,
		TimeoutSecs: cloned.TimeoutSecs,
		RunTTLSecs:  cloned.RunTTLSecs,
		ProjectID:   cloned.ProjectID,
	}, domain.Job{
		Name:        "Cloned Job",
		Slug:        "cloned-job",
		EndpointURL: sourceJob.EndpointURL,
		MaxAttempts: sourceJob.MaxAttempts,
		TimeoutSecs: sourceJob.TimeoutSecs,
		RunTTLSecs:  sourceJob.RunTTLSecs,
		ProjectID:   sourceJob.ProjectID,
	})
}

func TestHandleCloneJob_NotFound(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	srv.config.FFBatchJobOps = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch", `{"jobs":[]}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleBatchCreateJobs_FeatureDisabled(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	// FFBatchJobOps defaults to false

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch", `{"jobs":[{"project_id":"p","name":"n","slug":"s","endpoint_url":"https://example.com"}]}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if !strings.Contains(w.Body.String(), "batch job operations feature is not enabled") {
		t.Fatalf("expected feature-disabled error, got %s", w.Body.String())
	}
}

func TestHandleBatchCreateJobs_TooManyJobs(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch-enable", `{"ids":["job-1"]}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if !strings.Contains(w.Body.String(), "batch job operations feature is not enabled") {
		t.Fatalf("expected feature-disabled error, got %s", w.Body.String())
	}
}

func TestHandleBatchDisableJobs_TooManyIDs(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-123/health", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if !strings.Contains(w.Body.String(), "job health scoring feature is not enabled") {
		t.Fatalf("expected feature-disabled error, got %s", w.Body.String())
	}
}

func TestHandleCreateEnvironment_Success(t *testing.T) {
	t.Parallel()
	var created atomic.Bool
	ms := &mockAPIStore{
		createEnvironmentFn: func(_ context.Context, env *domain.Environment) error {
			created.Store(true)
			env.ID = "env-123"
			env.CreatedAt = time.Now()
			env.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFEnvironments = true

	body := `{
		"project_id": "proj-1",
		"name": "Development",
		"slug": "dev",
		"variables": {"LOG_LEVEL":"debug"}
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/environments/", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !created.Load() {
		t.Fatal("CreateEnvironment was not called")
	}
}

func TestHandleCreateEnvironment_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	srv.config.FFEnvironments = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/environments/", `{}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateEnvironment_FeatureDisabled(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/environments/", `{"project_id":"proj-1","name":"Development","slug":"dev"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetEnvironment_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getEnvironmentFn: func(_ context.Context, id string) (*domain.Environment, error) {
			return &domain.Environment{
				ID:        id,
				ProjectID: "proj-1",
				Name:      "Development",
				Slug:      "dev",
				Variables: map[string]string{"LOG_LEVEL": "debug"},
			}, nil
		},
		getResolvedEnvVarsFn: func(_ context.Context, id string) (map[string]string, error) {
			if id != "env-1" {
				t.Fatalf("unexpected environment id: %s", id)
			}
			return map[string]string{"LOG_LEVEL": "debug", "REGION": "us-east-1"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFEnvironments = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/environments/env-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["id"] != "env-1" {
		t.Fatalf("expected id=env-1, got %v", resp["id"])
	}
	resolved, ok := resp["resolved_variables"].(map[string]any)
	if !ok {
		t.Fatalf("expected resolved_variables object, got %T", resp["resolved_variables"])
	}
	if resolved["REGION"] != "us-east-1" {
		t.Fatalf("expected resolved REGION, got %v", resolved["REGION"])
	}
}

func TestHandleListEnvironments_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		listEnvironmentsFn: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.Environment, error) {
			return []domain.Environment{
				{ID: "env-1", ProjectID: projectID, Name: "Development", Slug: "dev"},
				{ID: "env-2", ProjectID: projectID, Name: "Production", Slug: "production"},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFEnvironments = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/environments/?project_id=proj-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	if len(resp) != 2 {
		t.Fatalf("expected 2 environments, got %d", len(resp))
	}
}

func TestHandleGetResolvedVariables_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getResolvedEnvVarsFn: func(_ context.Context, id string) (map[string]string, error) {
			if id != "env-1" {
				t.Fatalf("unexpected environment id: %s", id)
			}
			return map[string]string{"API_URL": "https://api.example.com", "LOG_LEVEL": "debug"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFEnvironments = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/environments/env-1/variables", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["variables"]["API_URL"] != "https://api.example.com" {
		t.Fatalf("expected API_URL variable, got %v", resp["variables"]["API_URL"])
	}
}

// Phase C: Execution Replay/Debug tests.

func TestHandleGetDebugBundle_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getDebugBundleFn: func(_ context.Context, runID string) (*domain.DebugBundle, error) {
			if runID != "run-1" {
				t.Fatalf("unexpected runID: %s", runID)
			}
			return &domain.DebugBundle{
				Run:         &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted},
				Events:      []domain.RunEvent{{ID: "evt-1", RunID: "run-1", Message: "started"}},
				Checkpoints: []domain.RunCheckpoint{{ID: "cp-1", RunID: "run-1", Sequence: 1}},
				Usage:       []domain.RunUsage{},
				ToolCalls:   []domain.RunToolCall{},
				Outputs:     []domain.RunOutput{},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFDebugBundle = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/debug-bundle", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var bundle domain.DebugBundle
	if err := json.Unmarshal(w.Body.Bytes(), &bundle); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if bundle.Run.ID != "run-1" {
		t.Fatalf("expected run-1, got %s", bundle.Run.ID)
	}
	if len(bundle.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bundle.Events))
	}
	if len(bundle.Checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(bundle.Checkpoints))
	}
}

func TestHandleGetDebugBundle_FeatureDisabled(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getDebugBundleFn: func(_ context.Context, _ string) (*domain.DebugBundle, error) {
			t.Fatal("GetDebugBundle should not be called when FFDebugBundle is disabled")
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/debug-bundle", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetDebugBundle_RunNotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getDebugBundleFn: func(_ context.Context, _ string) (*domain.DebugBundle, error) {
			return nil, store.ErrRunNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFDebugBundle = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/debug-bundle", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSetDebugMode_Success(t *testing.T) {
	t.Parallel()
	var calledRunID string
	var calledDebugMode bool
	ms := &mockAPIStore{
		updateRunDebugModeFn: func(_ context.Context, runID string, debugMode bool) error {
			calledRunID = runID
			calledDebugMode = debugMode
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFDebugBundle = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/debug", `{"debug_mode": true}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if calledRunID != "run-1" {
		t.Fatalf("expected run-1, got %s", calledRunID)
	}
	if !calledDebugMode {
		t.Fatal("expected debug_mode to be true")
	}
}

func TestHandleSetDebugMode_FeatureDisabled(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		updateRunDebugModeFn: func(_ context.Context, _ string, _ bool) error {
			t.Fatal("UpdateRunDebugMode should not be called when FFDebugBundle is disabled")
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/debug", `{"debug_mode": true}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSetDebugMode_RunNotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		updateRunDebugModeFn: func(_ context.Context, _ string, _ bool) error {
			return store.ErrRunNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFDebugBundle = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/debug", `{"debug_mode": true}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReplayRun_WithCheckpoint(t *testing.T) {
	t.Parallel()
	var enqueuedRun *domain.JobRun
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        "run-1",
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusFailed,
				Payload:   json.RawMessage(`{"original":true}`),
			}, nil
		},
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: true, TimeoutSecs: 30}, nil
		},
		listRunCheckpointsFn: func(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.RunCheckpoint, error) {
			if runID != "run-1" {
				t.Fatalf("unexpected runID: %s", runID)
			}
			return []domain.RunCheckpoint{
				{ID: "cp-1", RunID: "run-1", Sequence: 1, State: json.RawMessage(`{"step":1}`)},
				{ID: "cp-2", RunID: "run-1", Sequence: 2, State: json.RawMessage(`{"step":2}`)},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedRun = run
			return nil
		},
	}, nil)
	srv.config.FFRunReplay = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/replay?from_checkpoint=2", ""))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if enqueuedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	if string(enqueuedRun.Payload) != `{"step":2}` {
		t.Fatalf("expected checkpoint state as payload, got %s", string(enqueuedRun.Payload))
	}
	if !enqueuedRun.DebugMode {
		t.Fatal("expected debug_mode to be true for checkpoint replay")
	}
}

func TestHandleReplayRun_WithCheckpoint_NotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        "run-1",
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusFailed,
			}, nil
		},
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: true, TimeoutSecs: 30}, nil
		},
		listRunCheckpointsFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.RunCheckpoint, error) {
			return []domain.RunCheckpoint{
				{ID: "cp-1", RunID: "run-1", Sequence: 1, State: json.RawMessage(`{"step":1}`)},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFRunReplay = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/replay?from_checkpoint=99", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReplayRun_InvalidCheckpointParam(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:     "run-1",
				JobID:  "job-1",
				Status: domain.StatusFailed,
			}, nil
		},
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: true, TimeoutSecs: 30}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFRunReplay = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/replay?from_checkpoint=abc", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRunLineage_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		listRunLineageFn: func(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			if runID != "run-1" {
				t.Fatalf("expected run-1, got %s", runID)
			}
			return []domain.JobRun{
				{ID: "run-root", LineageDepth: 0},
				{ID: "run-1", ContinuationOf: "run-root", LineageDepth: 1},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFRunContinuation = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/lineage", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var runs []domain.JobRun
	decodePaginatedList(t, w.Body.Bytes(), &runs)
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	if runs[0].ID != "run-root" {
		t.Fatalf("expected first run to be root, got %s", runs[0].ID)
	}
}

func TestHandleListRunLineage_FeatureDisabled(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	srv.config.FFRunContinuation = false

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/lineage", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRunLineage_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		listRunLineageFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFRunContinuation = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/lineage", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}
