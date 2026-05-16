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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if capturedRun == nil {
		t.Fatal("expected enqueue to be called")
	}
	if capturedRun.JobVersion != 3 {
		t.Fatalf("expected job_version=3, got %d", capturedRun.JobVersion)
	}
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if capturedRun == nil {
		t.Fatal("expected enqueue to be called")
	}
	if capturedRun.JobVersion != 1 {
		t.Fatalf("expected job_version=1, got %d", capturedRun.JobVersion)
	}
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if capturedRun == nil {
		t.Fatal("expected enqueue to be called")
	}
	if capturedRun.JobVersion != 0 {
		t.Fatalf("expected job_version=0, got %d", capturedRun.JobVersion)
	}
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(capturedRuns) != 3 {
		t.Fatalf("expected 3 enqueued runs, got %d", len(capturedRuns))
	}
	for i, run := range capturedRuns {
		if run.JobVersion != 5 {
			t.Fatalf("expected run[%d].job_version=5, got %d", i, run.JobVersion)
		}
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(capturedRuns) != 3 {
		t.Fatalf("expected 3 enqueued runs, got %d", len(capturedRuns))
	}
	first := capturedRuns[0].JobVersion
	for i, run := range capturedRuns {
		if run.JobVersion != first {
			t.Fatalf("expected run[%d].job_version=%d, got %d", i, first, run.JobVersion)
		}
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["version"] != float64(1) {
		t.Fatalf("expected version=1, got %v", resp["version"])
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["version"] != float64(3) {
		t.Fatalf("expected version=3, got %v", resp["version"])
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	if len(resp) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(resp))
	}
	for i, v := range resp {
		if v["version"] == nil {
			t.Fatalf("expected version on item %d, got nil", i)
		}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	if len(resp) != 0 {
		t.Fatalf("expected empty array, got %d items", len(resp))
	}
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

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["version"] != float64(5) {
		t.Fatalf("expected version=5, got %v", resp["version"])
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	if len(resp) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(resp))
	}
	for i, job := range resp {
		if job["version"] == nil {
			t.Fatalf("expected version on job[%d], got nil", i)
		}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["job_version"] != float64(3) {
		t.Fatalf("expected job_version=3, got %v", resp["job_version"])
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	if len(resp) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(resp))
	}
	for i, run := range resp {
		if run["job_version"] == nil {
			t.Fatalf("expected job_version on run[%d], got nil", i)
		}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	if len(resp) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(resp))
	}
	if resp[0]["version"] != float64(3) {
		t.Fatalf("expected first version=3, got %v", resp[0]["version"])
	}
	if resp[1]["version"] != float64(2) {
		t.Fatalf("expected second version=2, got %v", resp[1]["version"])
	}
}
