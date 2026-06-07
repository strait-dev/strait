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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, "ok", resp["status"])
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	assert.Equal(
		t, "ok", resp["status"])

	if _, ok := resp["version"]; !ok {
		assert.Fail(t,

			"expected version field in public response")
	}
	if _, ok := resp["timestamp"]; !ok {
		assert.Fail(t,

			"expected timestamp field in public response")
	}
	if _, ok := resp["edition"]; ok {
		assert.Fail(t,

			"edition should not be in public response (internal only)")
	}
}

func TestHandleAuth_MissingSecret(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/jobs/", nil)
	// No X-Internal-Secret header

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,

		w.Code,
	)
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.True(
		t, created.Load(),
	)
}

func TestHandleCreateJob_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", `{}`))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code,
	)

	var resp struct {
		Error struct {
			Code    string   `json:"code"`
			Message string   `json:"message"`
			Details []string `json:"details"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, ErrorCodeValidationFailed,

		resp.
			Error.
			Code)
	require.Equal(t, "validation failed",
		resp.
			Error.
			Message,
	)
	require.NotEmpty(t, resp.Error.
		Details)
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
	require.NoError(t, err)

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", string(body)))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "too many tags (max 20)")
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
	require.NoError(t, err)

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", string(body)))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "tag key too long (max 64 characters)")
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, "job-123", resp["id"])
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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	require.Len(t,
		resp, 2)
}

func TestHandleListJobs_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/", ""))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.True(
		t, created.Load(),
	)
}

func TestHandleCreateJobGroup_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/job-groups/", `{}`))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code,
	)

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, ErrorCodeValidationFailed,

		resp.
			Error.
			Code)
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
	require.Equal(t, http.StatusOK,
		w.Code)
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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	require.Len(t,
		resp, 2)
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
	require.Equal(t, http.StatusNoContent,
		w.
			Code)
	require.Equal(t, "group-123",
		deletedID)
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	require.Len(t,
		resp, 1)
}

func TestHandleListJobs_FilterByTag(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListJobsByTagFunc: func(_ context.Context, projectID, tagKey, tagValue string, _ int, _ *time.Time) ([]domain.Job, error) {
			require.False(t, projectID !=
				"proj-1" ||
				tagKey !=
					"team" ||
				tagValue !=
					"core")

			return []domain.Job{{ID: "job-1", ProjectID: projectID, Name: "Job 1"}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/jobs/?tag_key=team&tag_value=core", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	require.Len(t,
		resp, 1)
}

func TestHandleCreateJobDependency_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true}, nil
	}
	ms.CreateJobDependencyFunc = func(_ context.Context, dep *domain.JobDependency) error {
		require.Equal(t, "job-1", dep.
			JobID)
		require.Equal(t, "job-2", dep.
			DependsOnJobID,
		)
		require.Equal(t, "completed",
			dep.Condition,
		)

		dep.ID = "dep-1"
		dep.CreatedAt = time.Now()
		return nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/dependencies", `{"depends_on_job_id":"job-2"}`))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp domain.JobDependency
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, "dep-1", resp.
		ID)
}

func TestHandleCreateJobDependency_SelfReference(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true}, nil
	}
	ms.CreateJobDependencyFunc = func(_ context.Context, _ *domain.JobDependency) error {
		require.Fail(t,

			"CreateJobDependency should not be called")
		return nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/dependencies", `{"depends_on_job_id":"job-1"}`))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
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
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code,
	)
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []domain.JobDependency
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	require.Len(t,
		resp, 1)
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
		require.Equal(t, 51, limit)
		require.Nil(t, cursor)

		return []domain.JobDependency{{ID: "dep-1", JobID: jobID, DependsOnJobID: "job-2", Condition: "completed", CreatedAt: createdAt, CacheVersion: 3}}, nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.jobDependencyCache = newJobDependencyCache(time.Minute)

	for range 2 {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-1/dependencies", ""))
		require.Equal(t, http.StatusOK,
			w.Code)
	}
	require.EqualValues(t, 1, listCalls.
		Load())
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
		require.NotNil(t, cursor)

		return []domain.JobDependency{{ID: "dep-1", JobID: jobID, DependsOnJobID: "job-2", Condition: "completed", CreatedAt: createdAt}}, nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.jobDependencyCache = newJobDependencyCache(time.Minute)
	url := "/v1/jobs/job-1/dependencies?cursor=" + createdAt.Add(time.Second).Format(time.RFC3339Nano)

	for range 2 {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, url, ""))
		require.Equal(t, http.StatusOK,
			w.Code)
	}
	require.EqualValues(t, 2, listCalls.
		Load())
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
		require.Equal(t, 11, limit)
		require.Nil(t, cursor)

		return []domain.JobDependency{{ID: "dep-1", JobID: jobID, DependsOnJobID: "job-2", Condition: "completed", CreatedAt: createdAt, CacheVersion: 3}}, nil
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.jobDependencyCache = newJobDependencyCache(time.Minute)

	for range 2 {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-1/dependencies?limit=10", ""))
		require.Equal(t, http.StatusOK,
			w.Code)
	}
	require.EqualValues(t, 2, listCalls.
		Load())
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
	require.Equal(t, http.StatusNoContent,
		w.
			Code)
	require.Equal(t, "dep-9", deletedID)
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.False(t, resp["id"] ==
		nil || resp["id"] == "",
	)

	if _, ok := resp["run_token"]; ok {
		require.Fail(t,

			"trigger response must not expose SDK run_token")
	}
	require.Equal(t, "queued", resp["status"])
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.Equal(t, domain.StatusWaiting,
		createdRunStatus,
	)
	require.False(t, enqueueCalled)
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
			require.False(t, jobID != "job-123" ||
				key !=
					"same-key",
			)

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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, lookupCalled)
	require.False(t, enqueueCalled)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, "run-existing",
		resp["id"])
	require.Equal(t, string(domain.
		StatusWaiting,
	),
		resp["status"])

	if hit, ok := resp["idempotency_hit"].(bool); !ok || !hit {
		require.Failf(t, "test failure",

			"expected idempotency_hit=true, got %v", resp["idempotency_hit"])
	}
	if _, ok := resp["run_token"]; ok {
		require.Fail(t,

			"did not expect run_token for idempotency hit")
	}
	if _, ok := resp["payload_hash"]; ok {
		require.Fail(t,

			"did not expect payload_hash for idempotency hit")
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
	require.Equal(t, http.StatusInternalServerError,

		w.Code,
	)
	require.Contains(
		t, w.Body.
			String(), "internal server error")
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.True(
		t, enqueueCalled)
	require.False(t, createRunCalled)
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleGetRunDependencyStatus_Success(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", Status: domain.StatusWaiting}, nil
		},
		ListJobDependenciesFunc: func(_ context.Context, jobID string, _ int, _ *time.Time) ([]domain.JobDependency, error) {
			require.Equal(t, "job-1", jobID)

			return []domain.JobDependency{{ID: "dep-1", JobID: "job-1", DependsOnJobID: "job-2", Condition: "completed"}}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, run *domain.JobRun) (bool, error) {
			require.Equal(t, "run-1", run.
				ID)

			return false, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/dependency-status", ""))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, false, resp["dependencies_satisfied"])
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.InDelta(t, float64(5), resp["queued"], 1e-9)
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
			assert.Equal(
				t, domain.StatusCanceled,
				to,
			)

			return nil
		},
		ListChildRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-123", ""))
	require.Equal(t, http.StatusOK,
		w.Code)
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
	require.Equal(t, http.StatusNoContent,
		w.
			Code)
	require.Equal(t, "job-123", deletedID)
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
	require.Equal(t, http.StatusConflict,
		w.
			Code)
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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, updated)
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
			require.Equal(t, http.StatusBadRequest,

				w.Code)
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)
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
			require.Equal(t, tt.wantStatus,
				w.Code)
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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, updated)
}

func TestHandleUpdateJob_ValidateTagsValueTooLong(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Name: "Test", Slug: "test", EndpointURL: "https://example.com", Enabled: true}, nil
		},
		UpdateJobFunc: func(_ context.Context, _ *domain.Job) error {
			require.Fail(t,

				"UpdateJob should not be called for invalid tags")
			return nil
		},
	}

	req := map[string]any{
		"tags": map[string]string{
			"team": strings.Repeat("v", 257),
		},
	}
	body, err := json.Marshal(req)
	require.NoError(t, err)

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", string(body)))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "tag value too long (max 256 characters)")
}

func TestHandleUpdateJob_ValidateTagsEmptyKey(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Name: "Test", Slug: "test", EndpointURL: "https://example.com", Enabled: true}, nil
		},
		UpdateJobFunc: func(_ context.Context, _ *domain.Job) error {
			require.Fail(t,

				"UpdateJob should not be called for invalid tags")
			return nil
		},
	}

	req := map[string]any{
		"tags": map[string]string{
			"": "core",
		},
	}
	body, err := json.Marshal(req)
	require.NoError(t, err)

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", string(body)))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "tag keys must be non-empty")
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
			require.Equal(t, "run-123", id)

			return originalRun, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			require.Equal(t, "job-1", id)

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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, enqueued)
	require.Equal(t, originalRun.JobID,
		enqueued.
			JobID,
	)
	require.Equal(t, originalRun.ProjectID,

		enqueued.
			ProjectID,
	)
	require.Equal(t, 1, enqueued.Attempt)
	require.Equal(t, string(originalRun.
		Payload,
	), string(
		enqueued.Payload,
	))
	require.Empty(t, enqueued.
		IdempotencyKey,
	)

	// Replays should NOT carry the original idempotency key to avoid
	// conflicts with active runs sharing the same key.
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "job is disabled")
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleListDeadLetterRuns_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListDeadLetterRunsFunc: func(_ context.Context, projectID string, limit int, _ *time.Time) ([]domain.JobRun, error) {
			require.Equal(t, "proj-1", projectID)
			require.Equal(t, 26, limit)

			// handler passes limit+1 for has_more detection

			return []domain.JobRun{{ID: "run-dlq-1", ProjectID: "proj-1", Status: domain.StatusDeadLetter}}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/dlq?limit=25", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var runs []domain.JobRun
	decodePaginatedList(t, w.Body.Bytes(), &runs)
	require.Len(t,
		runs, 1)
	require.Equal(t, domain.StatusDeadLetter,

		runs[0].Status,
	)
}

func TestHandleReplayDeadLetterRun_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusDeadLetter, ProjectID: "proj-1"}, nil
		},
		ReplayDeadLetterRunFunc: func(_ context.Context, runID string) (*domain.JobRun, error) {
			require.Equal(t, "run-123", runID)

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
	require.Equal(t, http.StatusOK,
		w.Code)

	var run domain.JobRun
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &run,
	))
	require.Equal(t, domain.StatusQueued,
		run.
			Status,
	)
	require.True(
		t, slices.Equal(enqueuedExisting,

			[]string{"run-123"}))
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
	require.Equal(t, http.StatusConflict,
		w.
			Code)
}

func TestHandleBulkReplayDeadLetterRuns_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusDeadLetter, ProjectID: "proj-1"}, nil
		},
		BulkReplayDeadLetterRunsFunc: func(_ context.Context, runIDs []string, projectID string, limit int) ([]domain.JobRun, error) {
			require.False(t, len(runIDs) !=
				2 || runIDs[0] !=
				"run-1" ||
				runIDs[1] != "run-2")
			require.Empty(t, projectID)
			require.Equal(t, 0, limit)

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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp struct {
		Count    int             `json:"count"`
		Replayed []domain.JobRun `json:"replayed"`
	}
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, 2, resp.Count)
	require.Len(t,
		resp.Replayed,
		2)
	require.True(
		t, slices.Equal(enqueuedExisting,

			[]string{"run-1",
				"run-2"}))
}

func TestHandleBulkReplayDeadLetterRuns_RunIDsModeDoesNotSendProjectIDOrLimit(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusDeadLetter, ProjectID: "proj-1"}, nil
		},
		BulkReplayDeadLetterRunsFunc: func(_ context.Context, runIDs []string, projectID string, limit int) ([]domain.JobRun, error) {
			require.False(t, len(runIDs) !=
				1 || runIDs[0] !=
				"run-1",
			)
			require.Empty(t, projectID)
			require.Equal(t, 0, limit)

			return []domain.JobRun{{ID: "run-1", Status: domain.StatusQueued, Attempt: 1}}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-dlq-replay", `{"run_ids":["run-1"],"limit":123}`))
	require.Equal(t, http.StatusOK,
		w.Code)
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp DryRunValidationResult
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.False(t, resp.Job == nil ||
		resp.
			Job.ID !=
			"job-123",
	)

	if raw := w.Body.String(); strings.Contains(raw, "endpoint_url") || strings.Contains(raw, "https://example.com/callback") {
		require.Failf(t, "test failure",

			"dry-run response leaked endpoint details: %s", raw)
	}
	require.NotEmpty(t, resp.PayloadHash)
	require.False(t, resp.ExpiresAt.
		IsZero(),
	)
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var cloned domain.Job
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &cloned,
	))

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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.Equal(t, 2, createdCount)

	var resp BatchCreateJobsResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Len(t,
		resp.Created, 2,
	)
	require.Empty(t,
		resp.Errors)
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp BatchCreateJobsResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Len(t,
		resp.Created, 1,
	)
	require.Len(t,
		resp.Errors, 1)
	require.Equal(t, 1, resp.Errors[0].Index)
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleBatchCreateJobs_EmptyArray(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch", `{"jobs":[]}`))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "too many jobs in batch")
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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, capturedEnabled,
	)
	require.Len(t,
		capturedIDs, 3)
	require.Equal(t, "proj-aaa", capturedProject)

	var resp BatchUpdateResult
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.EqualValues(t, 3, resp.Updated)
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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.False(t, capturedEnabled)

	var resp BatchUpdateResult
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.EqualValues(t, 2, resp.Updated)
}

func TestHandleBatchEnableJobs_EmptyIDs(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch-enable", `{"ids":[]}`))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "ids array is required")
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "too many ids in batch")
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp JobHealthResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, "job-123", resp.
		JobID)
	require.Equal(t, "7d", resp.Window)
	require.Equal(t, 100, resp.TotalRuns)
	require.InDelta(t, 85.0, resp.SuccessRate, 1e-9)
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp JobHealthResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, "7d", resp.Window)
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "invalid window")
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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.True(
		t, created.Load(),
	)
}

func TestHandleCreateEnvironment_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/environments/", `{}`))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code,
	)
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
			require.Equal(t, "env-1", id)

			return map[string]string{"LOG_LEVEL": "debug", "REGION": "us-east-1"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/environments/env-1", ""))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, "env-1", resp["id"])

	resolved, ok := resp["resolved_variable_keys"].([]any)
	require.True(
		t, ok)
	require.True(
		t, slices.ContainsFunc(resolved,
			func(v any) bool {
				return v == "REGION"
			}))
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	require.Len(t,
		resp, 2)
}

func TestHandleGetResolvedVariables_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "test", Slug: "test"}, nil
		},
		GetResolvedEnvironmentVariablesFunc: func(_ context.Context, id string) (map[string]string, error) {
			require.Equal(t, "env-1", id)

			return map[string]string{"API_URL": "https://api.example.com", "LOG_LEVEL": "debug"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/environments/env-1/variables", ""))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]map[string]string
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, "https://api.example.com",

		resp["variables"]["API_URL"])
}

func TestHandleGetDebugBundle_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetDebugBundleFunc: func(_ context.Context, runID string) (*domain.DebugBundle, error) {
			require.Equal(t, "run-1", runID)

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
	require.Equal(t, http.StatusOK,
		w.Code)

	var bundle domain.DebugBundle
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &bundle,
	))
	require.Equal(t, "run-1", bundle.
		Run.ID)
	require.Len(t,
		bundle.Events,
		1)
	require.Len(t,
		bundle.Checkpoints,
		1)
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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Equal(t, "run-1", calledRunID)
	require.True(
		t, calledDebugMode,
	)
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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
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
			require.Equal(t, "run-1", runID)

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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, enqueuedRun)
	require.Equal(t, `{"step":2}`,
		string(enqueuedRun.
			Payload,
		))
	require.True(
		t, enqueuedRun.DebugMode,
	)
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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleListRunLineage_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListRunLineageFunc: func(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			require.Equal(t, "run-1", runID)

			return []domain.JobRun{
				{ID: "run-root", LineageDepth: 0},
				{ID: "run-1", ContinuationOf: "run-root", LineageDepth: 1},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/lineage", ""))
	require.Equal(t, http.StatusOK,
		w.Code)

	var runs []domain.JobRun
	decodePaginatedList(t, w.Body.Bytes(), &runs)
	require.Len(t,
		runs, 2)
	require.Equal(t, "run-root", runs[0].ID)
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
	require.Equal(t, http.StatusInternalServerError,

		w.Code,
	)
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

func TestDBBackpressure_Returns429WhenPoolExhausted(t *testing.T) {
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
	require.Equal(t, http.StatusTooManyRequests,

		w.Code,
	)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, ErrorCodeRateLimited, body.Error.Code)
}

func TestDBBackpressure_TriggerRouteShortCircuitsBeforeAuthAndStore(t *testing.T) {
	t.Parallel()

	var authLookups atomic.Int32
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config: cfg,
		Store: &APIStoreMock{
			GetAPIKeyByHashFunc: func(context.Context, string) (*domain.APIKey, error) {
				authLookups.Add(1)
				return nil, errors.New("auth should not run when db admission is closed")
			},
		},
		Queue:       &mockQueue{},
		PoolStatter: newMockPoolStatter(24, 25), // 96% > 90%
	})
	t.Cleanup(srv.Close)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/jobs/job-1/trigger", strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer strait_test")
	r.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(w, r)

	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.EqualValues(t, 0, authLookups.Load())
	require.Equal(t, "1", w.Header().Get("Retry-After"))
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, ErrorCodeRateLimited, body.Error.Code)
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
	require.NotEqual(t, http.StatusTooManyRequests,

		w.
			Code)
}

func TestDBBackpressure_Returns429WhenAcquireWaitSpikes(t *testing.T) {
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
	require.Equal(t, http.StatusTooManyRequests,

		w.Code,
	)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, ErrorCodeRateLimited, body.Error.Code)
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
	require.NotEqual(t, http.StatusTooManyRequests,

		w.
			Code)
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
	require.Equal(t, http.StatusConflict,
		w.
			Code)
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
		assert.Equal(
			t, tt.wantErr, (err !=
				nil),
		)
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
	require.Equal(t, http.StatusUnauthorized,

		w.Code,
	)
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
	require.Equal(t, http.StatusOK,
		w.Code)
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
	require.Equal(t, http.StatusOK,
		w.Code)
}

func TestNullByteStrippingReader(t *testing.T) {
	t.Parallel()
	input := []byte("hello\x00world")
	reader := &nullByteStrippingReader{r: strings.NewReader(string(input))}
	out, err := io.ReadAll(reader)
	require.NoError(t, err)

	expected := "hello world"
	require.Equal(t, expected, string(out))
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
	require.NotEqual(t, http.StatusInternalServerError,

		w.
			Code)

	// Should not return 500 -- 201 or 200 means null byte was stripped
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "past")
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "30 days")
}
