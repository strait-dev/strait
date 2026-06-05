package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/store"
	"strait/internal/testutil"
)

// decodePaginatedList decodes a PaginatedResponse body into the given slice pointer.
func decodePaginatedList(tb testing.TB, body []byte, out any) {
	tb.Helper()
	var envelope struct {
		Data       json.RawMessage `json:"data"`
		HasMore    bool            `json:"has_more"`
		NextCursor *string         `json:"next_cursor,omitempty"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		tb.Fatalf("invalid paginated JSON: %v\nbody: %s", err, string(body))
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		tb.Fatalf("invalid data array JSON: %v", err)
	}
}

func newTestServer(t *testing.T, s APIStore, q *mockQueue, pub *mockPublisher) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	var p pubsub.Publisher
	if pub != nil {
		p = pub
	}
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   s,
		Queue:   q,
		PubSub:  p,
		Edition: domain.EditionCommunity,
	})
	t.Cleanup(srv.Close)
	return srv
}

func newTestServerWithPinger(t *testing.T, s APIStore, q *mockQueue, pub *mockPublisher, pinger Pinger) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	var p pubsub.Publisher
	if pub != nil {
		p = pub
	}
	srv := NewServer(ServerDeps{
		Config: cfg,
		Store:  s,
		Queue:  q,
		PubSub: p,
		Pinger: pinger,
	})
	t.Cleanup(srv.Close)
	return srv
}

func authedRequest(method, path string, body string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Header.Set("X-Internal-Secret", "test-secret-value")
	r.Header.Set("Content-Type", "application/json")
	return r
}

func authedProjectRequest(method, path, body, projectID string) *http.Request {
	r := authedRequest(method, path, body)
	if projectID != "" {
		r.Header.Set("X-Project-Id", projectID)
	}
	return r
}

func TestHandleHealth(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("expected status=ok, got %v", resp["status"])
	}
}

func TestHandleHealth_PublicResponseFields(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   &APIStoreMock{},
		Queue:   &mockQueue{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", resp["status"])
	}
	if _, ok := resp["version"]; !ok {
		t.Error("expected version field in public response")
	}
	if _, ok := resp["timestamp"]; !ok {
		t.Error("expected timestamp field in public response")
	}
	if _, ok := resp["edition"]; ok {
		t.Error("edition should not be in public response (internal only)")
	}
}

func TestHandleAuth_MissingSecret(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
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
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
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
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", `{}`))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}

	var resp struct {
		Error struct {
			Code    string   `json:"code"`
			Message string   `json:"message"`
			Details []string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Error.Code != ErrorCodeValidationFailed {
		t.Fatalf("expected validation_failed code, got %q", resp.Error.Code)
	}
	if resp.Error.Message != "validation failed" {
		t.Fatalf("expected validation failed message, got %q", resp.Error.Message)
	}
	if len(resp.Error.Details) == 0 {
		t.Fatal("expected validation details")
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

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
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

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
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
	ms := &APIStoreMock{
		ListJobsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.Job, error) {
			return []domain.Job{
				{ID: "job-1", ProjectID: projectID, Name: "Job 1"},
				{ID: "job-2", ProjectID: projectID, Name: "Job 2"},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/jobs/", "", "proj-1"))

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
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateJobGroup_Success(t *testing.T) {
	t.Parallel()
	var created atomic.Bool
	ms := &APIStoreMock{
		CreateJobGroupFunc: func(_ context.Context, group *domain.JobGroup) error {
			created.Store(true)
			group.ID = "group-123"
			group.CreatedAt = time.Now()
			group.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/job-groups/", `{}`))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Error.Code != ErrorCodeValidationFailed {
		t.Fatalf("expected validation_failed code, got %q", resp.Error.Code)
	}
}

func TestHandleGetJobGroup_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobGroupFunc: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "proj-1", Name: "Core", Slug: "core"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/job-groups/group-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetJobGroup_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobGroupFunc: func(_ context.Context, _ string) (*domain.JobGroup, error) {
			return nil, store.ErrJobGroupNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/job-groups/missing", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleListJobGroups_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListJobGroupsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.JobGroup, error) {
			return []domain.JobGroup{
				{ID: "group-1", ProjectID: projectID, Name: "Core", Slug: "core"},
				{ID: "group-2", ProjectID: projectID, Name: "Ops", Slug: "ops"},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		GetJobGroupFunc: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "test-project"}, nil
		},
		DeleteJobGroupFunc: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		GetJobGroupFunc: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "test-project"}, nil
		},
		ListJobsByGroupFunc: func(_ context.Context, groupID string, _ int, _ *time.Time) ([]domain.Job, error) {
			return []domain.Job{{ID: "job-1", GroupID: groupID, ProjectID: "proj-1", Name: "Job 1"}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		ListJobsByTagFunc: func(_ context.Context, projectID, tagKey, tagValue string, _ int, _ *time.Time) ([]domain.Job, error) {
			if projectID != "proj-1" || tagKey != "team" || tagValue != "core" {
				t.Fatalf("unexpected list by tag args: %q %q %q", projectID, tagKey, tagValue)
			}
			return []domain.Job{{ID: "job-1", ProjectID: projectID, Name: "Job 1"}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/jobs/?tag_key=team&tag_value=core", "", "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	if len(resp) != 1 {
		t.Fatalf("expected 1 job, got %d", len(resp))
	}
}

func TestHandleCreateJobDependency_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true}, nil
	}
	ms.CreateJobDependencyFunc = func(_ context.Context, dep *domain.JobDependency) error {
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
	ms := &APIStoreMock{}
	ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true}, nil
	}
	ms.CreateJobDependencyFunc = func(_ context.Context, _ *domain.JobDependency) error {
		t.Fatal("CreateJobDependency should not be called")
		return nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/dependencies", `{"depends_on_job_id":"job-1"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateJobDependency_MissingFields(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true}, nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/dependencies", `{}`))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestHandleListJobDependencies_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
	}
	ms.ListJobDependenciesFunc = func(_ context.Context, jobID string, _ int, _ *time.Time) ([]domain.JobDependency, error) {
		return []domain.JobDependency{{ID: "dep-1", JobID: jobID, DependsOnJobID: "job-2", Condition: "completed", CreatedAt: time.Now()}}, nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

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

func TestHandleListJobDependencies_UsesCacheForFirstPage(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
	}
	var listCalls atomic.Int64
	createdAt := time.Now().UTC()
	ms.ListJobDependenciesFunc = func(_ context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobDependency, error) {
		listCalls.Add(1)
		if limit != 51 {
			t.Fatalf("limit = %d, want 51", limit)
		}
		if cursor != nil {
			t.Fatalf("cursor = %v, want nil for cached first page", cursor)
		}
		return []domain.JobDependency{{ID: "dep-1", JobID: jobID, DependsOnJobID: "job-2", Condition: "completed", CreatedAt: createdAt, CacheVersion: 3}}, nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.jobDependencyCache = newJobDependencyCache(time.Minute)

	for range 2 {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-1/dependencies", ""))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	}
	if listCalls.Load() != 1 {
		t.Fatalf("ListJobDependencies calls = %d, want 1", listCalls.Load())
	}
}

func TestHandleListJobDependencies_CursorBypassesCache(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
	}
	var listCalls atomic.Int64
	createdAt := time.Now().UTC()
	ms.ListJobDependenciesFunc = func(_ context.Context, jobID string, _ int, cursor *time.Time) ([]domain.JobDependency, error) {
		listCalls.Add(1)
		if cursor == nil {
			t.Fatal("cursor = nil, want cursor for uncached page")
		}
		return []domain.JobDependency{{ID: "dep-1", JobID: jobID, DependsOnJobID: "job-2", Condition: "completed", CreatedAt: createdAt}}, nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.jobDependencyCache = newJobDependencyCache(time.Minute)
	url := "/v1/jobs/job-1/dependencies?cursor=" + createdAt.Add(time.Second).Format(time.RFC3339Nano)

	for range 2 {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, url, ""))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	}
	if listCalls.Load() != 2 {
		t.Fatalf("ListJobDependencies calls = %d, want 2", listCalls.Load())
	}
}

func TestHandleListJobDependencies_CustomLimitBypassesCache(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
	}
	var listCalls atomic.Int64
	createdAt := time.Now().UTC()
	ms.ListJobDependenciesFunc = func(_ context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobDependency, error) {
		listCalls.Add(1)
		if limit != 11 {
			t.Fatalf("limit = %d, want 11", limit)
		}
		if cursor != nil {
			t.Fatalf("cursor = %v, want nil", cursor)
		}
		return []domain.JobDependency{{ID: "dep-1", JobID: jobID, DependsOnJobID: "job-2", Condition: "completed", CreatedAt: createdAt, CacheVersion: 3}}, nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.jobDependencyCache = newJobDependencyCache(time.Minute)

	for range 2 {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-1/dependencies?limit=10", ""))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	}
	if listCalls.Load() != 2 {
		t.Fatalf("ListJobDependencies calls = %d, want 2", listCalls.Load())
	}
}

func TestHandleDeleteJobDependency_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true}, nil
	}
	ms.GetJobDependencyFunc = func(_ context.Context, id string) (*domain.JobDependency, error) {
		return &domain.JobDependency{ID: id, JobID: "job-1", DependsOnJobID: "job-other", Condition: "completed"}, nil
	}
	deletedID := ""
	ms.DeleteJobDependencyFunc = func(_ context.Context, id string) error {
		deletedID = id
		return nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
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
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
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
	if _, ok := resp["run_token"]; ok {
		t.Fatal("trigger response must not expose SDK run_token")
	}
	if resp["status"] != "queued" {
		t.Fatalf("expected status=queued, got %v", resp["status"])
	}
}

func TestHandleTriggerJob_WaitsForUnsatisfiedDependencies(t *testing.T) {
	t.Parallel()

	createdRunStatus := domain.StatusQueued
	enqueueCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Dependent",
				Slug:        "dependent",
				EndpointURL: "https://example.com/callback",
				Enabled:     true,
				TimeoutSecs: 300,
				MaxAttempts: 3,
			}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return false, nil
		},
		CreateRunFunc: func(_ context.Context, run *domain.JobRun) error {
			createdRunStatus = run.Status
			return nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueueCalled = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if createdRunStatus != domain.StatusWaiting {
		t.Fatalf("created run status = %s, want waiting", createdRunStatus)
	}
	if enqueueCalled {
		t.Fatal("enqueue should not be called for waiting dependency run")
	}
}

func TestHandleTriggerJob_WaitingDependencyConflictReturnsIdempotentHit(t *testing.T) {
	t.Parallel()

	enqueueCalled := false
	lookupCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Dependent",
				Slug:        "dependent",
				EndpointURL: "https://example.com/callback",
				Enabled:     true,
				TimeoutSecs: 300,
				MaxAttempts: 3,
			}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return false, nil
		},
		CreateRunFunc: func(_ context.Context, _ *domain.JobRun) error {
			return domain.ErrIdempotencyConflict
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			lookupCalled = true
			if jobID != "job-123" || key != "same-key" {
				t.Fatalf("unexpected idempotency lookup args: %s %s", jobID, key)
			}
			return &domain.JobRun{ID: "run-existing", Status: domain.StatusWaiting}, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueueCalled = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "same-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (idempotent hit), got %d: %s", w.Code, w.Body.String())
	}
	if !lookupCalled {
		t.Fatal("expected idempotency lookup to be called")
	}
	if enqueueCalled {
		t.Fatal("enqueue should not be called for waiting dependency idempotency hit")
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["id"] != "run-existing" {
		t.Fatalf("expected existing run id, got %v", resp["id"])
	}
	if resp["status"] != string(domain.StatusWaiting) {
		t.Fatalf("expected waiting status, got %v", resp["status"])
	}
	if hit, ok := resp["idempotency_hit"].(bool); !ok || !hit {
		t.Fatalf("expected idempotency_hit=true, got %v", resp["idempotency_hit"])
	}
	if _, ok := resp["run_token"]; ok {
		t.Fatal("did not expect run_token for idempotency hit")
	}
	if _, ok := resp["payload_hash"]; ok {
		t.Fatal("did not expect payload_hash for idempotency hit")
	}
}

func TestHandleTriggerJob_WaitingDependencyConflictLookupError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Dependent",
				Slug:        "dependent",
				EndpointURL: "https://example.com/callback",
				Enabled:     true,
				TimeoutSecs: 300,
				MaxAttempts: 3,
			}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return false, nil
		},
		CreateRunFunc: func(_ context.Context, _ *domain.JobRun) error {
			return domain.ErrIdempotencyConflict
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, errors.New("lookup failed")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "same-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "internal server error") {
		t.Fatalf("expected sanitized internal error, got %s", w.Body.String())
	}
}

func TestHandleTriggerJob_QueuesWhenDependenciesSatisfied(t *testing.T) {
	t.Parallel()

	enqueueCalled := false
	createRunCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Dependent",
				Slug:        "dependent",
				EndpointURL: "https://example.com/callback",
				Enabled:     true,
				TimeoutSecs: 300,
				MaxAttempts: 3,
			}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
		CreateRunFunc: func(_ context.Context, _ *domain.JobRun) error {
			createRunCalled = true
			return nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueueCalled = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !enqueueCalled {
		t.Fatal("expected enqueue to be called")
	}
	if createRunCalled {
		t.Fatal("create run should not be called when dependencies are satisfied")
	}
}

func TestHandleTriggerJob_DisabledJob(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
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

func TestHandleGetRunDependencyStatus_Success(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", Status: domain.StatusWaiting}, nil
		},
		ListJobDependenciesFunc: func(_ context.Context, jobID string, _ int, _ *time.Time) ([]domain.JobDependency, error) {
			if jobID != "job-1" {
				t.Fatalf("jobID = %s, want job-1", jobID)
			}
			return []domain.JobDependency{{ID: "dep-1", JobID: "job-1", DependsOnJobID: "job-2", Condition: "completed"}}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, run *domain.JobRun) (bool, error) {
			if run.ID != "run-1" {
				t.Fatalf("run.ID = %s, want run-1", run.ID)
			}
			return false, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/dependency-status", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["dependencies_satisfied"] != false {
		t.Fatalf("dependencies_satisfied = %v, want false", resp["dependencies_satisfied"])
	}
}

func TestHandleStats_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		QueueStatsFunc: func(_ context.Context) (*store.QueueStats, error) {
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
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
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
		UpdateRunStatusFunc: func(_ context.Context, _ string, _ domain.RunStatus, to domain.RunStatus, _ map[string]any) error {
			if to != domain.StatusCanceled {
				t.Errorf("expected transition to canceled, got %s", to)
			}
			return nil
		},
		ListChildRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
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

func TestHandleDeleteJob_Success(t *testing.T) {
	t.Parallel()
	var deletedID string
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		DeleteJobFunc: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/jobs/job-123", ""))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if deletedID != "job-123" {
		t.Fatalf("expected DeleteJob called with job-123, got %s", deletedID)
	}
}

func TestHandleDeleteJob_ActiveRuns(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		DeleteJobFunc: func(_ context.Context, _ string) error {
			return store.ErrJobHasActiveRuns
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/jobs/job-123", ""))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateJob_ValidEndpointURL(t *testing.T) {
	t.Parallel()
	var updated bool
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Test",
				Slug:        "test",
				EndpointURL: "https://example.com",
				Enabled:     true,
			}, nil
		},
		UpdateJobFunc: func(_ context.Context, _ *domain.Job) error {
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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
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
		name       string
		url        string
		wantStatus int
	}{
		// Struct-level URL validation -> 422 validation_failed.
		{"no scheme", "example.com/callback", http.StatusUnprocessableEntity},
		{"empty string", "", http.StatusUnprocessableEntity},
		// Custom scheme check past struct validation -> 400 bad_request.
		{"ftp scheme", "ftp://example.com/callback", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			body := `{"endpoint_url": "` + tt.url + `"}`
			srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", body))

			if w.Code != tt.wantStatus {
				t.Fatalf("expected %d for %q, got %d: %s", tt.wantStatus, tt.url, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleUpdateJob_NilEndpointURL_SkipsValidation(t *testing.T) {
	t.Parallel()
	var updated bool
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Test",
				Slug:        "test",
				EndpointURL: "https://example.com",
				Enabled:     true,
			}, nil
		},
		UpdateJobFunc: func(_ context.Context, _ *domain.Job) error {
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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Name: "Test", Slug: "test", EndpointURL: "https://example.com", Enabled: true}, nil
		},
		UpdateJobFunc: func(_ context.Context, _ *domain.Job) error {
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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Name: "Test", Slug: "test", EndpointURL: "https://example.com", Enabled: true}, nil
		},
		UpdateJobFunc: func(_ context.Context, _ *domain.Job) error {
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

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			if id != "run-123" {
				t.Fatalf("unexpected run id: %s", id)
			}
			return originalRun, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
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
	// Replays should NOT carry the original idempotency key to avoid
	// conflicts with active runs sharing the same key.
	if enqueued.IdempotencyKey != "" {
		t.Fatalf("expected empty idempotency key on replay, got %q", enqueued.IdempotencyKey)
	}
}

func TestHandleReplayRun_DisabledJob(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-123", JobID: "job-1", Status: domain.StatusFailed}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: false, TimeoutSecs: 60}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
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
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
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

func TestHandleListDeadLetterRuns_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListDeadLetterRunsFunc: func(_ context.Context, projectID string, limit int, _ *time.Time) ([]domain.JobRun, error) {
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

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/dlq?limit=25", "", "proj-1"))

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

func TestHandleReplayDeadLetterRun_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusDeadLetter, ProjectID: "proj-1"}, nil
		},
		ReplayDeadLetterRunFunc: func(_ context.Context, runID string) (*domain.JobRun, error) {
			if runID != "run-123" {
				t.Fatalf("unexpected runID: %s", runID)
			}
			return &domain.JobRun{ID: runID, Status: domain.StatusQueued, Attempt: 1}, nil
		},
	}
	var enqueuedExisting []string

	srv := newTestServer(t, ms, &mockQueue{
		enqueueExistingFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedExisting = append(enqueuedExisting, run.ID)
			return nil
		},
	}, nil)

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
	if !slices.Equal(enqueuedExisting, []string{"run-123"}) {
		t.Fatalf("EnqueueExisting calls = %+v, want replayed run", enqueuedExisting)
	}
}

func TestHandleReplayDeadLetterRun_NotDeadLetter(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusFailed, ProjectID: "proj-1"}, nil
		},
		ReplayDeadLetterRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, fmt.Errorf("replay dead letter run: %w: run run-123 has status failed, expected dead_letter", store.ErrRunConflict)
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-123/dlq-replay", ""))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleBulkReplayDeadLetterRuns_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusDeadLetter, ProjectID: "proj-1"}, nil
		},
		BulkReplayDeadLetterRunsFunc: func(_ context.Context, runIDs []string, projectID string, limit int) ([]domain.JobRun, error) {
			if len(runIDs) != 2 || runIDs[0] != "run-1" || runIDs[1] != "run-2" {
				t.Fatalf("unexpected run_ids: %+v", runIDs)
			}
			if projectID != "" {
				t.Fatalf("expected empty project_id, got %q", projectID)
			}
			if limit != 0 {
				t.Fatalf("expected zero limit for run_ids mode, got %d", limit)
			}
			return []domain.JobRun{
				{ID: "run-1", Status: domain.StatusQueued, Attempt: 1},
				{ID: "run-2", Status: domain.StatusQueued, Attempt: 1},
			}, nil
		},
	}
	var enqueuedExisting []string

	srv := newTestServer(t, ms, &mockQueue{
		enqueueExistingFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedExisting = append(enqueuedExisting, run.ID)
			return nil
		},
	}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-dlq-replay", `{"run_ids":["run-1","run-2"]}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Count    int             `json:"count"`
		Replayed []domain.JobRun `json:"replayed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Count != 2 {
		t.Fatalf("expected count=2, got %d", resp.Count)
	}
	if len(resp.Replayed) != 2 {
		t.Fatalf("expected 2 replayed runs, got %d", len(resp.Replayed))
	}
	if !slices.Equal(enqueuedExisting, []string{"run-1", "run-2"}) {
		t.Fatalf("EnqueueExisting calls = %+v, want both replayed runs", enqueuedExisting)
	}
}

func TestHandleBulkReplayDeadLetterRuns_RunIDsModeDoesNotSendProjectIDOrLimit(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusDeadLetter, ProjectID: "proj-1"}, nil
		},
		BulkReplayDeadLetterRunsFunc: func(_ context.Context, runIDs []string, projectID string, limit int) ([]domain.JobRun, error) {
			if len(runIDs) != 1 || runIDs[0] != "run-1" {
				t.Fatalf("unexpected run_ids: %+v", runIDs)
			}
			if projectID != "" {
				t.Fatalf("run_ids replay must not also pass project_id, got %q", projectID)
			}
			if limit != 0 {
				t.Fatalf("run_ids replay must not also pass limit, got %d", limit)
			}
			return []domain.JobRun{{ID: "run-1", Status: domain.StatusQueued, Attempt: 1}}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-dlq-replay", `{"run_ids":["run-1"],"limit":123}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTriggerJob_DryRunMode(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
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
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID}, nil
		},
		CountProjectQueuedRunsFunc: func(_ context.Context, projectID string) (int, error) {
			return 5, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{}
	srv := newTestServer(t, ms, mq, nil)
	// Enable dry-run feature

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
	if raw := w.Body.String(); strings.Contains(raw, "endpoint_url") || strings.Contains(raw, "https://example.com/callback") {
		t.Fatalf("dry-run response leaked endpoint details: %s", raw)
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
	ms := &APIStoreMock{}
	ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
		if id == "job-source" {
			return sourceJob, nil
		}
		return nil, store.ErrJobNotFound
	}
	ms.CreateJobFunc = func(_ context.Context, job *domain.Job) error {
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
	ms := &APIStoreMock{}
	ms.GetJobFunc = func(_ context.Context, _ string) (*domain.Job, error) {
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
	ms := &APIStoreMock{}
	ms.GetJobFunc = func(_ context.Context, _ string) (*domain.Job, error) {
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
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			createdCount++
			job.ID = fmt.Sprintf("job-%d", createdCount)
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			job.ID = "job-ok"
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch", `{"jobs":[]}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleBatchCreateJobs_TooManyJobs(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

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
	var capturedProject string
	ms := &APIStoreMock{
		BatchUpdateJobsEnabledFunc: func(_ context.Context, ids []string, enabled bool, projectID string) (int64, error) {
			capturedIDs = ids
			capturedEnabled = enabled
			capturedProject = projectID
			return int64(len(ids)), nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs/batch-enable", `{"ids":["job-1","job-2","job-3"]}`, "proj-aaa"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !capturedEnabled {
		t.Fatal("expected enabled=true")
	}
	if len(capturedIDs) != 3 {
		t.Fatalf("expected 3 ids, got %d", len(capturedIDs))
	}
	if capturedProject != "proj-aaa" {
		t.Fatalf("expected projectID=proj-aaa, got %q", capturedProject)
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
	ms := &APIStoreMock{
		BatchUpdateJobsEnabledFunc: func(_ context.Context, ids []string, enabled bool, _ string) (int64, error) {
			capturedEnabled = enabled
			return int64(len(ids)), nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs/batch-disable", `{"ids":["job-1","job-2"]}`, "proj-aaa"))

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
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch-enable", `{"ids":[]}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "ids array is required") {
		t.Fatalf("expected empty-ids error, got %s", w.Body.String())
	}
}

func TestHandleBatchDisableJobs_TooManyIDs(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Name: "Test"}, nil
		},
		GetJobHealthStatsFunc: func(_ context.Context, jobID string, since time.Time) (*store.JobHealthStats, error) {
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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		GetJobHealthStatsFunc: func(_ context.Context, _ string, _ time.Time) (*store.JobHealthStats, error) {
			return &store.JobHealthStats{TotalRuns: 10, CompletedRuns: 10, SuccessRate: 100.0}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/missing/health", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleCreateEnvironment_Success(t *testing.T) {
	t.Parallel()
	var created atomic.Bool
	ms := &APIStoreMock{
		CreateEnvironmentFunc: func(_ context.Context, env *domain.Environment) error {
			created.Store(true)
			env.ID = "env-123"
			env.CreatedAt = time.Now()
			env.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/environments/", `{}`))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestHandleGetEnvironment_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{
				ID:        id,
				ProjectID: "proj-1",
				Name:      "Development",
				Slug:      "dev",
				Variables: map[string]string{"LOG_LEVEL": "debug"},
			}, nil
		},
		GetResolvedEnvironmentVariablesFunc: func(_ context.Context, id string) (map[string]string, error) {
			if id != "env-1" {
				t.Fatalf("unexpected environment id: %s", id)
			}
			return map[string]string{"LOG_LEVEL": "debug", "REGION": "us-east-1"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	resolved, ok := resp["resolved_variable_keys"].([]any)
	if !ok {
		t.Fatalf("expected resolved_variable_keys array, got %T", resp["resolved_variable_keys"])
	}
	if !slices.ContainsFunc(resolved, func(v any) bool { return v == "REGION" }) {
		t.Fatalf("expected resolved REGION key, got %v", resolved)
	}
}

func TestHandleListEnvironments_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListEnvironmentsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.Environment, error) {
			return []domain.Environment{
				{ID: "env-1", ProjectID: projectID, Name: "Development", Slug: "dev"},
				{ID: "env-2", ProjectID: projectID, Name: "Production", Slug: "production"},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "test", Slug: "test"}, nil
		},
		GetResolvedEnvironmentVariablesFunc: func(_ context.Context, id string) (map[string]string, error) {
			if id != "env-1" {
				t.Fatalf("unexpected environment id: %s", id)
			}
			return map[string]string{"API_URL": "https://api.example.com", "LOG_LEVEL": "debug"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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

func TestHandleGetDebugBundle_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetDebugBundleFunc: func(_ context.Context, runID string) (*domain.DebugBundle, error) {
			if runID != "run-1" {
				t.Fatalf("unexpected runID: %s", runID)
			}
			return &domain.DebugBundle{
				Run:         &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted},
				Events:      []domain.RunEvent{{ID: "evt-1", RunID: "run-1", Message: "started"}},
				Checkpoints: []domain.RunCheckpoint{{ID: "cp-1", RunID: "run-1", Sequence: 1}},
				Outputs:     []domain.RunOutput{},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

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

func TestHandleGetDebugBundle_RunNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetDebugBundleFunc: func(_ context.Context, _ string) (*domain.DebugBundle, error) {
			return nil, store.ErrRunNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		UpdateRunDebugModeFunc: func(_ context.Context, runID string, debugMode bool) error {
			calledRunID = runID
			calledDebugMode = debugMode
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

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

func TestHandleSetDebugMode_RunNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		UpdateRunDebugModeFunc: func(_ context.Context, _ string, _ bool) error {
			return store.ErrRunNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/debug", `{"debug_mode": true}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReplayRun_WithCheckpoint(t *testing.T) {
	t.Parallel()
	var enqueuedRun *domain.JobRun
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        "run-1",
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusFailed,
				Payload:   json.RawMessage(`{"original":true}`),
			}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: true, TimeoutSecs: 30}, nil
		},
		ListRunCheckpointsFunc: func(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.RunCheckpoint, error) {
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
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        "run-1",
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusFailed,
			}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: true, TimeoutSecs: 30}, nil
		},
		ListRunCheckpointsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.RunCheckpoint, error) {
			return []domain.RunCheckpoint{
				{ID: "cp-1", RunID: "run-1", Sequence: 1, State: json.RawMessage(`{"step":1}`)},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/replay?from_checkpoint=99", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReplayRun_InvalidCheckpointParam(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:     "run-1",
				JobID:  "job-1",
				Status: domain.StatusFailed,
			}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: true, TimeoutSecs: 30}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/replay?from_checkpoint=abc", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRunLineage_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListRunLineageFunc: func(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.JobRun, error) {
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

func TestHandleListRunLineage_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListRunLineageFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/lineage", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// mockPoolStatter is read concurrently by the background backpressure sampler
// and mutated by the test goroutine, so all fields go through atomics to keep
// -race happy. Time.Duration is stored as int64 nanoseconds.
type mockPoolStatter struct {
	acquired         atomic.Int32
	max              atomic.Int32
	emptyAcquire     atomic.Int64
	emptyAcquireWait atomic.Int64 // nanoseconds
}

func (m *mockPoolStatter) AcquiredConns() int32 { return m.acquired.Load() }
func (m *mockPoolStatter) MaxConns() int32      { return m.max.Load() }
func (m *mockPoolStatter) EmptyAcquireCount() int64 {
	return m.emptyAcquire.Load()
}
func (m *mockPoolStatter) EmptyAcquireWaitTime() time.Duration {
	return time.Duration(m.emptyAcquireWait.Load())
}

func newMockPoolStatter(acquired, max int32) *mockPoolStatter {
	m := &mockPoolStatter{}
	m.acquired.Store(acquired)
	m.max.Store(max)
	return m
}

func TestDBBackpressure_Returns503WhenPoolExhausted(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:      cfg,
		Store:       &APIStoreMock{},
		Queue:       &mockQueue{},
		PoolStatter: newMockPoolStatter(24, 25), // 96% > 90%
	})
	t.Cleanup(srv.Close)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/health", ""))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") != "1" {
		t.Fatalf("expected Retry-After=1, got %s", w.Header().Get("Retry-After"))
	}
}

func TestDBBackpressure_AllowsRequestsWhenPoolHealthy(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:      cfg,
		Store:       &APIStoreMock{},
		Queue:       &mockQueue{},
		PoolStatter: newMockPoolStatter(10, 25), // 40% < 90%
	})
	t.Cleanup(srv.Close)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	if w.Code == http.StatusServiceUnavailable {
		t.Fatal("expected request to pass through when pool is healthy")
	}
}

func TestDBBackpressure_Returns503WhenAcquireWaitSpikes(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	statter := newMockPoolStatter(2, 25)
	srv := NewServer(ServerDeps{
		Config:      cfg,
		Store:       &APIStoreMock{},
		Queue:       &mockQueue{},
		PoolStatter: statter,
	})
	t.Cleanup(srv.Close)

	// Stop the async sampler so we can drive a single deterministic sample
	// without racing the ticker; the published shedding atomic is what the
	// middleware reads, and sampleOnce updates it synchronously.
	srv.poolBackpressure.Stop()
	statter.emptyAcquire.Store(10)
	statter.emptyAcquireWait.Store(int64(time.Second)) // avg = 100ms (above 50ms threshold)
	srv.poolBackpressure.sampleOnce()

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/health", ""))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 after acquire wait spike, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") != "1" {
		t.Fatalf("expected Retry-After=1, got %s", w.Header().Get("Retry-After"))
	}
}

func TestDBBackpressure_AllowsSmallAcquireWait(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	statter := newMockPoolStatter(2, 25)
	srv := NewServer(ServerDeps{
		Config:      cfg,
		Store:       &APIStoreMock{},
		Queue:       &mockQueue{},
		PoolStatter: statter,
	})
	t.Cleanup(srv.Close)

	srv.poolBackpressure.Stop()
	statter.emptyAcquire.Store(10)
	statter.emptyAcquireWait.Store(int64(100 * time.Millisecond)) // avg = 10ms (below threshold)
	srv.poolBackpressure.sampleOnce()

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	if w.Code == http.StatusServiceUnavailable {
		t.Fatal("expected request to pass through when average acquire wait is below threshold")
	}
}

func TestHandleUpdateJob_VersionConflict_Returns409(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "test-job",
				Slug:        "test-job",
				EndpointURL: "https://example.com",
				Enabled:     true,
				TimeoutSecs: 60,
				Version:     1,
			}, nil
		},
		UpdateJobFunc: func(_ context.Context, _ *domain.Job) error {
			return store.ErrJobVersionConflict
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-1", `{"name":"updated"}`))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestValidateCronFieldCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		expr    string
		wantErr bool
	}{
		{"* * * * *", false},
		{"0 0 * * *", false},
		{"0 0 * * * *", true}, // 6 fields rejected -- parser only supports 5
		{"* * *", true},
		{"* * * * * * *", true},
		{"*", true},
	}
	for _, tt := range tests {
		err := validateCronFieldCount(tt.expr)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateCronFieldCount(%q) error=%v, wantErr=%v", tt.expr, err, tt.wantErr)
		}
	}
}

func TestMetrics_Unauthenticated_Returns401(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		InternalSecret: "test-secret-value",
		JWTSigningKey:  testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:         cfg,
		Store:          &APIStoreMock{},
		Queue:          &mockQueue{},
		Edition:        domain.EditionCloud,
		MetricsHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }),
	})
	t.Cleanup(srv.Close)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated /metrics, got %d", w.Code)
	}
}

func TestMetrics_Authenticated_Returns200(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		InternalSecret: "test-secret-value",
		JWTSigningKey:  testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:         cfg,
		Store:          &APIStoreMock{},
		Queue:          &mockQueue{},
		Edition:        domain.EditionCloud,
		MetricsHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }),
	})
	t.Cleanup(srv.Close)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r.Header.Set("X-Internal-Secret", "test-secret-value")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for authenticated /metrics, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMetrics_AuthorizationBearerInternalSecret_Returns200(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		InternalSecret: "test-secret-value",
		JWTSigningKey:  testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:         cfg,
		Store:          &APIStoreMock{},
		Queue:          &mockQueue{},
		Edition:        domain.EditionCloud,
		MetricsHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }),
	})
	t.Cleanup(srv.Close)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r.Header.Set("Authorization", "Bearer test-secret-value")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for bearer internal secret /metrics, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNullByteStrippingReader(t *testing.T) {
	t.Parallel()
	input := []byte("hello\x00world")
	reader := &nullByteStrippingReader{r: strings.NewReader(string(input))}
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "hello world"
	if string(out) != expected {
		t.Fatalf("expected %q, got %q", expected, string(out))
	}
}

func TestHandleTriggerJob_NullByteInPayload_DoesNotCrash(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)
	// JSON with null byte inside a string value
	body := "{\"payload\":{\"key\":\"val\x00ue\"}}"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body))

	// Should not return 500 -- 201 or 200 means null byte was stripped
	if w.Code == http.StatusInternalServerError {
		t.Fatalf("expected non-500 response, got 500: %s", w.Body.String())
	}
}

func TestHandleTriggerJob_PastScheduledAt_Returns400(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	pastTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	body := fmt.Sprintf(`{"scheduled_at":"%s"}`, pastTime)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "past") {
		t.Fatalf("expected error about past, got %s", w.Body.String())
	}
}

func TestHandleTriggerJob_ScheduledAtTooFar_Returns400(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	futureTime := time.Now().Add(31 * 24 * time.Hour).Format(time.RFC3339)
	body := fmt.Sprintf(`{"scheduled_at":"%s"}`, futureTime)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "30 days") {
		t.Fatalf("expected error about 30 days, got %s", w.Body.String())
	}
}
