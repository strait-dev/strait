package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func makeVersionedJob(id string, version int) *domain.Job {
	return &domain.Job{
		ID:          id,
		ProjectID:   "proj-1",
		Name:        "Test Job",
		Slug:        "test-job",
		EndpointURL: "https://example.com/callback",
		Enabled:     true,
		TimeoutSecs: 300,
		MaxAttempts: 3,
		Version:     version,
	}
}

func TestTriggerJob_StampsJobVersion(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return makeVersionedJob(id, 3), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, capturedRun)
	require.Equal(t, 3, capturedRun.
		JobVersion,
	)
}

func TestTriggerJob_StampsVersionOne(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return makeVersionedJob(id, 1), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, capturedRun)
	require.Equal(t, 1, capturedRun.
		JobVersion,
	)
}

func TestTriggerJob_DefaultVersionIfZero(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return makeVersionedJob(id, 0), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, capturedRun)
	require.Equal(t, 0, capturedRun.
		JobVersion,
	)
}

func TestBulkTrigger_StampsJobVersion(t *testing.T) {
	t.Parallel()
	var capturedRuns []*domain.JobRun

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return makeVersionedJob(id, 5), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRuns = append(capturedRuns, run)
			return nil
		},
	}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	body := `{"items":[{},{},{}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.Len(t,
		capturedRuns, 3,
	)

	for _, run := range capturedRuns {
		require.Equal(t, 5, run.JobVersion)
	}
}

func TestBulkTrigger_VersionConsistency(t *testing.T) {
	t.Parallel()
	var capturedRuns []*domain.JobRun

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return makeVersionedJob(id, 9), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRuns = append(capturedRuns, run)
			return nil
		},
	}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	body := `{"items":[{},{},{}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.Len(t,
		capturedRuns, 3,
	)

	first := capturedRuns[0].JobVersion
	for _, run := range capturedRuns {
		require.Equal(t, first, run.JobVersion)
	}
}

func TestCreateJob_ReturnsVersion(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			job.ID = "job-123"
			job.Version = 1
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "Versioned Job",
		"slug": "versioned-job",
		"endpoint_url": "https://example.com/callback"
	}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.InDelta(t, float64(1), resp["version"], 1e-9)
}

func TestUpdateJob_IncrementsVersion(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return makeVersionedJob(id, 2), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
		UpdateJobFunc: func(_ context.Context, job *domain.Job) error {
			job.Version = 3
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"name":"Updated Name"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", body))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.InDelta(t, float64(3), resp["version"], 1e-9)
}

func TestListJobVersions_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		ListJobVersionsByJobFunc: func(_ context.Context, jobID string, _ int, _ *time.Time) ([]domain.JobVersion, error) {
			return []domain.JobVersion{
				{ID: "v3", JobID: jobID, Version: 3, Name: "name-v3", Slug: "slug-v3", EndpointURL: "https://example.com"},
				{ID: "v2", JobID: jobID, Version: 2, Name: "name-v2", Slug: "slug-v2", EndpointURL: "https://example.com"},
				{ID: "v1", JobID: jobID, Version: 1, Name: "name-v1", Slug: "slug-v1", EndpointURL: "https://example.com"},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-123/versions", ""))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	require.Len(t,
		resp, 3)

	for _, v := range resp {
		require.NotNil(t, v["version"])
	}
}

func TestListJobVersions_Empty(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		ListJobVersionsByJobFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobVersion, error) {
			return []domain.JobVersion{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-123/versions", ""))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	require.Empty(t,
		resp)
}

func TestListJobVersions_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-123", ProjectID: "proj-1"}, nil
		},
		ListJobVersionsByJobFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobVersion, error) {
			return nil, errors.New("boom")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-123/versions", ""))
	require.Equal(t, http.StatusInternalServerError,

		w.Code,
	)
}

func TestGetJob_IncludesVersion(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return makeVersionedJob(id, 5), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
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
	require.InDelta(t, float64(5), resp["version"], 1e-9)
}

func TestListJobs_IncludesVersion(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListJobsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.Job, error) {
			return []domain.Job{
				*makeVersionedJob("job-1", 2),
				*makeVersionedJob("job-2", 7),
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/jobs", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	require.Len(t,
		resp, 2)

	for _, job := range resp {
		require.NotNil(t, job["version"])
	}
}

func TestGetRun_IncludesJobVersion(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:          id,
				JobID:       "job-1",
				ProjectID:   "proj-1",
				Status:      domain.StatusQueued,
				Attempt:     1,
				TriggeredBy: domain.TriggerManual,
				JobVersion:  3,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-123", ""))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.InDelta(t, float64(3), resp["job_version"], 1e-9)
}

func TestListRuns_IncludesJobVersion(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusQueued, Attempt: 1, TriggeredBy: domain.TriggerManual, JobVersion: 2},
				{ID: "run-2", JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusQueued, Attempt: 1, TriggeredBy: domain.TriggerManual, JobVersion: 4},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	require.Len(t,
		resp, 2)

	for _, run := range resp {
		require.NotNil(t, run["job_version"])
	}
}

func TestListJobVersions_ReturnsExpectedVersionNumbers(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		ListJobVersionsByJobFunc: func(_ context.Context, jobID string, _ int, _ *time.Time) ([]domain.JobVersion, error) {
			return []domain.JobVersion{
				{ID: "v3", JobID: jobID, Version: 3, Name: "name-v3", Slug: "slug-v3", EndpointURL: "https://example.com"},
				{ID: "v2", JobID: jobID, Version: 2, Name: "name-v2", Slug: "slug-v2", EndpointURL: "https://example.com"},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-123/versions", ""))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	require.Len(t,
		resp, 2)
	require.InDelta(t, float64(3), resp[0]["version"], 1e-9)
	require.InDelta(t, float64(2), resp[1]["version"], 1e-9)
}
