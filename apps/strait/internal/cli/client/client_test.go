package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "valid http", url: "http://localhost:8080", wantErr: false},
		{name: "valid https", url: "https://api.example.com", wantErr: false},
		{name: "trailing slash", url: "http://localhost:8080/", wantErr: false},
		{name: "empty url", url: "", wantErr: true},
		{name: "whitespace", url: "   ", wantErr: true},
		{name: "invalid scheme", url: "ftp://example.com", wantErr: true},
		{name: "no scheme", url: "example.com", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := New(tc.url, "test-key", 10*time.Second)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c == nil {
				t.Fatal("expected client, got nil")
			}
		})
	}
}

func TestNew_DefaultTimeout(t *testing.T) {
	t.Parallel()

	c, err := New("http://localhost:8080", "key", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.http.Timeout != 30*time.Second {
		t.Fatalf("expected 30s timeout, got %v", c.http.Timeout)
	}
}

func TestNew_StreamHTTPHasNoTimeout(t *testing.T) {
	t.Parallel()

	c, err := New("http://localhost:8080", "key", 10*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.streamHTTP.Timeout != 0 {
		t.Fatalf("expected 0 timeout for streamHTTP, got %v", c.streamHTTP.Timeout)
	}
}

func TestListJobs(t *testing.T) {
	t.Parallel()

	jobs := []domain.Job{
		{ID: "job-1", ProjectID: "proj-1", Name: "Test Job", Slug: "test-job", Enabled: true},
		{ID: "job-2", ProjectID: "proj-1", Name: "Other Job", Slug: "other-job", Enabled: false},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/jobs")
		assertAuth(t, r, "test-key")
		if r.URL.Query().Get("project_id") != "proj-1" {
			t.Errorf("expected project_id=proj-1, got %q", r.URL.Query().Get("project_id"))
		}
		respondPaginated(t, w, http.StatusOK, jobs)
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListJobs(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(got))
	}
	if got[0].ID != "job-1" {
		t.Fatalf("expected job-1, got %s", got[0].ID)
	}
}

func TestGetJob(t *testing.T) {
	t.Parallel()

	job := domain.Job{ID: "job-1", ProjectID: "proj-1", Name: "Test Job", Slug: "test-job"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/jobs/job-1")
		assertAuth(t, r, "test-key")
		respondJSON(t, w, http.StatusOK, job)
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.GetJob(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.ID != "job-1" {
		t.Fatalf("expected job-1, got %s", got.ID)
	}
}

func TestCreateJob(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/jobs")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var req CreateJobRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.Name != "New Job" {
			t.Fatalf("expected name=New Job, got %q", req.Name)
		}

		respondJSON(t, w, http.StatusOK, domain.Job{ID: "job-new", Name: req.Name})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.CreateJob(context.Background(), CreateJobRequest{
		ProjectID:   "proj-1",
		Name:        "New Job",
		Slug:        "new-job",
		EndpointURL: "https://example.com/hook",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if got.ID != "job-new" {
		t.Fatalf("expected job-new, got %s", got.ID)
	}
}

func TestDeleteJob(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodDelete)
		assertPath(t, r, "/v1/jobs/job-1")
		assertAuth(t, r, "test-key")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	if err := c.DeleteJob(context.Background(), "job-1"); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
}

func TestListRuns(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)
	runs := []domain.JobRun{
		{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusCompleted, CreatedAt: now},
		{ID: "run-2", JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusFailed, CreatedAt: now.Add(-time.Minute)},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/runs")
		assertAuth(t, r, "test-key")
		if r.URL.Query().Get("project_id") != "proj-1" {
			t.Errorf("expected project_id=proj-1")
		}
		if r.URL.Query().Get("limit") != "50" {
			t.Errorf("expected limit=50, got %q", r.URL.Query().Get("limit"))
		}
		respondPaginated(t, w, http.StatusOK, runs)
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListRuns(context.Background(), "proj-1", "", 50, nil)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(got))
	}
}

func TestListRuns_WithCursor(t *testing.T) {
	t.Parallel()

	cursor := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		cursorParam := r.URL.Query().Get("cursor")
		if cursorParam == "" {
			t.Fatal("expected cursor query parameter")
		}
		parsed, err := time.Parse(time.RFC3339, cursorParam)
		if err != nil {
			t.Fatalf("cursor not RFC3339: %v", err)
		}
		if !parsed.Equal(cursor) {
			t.Fatalf("cursor mismatch: got %v, want %v", parsed, cursor)
		}
		respondPaginated(t, w, http.StatusOK, []domain.JobRun{})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	_, err := c.ListRuns(context.Background(), "proj-1", "", 50, &cursor)
	if err != nil {
		t.Fatalf("ListRuns with cursor: %v", err)
	}
}

func TestListRuns_WithStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("status") != "executing" {
			t.Fatalf("expected status=executing, got %q", r.URL.Query().Get("status"))
		}
		respondPaginated(t, w, http.StatusOK, []domain.JobRun{})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	_, err := c.ListRuns(context.Background(), "proj-1", "executing", 50, nil)
	if err != nil {
		t.Fatalf("ListRuns with status: %v", err)
	}
}

func TestListAllRuns_Pagination(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)
	var callCount atomic.Int32

	// First page: 100 runs, second page: 5 runs
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		switch n {
		case 1:
			// First page: no cursor, return 100 runs
			if r.URL.Query().Get("cursor") != "" {
				t.Error("first request should not have cursor")
			}
			runs := make([]domain.JobRun, 100)
			for i := range runs {
				runs[i] = domain.JobRun{
					ID:        fmt.Sprintf("run-%d", i),
					ProjectID: "proj-1",
					CreatedAt: now.Add(-time.Duration(i) * time.Second),
				}
			}
			respondPaginated(t, w, http.StatusOK, runs)
		case 2:
			// Second page: should have cursor
			if r.URL.Query().Get("cursor") == "" {
				t.Error("second request should have cursor")
			}
			runs := make([]domain.JobRun, 5)
			for i := range runs {
				runs[i] = domain.JobRun{
					ID:        fmt.Sprintf("run-page2-%d", i),
					ProjectID: "proj-1",
					CreatedAt: now.Add(-time.Duration(100+i) * time.Second),
				}
			}
			respondPaginated(t, w, http.StatusOK, runs)
		default:
			t.Fatalf("unexpected call #%d", n)
		}
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListAllRuns(context.Background(), "proj-1", "")
	if err != nil {
		t.Fatalf("ListAllRuns: %v", err)
	}
	if len(got) != 105 {
		t.Fatalf("expected 105 total runs, got %d", len(got))
	}
	if callCount.Load() != 2 {
		t.Fatalf("expected 2 HTTP calls, got %d", callCount.Load())
	}
}

func TestTriggerJob(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/jobs/job-1/trigger")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		respondJSON(t, w, http.StatusOK, TriggerJobResponse{
			ID: "run-1",
		})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.TriggerJob(context.Background(), "job-1", TriggerJobRequest{}, "")
	if err != nil {
		t.Fatalf("TriggerJob: %v", err)
	}
	if got.ID != "run-1" {
		t.Fatalf("expected run-1, got %s", got.ID)
	}
}

func TestTriggerJob_IdempotencyKey(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Idempotency-Key")
		if key != "my-key-123" {
			t.Fatalf("expected idempotency key my-key-123, got %q", key)
		}
		respondJSON(t, w, http.StatusOK, TriggerJobResponse{ID: "run-1"})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	_, err := c.TriggerJob(context.Background(), "job-1", TriggerJobRequest{}, "my-key-123")
	if err != nil {
		t.Fatalf("TriggerJob with idempotency key: %v", err)
	}
}

func TestDoJSON_4xxError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid payload"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	_, err := c.ListJobs(context.Background(), "proj-1")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("error should contain status code: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid payload") {
		t.Fatalf("error should contain message: %v", err)
	}
}

func TestDoJSON_AuthHeader(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer secret-key-123" {
			t.Fatalf("expected Bearer secret-key-123, got %q", auth)
		}
		respondPaginated(t, w, http.StatusOK, []domain.Job{})
	}))
	defer srv.Close()

	c, err := New(srv.URL, "secret-key-123", 10*time.Second)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = c.ListJobs(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
}

func TestDoJSON_Retry429(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := callCount.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		respondPaginated(t, w, http.StatusOK, []domain.Job{{ID: "job-1"}})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListJobs(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 job, got %d", len(got))
	}
	if callCount.Load() != 3 {
		t.Fatalf("expected 3 calls (2 retries + 1 success), got %d", callCount.Load())
	}
}

func TestDoJSON_Retry5xx(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := callCount.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		respondPaginated(t, w, http.StatusOK, []domain.Job{{ID: "job-1"}})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListJobs(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 job, got %d", len(got))
	}
	if callCount.Load() != 3 {
		t.Fatalf("expected 3 calls, got %d", callCount.Load())
	}
}

func TestDoJSON_RetryExhausted(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	_, err := c.ListJobs(context.Background(), "proj-1")
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if callCount.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", callCount.Load())
	}
}

func TestUpdateJob(t *testing.T) {
	t.Parallel()

	name := "Updated Job"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPatch)
		assertPath(t, r, "/v1/jobs/job-1")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var req UpdateJobRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.Name == nil || *req.Name != name {
			t.Fatalf("expected name=%q", name)
		}

		respondJSON(t, w, http.StatusOK, domain.Job{ID: "job-1", Name: name})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.UpdateJob(context.Background(), "job-1", UpdateJobRequest{Name: &name})
	if err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}
	if got.Name != name {
		t.Fatalf("expected %q, got %q", name, got.Name)
	}
}

func TestBulkTriggerJob(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/jobs/job-1/trigger/bulk")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var req BulkTriggerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if len(req.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(req.Items))
		}

		respondJSON(t, w, http.StatusOK, BulkTriggerResponse{
			Results: []BulkTriggerResult{{ID: "run-1", Status: "queued"}},
			Total:   1,
			Created: 1,
		})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.BulkTriggerJob(context.Background(), "job-1", BulkTriggerRequest{Items: []BulkTriggerItem{{Priority: 3}}})
	if err != nil {
		t.Fatalf("BulkTriggerJob: %v", err)
	}
	if got.Created != 1 || len(got.Results) != 1 {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestListJobVersions(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/jobs/job-1/versions")
		assertAuth(t, r, "test-key")
		respondPaginated(t, w, http.StatusOK, []domain.JobVersion{{
			ID:          "jv-1",
			JobID:       "job-1",
			Version:     1,
			Name:        "Job",
			Slug:        "job",
			EndpointURL: "https://example.com/hook",
			MaxAttempts: 3,
			TimeoutSecs: 30,
			CreatedAt:   now,
		}})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListJobVersions(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("ListJobVersions: %v", err)
	}
	if len(got) != 1 || got[0].ID != "jv-1" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestGetRun(t *testing.T) {
	t.Parallel()

	run := domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusExecuting}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/runs/run-1")
		assertAuth(t, r, "test-key")
		respondJSON(t, w, http.StatusOK, run)
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.GetRun(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.ID != "run-1" {
		t.Fatalf("expected run-1, got %s", got.ID)
	}
}

func TestCancelRun(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodDelete)
		assertPath(t, r, "/v1/runs/run-1")
		assertAuth(t, r, "test-key")
		respondJSON(t, w, http.StatusOK, domain.JobRun{ID: "run-1", Status: domain.StatusCanceled})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.CancelRun(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if got.Status != domain.StatusCanceled {
		t.Fatalf("expected status canceled, got %s", got.Status)
	}
}

func TestListRunEvents(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/runs/run-1/events")
		assertAuth(t, r, "test-key")
		if r.URL.Query().Get("level") != "info" {
			t.Fatalf("expected level=info, got %q", r.URL.Query().Get("level"))
		}
		if r.URL.Query().Get("type") != "progress" {
			t.Fatalf("expected type=progress, got %q", r.URL.Query().Get("type"))
		}
		respondPaginated(t, w, http.StatusOK, []domain.RunEvent{{
			ID:        "evt-1",
			RunID:     "run-1",
			Type:      domain.EventType("progress"),
			Level:     "info",
			Message:   "step done",
			CreatedAt: now,
		}})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListRunEvents(context.Background(), "run-1", "info", "progress")
	if err != nil {
		t.Fatalf("ListRunEvents: %v", err)
	}
	if len(got) != 1 || got[0].ID != "evt-1" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestListWorkflows(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/workflows")
		assertAuth(t, r, "test-key")
		if r.URL.Query().Get("project_id") != "proj-1" {
			t.Fatalf("expected project_id=proj-1, got %q", r.URL.Query().Get("project_id"))
		}
		respondPaginated(t, w, http.StatusOK, []domain.Workflow{{ID: "wf-1", ProjectID: "proj-1", Name: "Flow", Slug: "flow", Enabled: true}})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListWorkflows(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(got) != 1 || got[0].ID != "wf-1" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestGetWorkflow(t *testing.T) {
	t.Parallel()

	resp := WorkflowResponse{
		Workflow: domain.Workflow{ID: "wf-1", ProjectID: "proj-1", Name: "Flow", Slug: "flow", Enabled: true},
		Steps:    []domain.WorkflowStep{{ID: "step-1", WorkflowID: "wf-1", StepRef: "step_1", DependsOn: []string{}, OnFailure: domain.FailWorkflow}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/workflows/wf-1")
		assertAuth(t, r, "test-key")
		respondJSON(t, w, http.StatusOK, resp)
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.GetWorkflow(context.Background(), "wf-1")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if got.ID != "wf-1" || len(got.Steps) != 1 {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestCreateWorkflow(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/workflows")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var req CreateWorkflowRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.ProjectID != "proj-1" || req.Name != "Flow" {
			t.Fatalf("unexpected request: %+v", req)
		}

		respondJSON(t, w, http.StatusOK, WorkflowResponse{Workflow: domain.Workflow{ID: "wf-1", ProjectID: req.ProjectID, Name: req.Name, Slug: req.Slug, Enabled: true}})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.CreateWorkflow(context.Background(), CreateWorkflowRequest{ProjectID: "proj-1", Name: "Flow", Slug: "flow"})
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if got.ID != "wf-1" {
		t.Fatalf("expected wf-1, got %s", got.ID)
	}
}

func TestUpdateWorkflow(t *testing.T) {
	t.Parallel()

	name := "Renamed Flow"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPatch)
		assertPath(t, r, "/v1/workflows/wf-1")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var req UpdateWorkflowRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.Name == nil || *req.Name != name {
			t.Fatalf("expected name=%q", name)
		}

		respondJSON(t, w, http.StatusOK, WorkflowResponse{Workflow: domain.Workflow{ID: "wf-1", Name: name}})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.UpdateWorkflow(context.Background(), "wf-1", UpdateWorkflowRequest{Name: &name})
	if err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	if got.Name != name {
		t.Fatalf("expected %q, got %q", name, got.Name)
	}
}

func TestDeleteWorkflow(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodDelete)
		assertPath(t, r, "/v1/workflows/wf-1")
		assertAuth(t, r, "test-key")
		respondJSON(t, w, http.StatusOK, map[string]string{})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	if err := c.DeleteWorkflow(context.Background(), "wf-1"); err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}
}

func TestTriggerWorkflow(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/workflows/wf-1/trigger")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var req TriggerWorkflowRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.ProjectID != "proj-1" {
			t.Fatalf("expected project_id=proj-1, got %q", req.ProjectID)
		}

		respondJSON(t, w, http.StatusOK, domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusPending})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.TriggerWorkflow(context.Background(), "wf-1", TriggerWorkflowRequest{ProjectID: "proj-1"})
	if err != nil {
		t.Fatalf("TriggerWorkflow: %v", err)
	}
	if got.ID != "wr-1" {
		t.Fatalf("expected wr-1, got %s", got.ID)
	}
}

func TestListWorkflowRuns(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/workflows/wf-1/runs")
		assertAuth(t, r, "test-key")
		if r.URL.Query().Get("limit") != "20" {
			t.Fatalf("expected limit=20, got %q", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("offset") != "40" {
			t.Fatalf("expected offset=40, got %q", r.URL.Query().Get("offset"))
		}
		respondPaginated(t, w, http.StatusOK, []domain.WorkflowRun{{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListWorkflowRuns(context.Background(), "wf-1", 20, 40)
	if err != nil {
		t.Fatalf("ListWorkflowRuns: %v", err)
	}
	if len(got) != 1 || got[0].ID != "wr-1" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestGetWorkflowRun(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/workflow-runs/wr-1")
		assertAuth(t, r, "test-key")
		respondJSON(t, w, http.StatusOK, domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.GetWorkflowRun(context.Background(), "wr-1")
	if err != nil {
		t.Fatalf("GetWorkflowRun: %v", err)
	}
	if got.ID != "wr-1" {
		t.Fatalf("expected wr-1, got %s", got.ID)
	}
}

func TestCancelWorkflowRun(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodDelete)
		assertPath(t, r, "/v1/workflow-runs/wr-1")
		assertAuth(t, r, "test-key")
		respondJSON(t, w, http.StatusOK, domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusCanceled})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.CancelWorkflowRun(context.Background(), "wr-1")
	if err != nil {
		t.Fatalf("CancelWorkflowRun: %v", err)
	}
	if got.Status != domain.WfStatusCanceled {
		t.Fatalf("expected status canceled, got %s", got.Status)
	}
}

func TestListWorkflowStepRuns(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/workflow-runs/wr-1/steps")
		assertAuth(t, r, "test-key")
		respondPaginated(t, w, http.StatusOK, []domain.WorkflowStepRun{{
			ID:             "wsr-1",
			WorkflowRunID:  "wr-1",
			WorkflowStepID: "step-1",
			StepRef:        "step_1",
			Attempt:        1,
			Status:         domain.StepRunning,
		}})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListWorkflowStepRuns(context.Background(), "wr-1")
	if err != nil {
		t.Fatalf("ListWorkflowStepRuns: %v", err)
	}
	if len(got) != 1 || got[0].ID != "wsr-1" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestCreateAPIKey(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/api-keys")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var req CreateAPIKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.ProjectID != "proj-1" || req.Name != "cli" {
			t.Fatalf("unexpected request: %+v", req)
		}

		respondJSON(t, w, http.StatusOK, APIKeyCreateResponse{
			ID:        "key-1",
			ProjectID: req.ProjectID,
			Name:      req.Name,
			Key:       "strait_live_123",
			KeyPrefix: "strait_",
			Scopes:    req.Scopes,
			CreatedAt: now,
		})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.CreateAPIKey(context.Background(), CreateAPIKeyRequest{ProjectID: "proj-1", Name: "cli", Scopes: []string{"jobs:read"}})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if got.ID != "key-1" || got.Key == "" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestListAPIKeys(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/api-keys")
		assertAuth(t, r, "test-key")
		if r.URL.Query().Get("project_id") != "proj-1" {
			t.Fatalf("expected project_id=proj-1, got %q", r.URL.Query().Get("project_id"))
		}
		respondPaginated(t, w, http.StatusOK, []domain.APIKey{{ID: "key-1", ProjectID: "proj-1", Name: "cli", KeyPrefix: "strait_", Scopes: []string{"jobs:read"}}})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListAPIKeys(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(got) != 1 || got[0].ID != "key-1" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestRevokeAPIKey(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodDelete)
		assertPath(t, r, "/v1/api-keys/key-1")
		assertAuth(t, r, "test-key")
		respondJSON(t, w, http.StatusOK, map[string]string{})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	if err := c.RevokeAPIKey(context.Background(), "key-1"); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
}

func TestRotateAPIKey(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/api-keys/key-1/rotate")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var req RotateAPIKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.GracePeriodMinutes != 30 {
			t.Fatalf("expected grace_period_minutes=30, got %d", req.GracePeriodMinutes)
		}

		respondJSON(t, w, http.StatusOK, RotateAPIKeyResponse{
			OldKeyID:       "key-1",
			NewKeyID:       "key-2",
			ProjectID:      "proj-1",
			Name:           "cli",
			Key:            "strait_live_456",
			KeyPrefix:      "strait_",
			Scopes:         []string{"jobs:read"},
			CreatedAt:      now,
			GraceExpiresAt: now.Add(30 * time.Minute),
		})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.RotateAPIKey(context.Background(), "key-1", RotateAPIKeyRequest{GracePeriodMinutes: 30})
	if err != nil {
		t.Fatalf("RotateAPIKey: %v", err)
	}
	if got.NewKeyID != "key-2" {
		t.Fatalf("expected key-2, got %s", got.NewKeyID)
	}
}

func TestListEventTriggers(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/events")
		assertAuth(t, r, "test-key")
		if r.URL.Query().Get("project_id") != "proj-1" {
			t.Fatalf("expected project_id=proj-1, got %q", r.URL.Query().Get("project_id"))
		}
		if r.URL.Query().Get("status") != "waiting" {
			t.Fatalf("expected status=waiting, got %q", r.URL.Query().Get("status"))
		}
		respondPaginated(t, w, http.StatusOK, []domain.EventTrigger{{
			ID:          "et-1",
			EventKey:    "payment.received",
			ProjectID:   "proj-1",
			SourceType:  domain.EventSourceWorkflowStep,
			Status:      domain.EventTriggerStatusWaiting,
			TimeoutSecs: 120,
			RequestedAt: now,
			ExpiresAt:   now.Add(time.Hour),
		}})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListEventTriggers(context.Background(), "proj-1", "waiting")
	if err != nil {
		t.Fatalf("ListEventTriggers: %v", err)
	}
	if len(got) != 1 || got[0].ID != "et-1" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestGetEventTrigger(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/events/payment.received")
		assertAuth(t, r, "test-key")
		respondJSON(t, w, http.StatusOK, domain.EventTrigger{
			ID:          "et-1",
			EventKey:    "payment.received",
			ProjectID:   "proj-1",
			SourceType:  domain.EventSourceWorkflowStep,
			Status:      domain.EventTriggerStatusWaiting,
			TimeoutSecs: 120,
			RequestedAt: now,
			ExpiresAt:   now.Add(time.Hour),
		})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.GetEventTrigger(context.Background(), "payment.received")
	if err != nil {
		t.Fatalf("GetEventTrigger: %v", err)
	}
	if got.EventKey != "payment.received" {
		t.Fatalf("expected payment.received, got %s", got.EventKey)
	}
}

func TestSendEvent(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/events/payment.received/send")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var body map[string]map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["payload"]["order_id"] != "ord-123" {
			t.Fatalf("unexpected payload: %+v", body)
		}

		respondJSON(t, w, http.StatusOK, domain.EventTrigger{
			ID:              "et-1",
			EventKey:        "payment.received",
			ProjectID:       "proj-1",
			SourceType:      domain.EventSourceWorkflowStep,
			Status:          domain.EventTriggerStatusReceived,
			TimeoutSecs:     120,
			RequestedAt:     now,
			ReceivedAt:      &now,
			ExpiresAt:       now.Add(time.Hour),
			ResponsePayload: mustMarshal(t, map[string]any{"ok": true}),
		})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.SendEvent(context.Background(), "payment.received", map[string]any{"order_id": "ord-123"})
	if err != nil {
		t.Fatalf("SendEvent: %v", err)
	}
	if got.Status != domain.EventTriggerStatusReceived {
		t.Fatalf("expected received, got %s", got.Status)
	}
}

func TestPurgeEventTriggers(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/events/purge")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["older_than_days"] != float64(30) {
			t.Fatalf("expected older_than_days=30, got %+v", body["older_than_days"])
		}
		if body["dry_run"] != false {
			t.Fatalf("expected dry_run=false, got %+v", body["dry_run"])
		}

		respondJSON(t, w, http.StatusOK, map[string]any{"deleted": 3})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.PurgeEventTriggers(context.Background(), 30, false)
	if err != nil {
		t.Fatalf("PurgeEventTriggers: %v", err)
	}
	if got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestStats(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/stats")
		assertAuth(t, r, "test-key")
		respondJSON(t, w, http.StatusOK, QueueStats{Queued: 10, Executing: 2, Delayed: 1})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got.Queued != 10 || got.Executing != 2 || got.Delayed != 1 {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestHealth(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/health")
		assertAuth(t, r, "test-key")
		respondJSON(t, w, http.StatusOK, HealthStatus{Status: "ok"})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if got.Status != "ok" {
		t.Fatalf("expected ok, got %s", got.Status)
	}
}

func TestHealthReady(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/health/ready")
		assertAuth(t, r, "test-key")
		respondJSON(t, w, http.StatusOK, HealthStatus{Status: "ok"})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.HealthReady(context.Background())
	if err != nil {
		t.Fatalf("HealthReady: %v", err)
	}
	if got.Status != "ok" {
		t.Fatalf("expected ok, got %s", got.Status)
	}
}

func TestListWorkflowRunsByProject(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/workflow-runs")
		assertAuth(t, r, "test-key")
		if r.URL.Query().Get("project_id") != "proj-1" {
			t.Fatalf("expected project_id=proj-1, got %q", r.URL.Query().Get("project_id"))
		}
		if r.URL.Query().Get("status") != "running" {
			t.Fatalf("expected status=running, got %q", r.URL.Query().Get("status"))
		}
		if r.URL.Query().Get("limit") != "15" {
			t.Fatalf("expected limit=15, got %q", r.URL.Query().Get("limit"))
		}
		respondPaginated(t, w, http.StatusOK, []domain.WorkflowRun{{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListWorkflowRunsByProject(context.Background(), "proj-1", "running", 15)
	if err != nil {
		t.Fatalf("ListWorkflowRunsByProject: %v", err)
	}
	if len(got) != 1 || got[0].ID != "wr-1" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestPurgeEventTriggers_DryRun(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/events/purge")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["dry_run"] != true {
			t.Fatalf("expected dry_run=true, got %+v", body["dry_run"])
		}

		respondJSON(t, w, http.StatusOK, map[string]any{"would_delete": 7})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.PurgeEventTriggers(context.Background(), 14, true)
	if err != nil {
		t.Fatalf("PurgeEventTriggers dry-run: %v", err)
	}
	if got != 7 {
		t.Fatalf("expected 7, got %d", got)
	}
}

// Test helpers.

func mustClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	c, err := New(baseURL, "test-key", 10*time.Second)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func assertMethod(t *testing.T, r *http.Request, want string) {
	t.Helper()
	if r.Method != want {
		t.Fatalf("expected method %s, got %s", want, r.Method)
	}
}

func assertPath(t *testing.T, r *http.Request, want string) {
	t.Helper()
	if r.URL.Path != want {
		t.Fatalf("expected path %s, got %s", want, r.URL.Path)
	}
}

func assertAuth(t *testing.T, r *http.Request, key string) {
	t.Helper()
	want := "Bearer " + key
	if r.Header.Get("Authorization") != want {
		t.Fatalf("expected auth %q, got %q", want, r.Header.Get("Authorization"))
	}
}

func assertContentType(t *testing.T, r *http.Request) {
	t.Helper()
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}
}

func respondJSON(t *testing.T, w http.ResponseWriter, status int, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

// respondPaginated wraps data in the PaginatedResponse envelope for list endpoints.
func respondPaginated(t *testing.T, w http.ResponseWriter, status int, data any) {
	t.Helper()
	respondJSON(t, w, status, paginatedResponse{
		Data:    mustMarshal(t, data),
		HasMore: false,
	})
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// Phase 0: Deployment API tests.

func TestCreateDeploymentVersion(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/deployments")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var req CreateDeploymentVersionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.ProjectID != "proj-1" {
			t.Fatalf("expected project_id=proj-1, got %q", req.ProjectID)
		}
		if req.Environment != "production" {
			t.Fatalf("expected environment=production, got %q", req.Environment)
		}
		if req.Checksum == "" {
			t.Fatal("expected checksum to be set")
		}

		respondJSON(t, w, http.StatusOK, DeploymentVersion{
			ID:          "dep-1",
			ProjectID:   req.ProjectID,
			Environment: req.Environment,
			Status:      "pending",
			Checksum:    req.Checksum,
			CreatedAt:   now,
		})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.CreateDeploymentVersion(context.Background(), CreateDeploymentVersionRequest{
		ProjectID:   "proj-1",
		Environment: "production",
		Runtime:     "node",
		Checksum:    "sha256:abc123",
	})
	if err != nil {
		t.Fatalf("CreateDeploymentVersion: %v", err)
	}
	if got.ID != "dep-1" {
		t.Fatalf("expected dep-1, got %s", got.ID)
	}
}

func TestCreateDeploymentVersion_MissingFields(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"project_id is required"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	_, err := c.CreateDeploymentVersion(context.Background(), CreateDeploymentVersionRequest{})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("error should contain status code: %v", err)
	}
}

func TestFinalizeDeployment(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/deployments/dep-1/finalize")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var req FinalizeDeploymentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.ProjectID != "proj-1" || req.Environment != "production" {
			t.Fatalf("unexpected request: %+v", req)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	err := c.FinalizeDeployment(context.Background(), "dep-1", FinalizeDeploymentRequest{
		ProjectID:   "proj-1",
		Environment: "production",
	})
	if err != nil {
		t.Fatalf("FinalizeDeployment: %v", err)
	}
}

func TestPromoteDeployment(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/deployments/dep-1/promote")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var req PromoteDeploymentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.ProjectID != "proj-1" || req.Environment != "production" {
			t.Fatalf("unexpected request: %+v", req)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	err := c.PromoteDeployment(context.Background(), "dep-1", PromoteDeploymentRequest{ProjectID: "proj-1", Environment: "production"})
	if err != nil {
		t.Fatalf("PromoteDeployment: %v", err)
	}
}

func TestRollbackDeployment(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/deployments/dep-1/rollback")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var req RollbackDeploymentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.ProjectID != "proj-1" || req.Environment != "production" {
			t.Fatalf("unexpected request: %+v", req)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	err := c.RollbackDeployment(context.Background(), "dep-1", RollbackDeploymentRequest{ProjectID: "proj-1", Environment: "production"})
	if err != nil {
		t.Fatalf("RollbackDeployment: %v", err)
	}
}

func TestListDeployments(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/deployments")
		assertAuth(t, r, "test-key")
		if r.URL.Query().Get("project_id") != "proj-1" {
			t.Fatalf("expected project_id=proj-1, got %q", r.URL.Query().Get("project_id"))
		}
		respondPaginated(t, w, http.StatusOK, []DeploymentVersion{
			{ID: "dep-1", ProjectID: "proj-1", Environment: "production", Status: "active", CreatedAt: now},
			{ID: "dep-2", ProjectID: "proj-1", Environment: "staging", Status: "pending", CreatedAt: now},
		})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListDeployments(context.Background(), "proj-1", 0)
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 deployments, got %d", len(got))
	}
	if got[0].ID != "dep-1" {
		t.Fatalf("expected dep-1, got %s", got[0].ID)
	}
}

func TestListServerSecrets(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/secrets")
		assertAuth(t, r, "test-key")
		if r.URL.Query().Get("project_id") != "proj-1" {
			t.Fatalf("expected project_id=proj-1, got %q", r.URL.Query().Get("project_id"))
		}
		if r.URL.Query().Get("environment") != "production" {
			t.Fatalf("expected environment=production, got %q", r.URL.Query().Get("environment"))
		}
		respondPaginated(t, w, http.StatusOK, []ServerSecret{
			{ID: "sec-1", ProjectID: "proj-1", SecretKey: "DB_PASSWORD", Environment: "production", CreatedAt: now, UpdatedAt: now},
		})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListServerSecrets(context.Background(), "proj-1", "production")
	if err != nil {
		t.Fatalf("ListServerSecrets: %v", err)
	}
	if len(got) != 1 || got[0].ID != "sec-1" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestCreateServerSecret(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/secrets")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var req CreateServerSecretRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.ProjectID != "proj-1" || req.SecretKey != "API_TOKEN" || req.SecretValue != "secret123" {
			t.Fatalf("unexpected request: %+v", req)
		}

		respondJSON(t, w, http.StatusOK, ServerSecret{
			ID:          "sec-1",
			ProjectID:   req.ProjectID,
			SecretKey:   req.SecretKey,
			Environment: req.Environment,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.CreateServerSecret(context.Background(), CreateServerSecretRequest{
		ProjectID:   "proj-1",
		SecretKey:   "API_TOKEN",
		SecretValue: "secret123",
		Environment: "production",
	})
	if err != nil {
		t.Fatalf("CreateServerSecret: %v", err)
	}
	if got.ID != "sec-1" {
		t.Fatalf("expected sec-1, got %s", got.ID)
	}
}

func TestDeleteServerSecret(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodDelete)
		assertPath(t, r, "/v1/secrets/sec-1")
		assertAuth(t, r, "test-key")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	if err := c.DeleteServerSecret(context.Background(), "sec-1"); err != nil {
		t.Fatalf("DeleteServerSecret: %v", err)
	}
}

func TestGetPerformanceAnalytics(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/analytics/performance")
		assertAuth(t, r, "test-key")
		if r.URL.Query().Get("project_id") != "proj-1" {
			t.Fatalf("expected project_id=proj-1, got %q", r.URL.Query().Get("project_id"))
		}
		if r.URL.Query().Get("period_hours") != "72" {
			t.Fatalf("expected period_hours=72, got %q", r.URL.Query().Get("period_hours"))
		}
		respondJSON(t, w, http.StatusOK, PerformanceAnalytics{
			SlowestJobs: []JobPerformance{
				{JobID: "job-1", JobSlug: "process-payment", TotalRuns: 100, FailedRuns: 5, AvgDurationSecs: 1.5, P95DurationSecs: 2.3},
			},
			Throughput: ThroughputStats{Completed: 95, Failed: 5, PeriodHours: 72},
			HealthSummary: HealthSummary{
				TotalJobs:       10,
				ActiveJobs:      8,
				SuccessRate:     0.95,
				AvgDurationSecs: 1.25,
				QueueDepth:      3,
			},
		})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.GetPerformanceAnalytics(context.Background(), "proj-1", 72)
	if err != nil {
		t.Fatalf("GetPerformanceAnalytics: %v", err)
	}
	if got.Throughput.PeriodHours != 72 || len(got.SlowestJobs) != 1 || got.SlowestJobs[0].JobID != "job-1" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestListMembers(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/members")
		assertAuth(t, r, "test-key")
		if r.URL.Query().Get("project_id") != "proj-1" {
			t.Fatalf("expected project_id=proj-1, got %q", r.URL.Query().Get("project_id"))
		}
		respondPaginated(t, w, http.StatusOK, []ProjectMember{
			{ID: "mem-1", ProjectID: "proj-1", UserID: "user-1", RoleID: "role-admin", GrantedBy: "owner-1", CreatedAt: now},
		})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListMembers(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(got) != 1 || got[0].ID != "mem-1" || got[0].UserID != "user-1" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestListAuditEvents(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/audit-events")
		assertAuth(t, r, "test-key")
		if r.URL.Query().Get("project_id") != "proj-1" {
			t.Fatalf("expected project_id=proj-1, got %q", r.URL.Query().Get("project_id"))
		}
		if r.URL.Query().Get("actor_id") != "actor-1" {
			t.Fatalf("expected actor_id=actor-1, got %q", r.URL.Query().Get("actor_id"))
		}
		if r.URL.Query().Get("resource_type") != "job" || r.URL.Query().Get("resource_id") != "job-1" {
			t.Fatalf("unexpected resource filters: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("order") != "asc" {
			t.Fatalf("expected order=asc, got %q", r.URL.Query().Get("order"))
		}
		if r.URL.Query().Get("from") == "" || r.URL.Query().Get("to") == "" {
			t.Fatalf("expected from/to filters, got %s", r.URL.RawQuery)
		}
		respondPaginated(t, w, http.StatusOK, []AuditEvent{
			{ID: "ae-1", ProjectID: "proj-1", ActorID: "actor-1", ActorType: "user", Action: "job.created", ResourceType: "job", ResourceID: "job-1", CreatedAt: now},
		})
	}))
	defer srv.Close()

	from := now.Add(-time.Hour)
	to := now.Add(time.Hour)
	c := mustClient(t, srv.URL)
	got, err := c.ListAuditEvents(context.Background(), ListAuditEventsParams{
		ProjectID:    "proj-1",
		ActorID:      "actor-1",
		ResourceType: "job",
		ResourceID:   "job-1",
		Limit:        10,
		From:         &from,
		To:           &to,
		Order:        "asc",
	})
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ae-1" || got[0].ActorID != "actor-1" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestAddMember(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodPost)
		assertPath(t, r, "/v1/members")
		assertAuth(t, r, "test-key")
		assertContentType(t, r)

		var req AssignMemberRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.UserID != "user-1" || req.RoleID != "role-admin" {
			t.Fatalf("unexpected request: %+v", req)
		}

		respondJSON(t, w, http.StatusOK, ProjectMember{
			ID:        "mem-1",
			ProjectID: "proj-1",
			UserID:    req.UserID,
			RoleID:    req.RoleID,
			GrantedBy: "owner-1",
			CreatedAt: now,
		})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.AddMember(context.Background(), AssignMemberRequest{
		UserID: "user-1",
		RoleID: "role-admin",
	})
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if got.ID != "mem-1" || got.UserID != "user-1" || got.RoleID != "role-admin" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestRemoveMember(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodDelete)
		assertPath(t, r, "/v1/members/user-1")
		assertAuth(t, r, "test-key")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	if err := c.RemoveMember(context.Background(), "user-1"); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
}

func TestListRoles(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/roles")
		assertAuth(t, r, "test-key")
		if r.URL.Query().Get("project_id") != "proj-1" {
			t.Fatalf("expected project_id=proj-1, got %q", r.URL.Query().Get("project_id"))
		}
		respondPaginated(t, w, http.StatusOK, []ProjectRole{
			{ID: "role-1", Name: "admin", Description: "Full access"},
			{ID: "role-2", Name: "viewer", Description: "Read-only access"},
		})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListRoles(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if len(got) != 2 || got[0].ID != "role-1" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestDoListAllJSON_MultiplePages(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/runs/run-1/events")
		assertAuth(t, r, "test-key")

		n := callCount.Add(1)
		cursor := r.URL.Query().Get("cursor")

		if n == 1 && cursor == "" {
			nextCursor := "page2"
			respondJSON(t, w, http.StatusOK, paginatedResponse{
				Data:       mustMarshal(t, []domain.RunEvent{{ID: "evt-1"}, {ID: "evt-2"}}),
				HasMore:    true,
				NextCursor: &nextCursor,
			})
		} else if cursor == "page2" {
			respondJSON(t, w, http.StatusOK, paginatedResponse{
				Data:    mustMarshal(t, []domain.RunEvent{{ID: "evt-3"}}),
				HasMore: false,
			})
		} else {
			t.Fatalf("unexpected call: n=%d cursor=%q", n, cursor)
		}
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListRunEvents(context.Background(), "run-1", "", "")
	if err != nil {
		t.Fatalf("ListRunEvents: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d", len(got))
	}
	if got[0].ID != "evt-1" || got[1].ID != "evt-2" || got[2].ID != "evt-3" {
		t.Fatalf("unexpected events: %+v", got)
	}
}

func TestDoListAllJSON_SinglePage(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertAuth(t, r, "test-key")
		respondPaginated(t, w, http.StatusOK, []domain.RunEvent{{ID: "evt-1"}})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListRunEvents(context.Background(), "run-1", "", "")
	if err != nil {
		t.Fatalf("ListRunEvents: %v", err)
	}
	if len(got) != 1 || got[0].ID != "evt-1" {
		t.Fatalf("unexpected events: %+v", got)
	}
}

func TestDoListAllJSON_EmptyResult(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertAuth(t, r, "test-key")
		respondPaginated(t, w, http.StatusOK, []domain.RunEvent{})
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	got, err := c.ListRunEvents(context.Background(), "run-1", "", "")
	if err != nil {
		t.Fatalf("ListRunEvents: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 events, got %d", len(got))
	}
}

func TestDoListAllJSON_TruncationWarning(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertAuth(t, r, "test-key")

		n := callCount.Add(1)
		cursor := fmt.Sprintf("page%d", n+1)
		respondJSON(t, w, http.StatusOK, paginatedResponse{
			Data:       mustMarshal(t, []domain.RunEvent{{ID: fmt.Sprintf("evt-%d", n)}}),
			HasMore:    true,
			NextCursor: &cursor,
		})
	}))
	defer srv.Close()

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	c := mustClient(t, srv.URL)
	got, err := c.ListRunEvents(context.Background(), "run-1", "", "")

	_ = w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	stderrOutput := buf.String()

	if err != nil {
		t.Fatalf("ListRunEvents: %v", err)
	}
	// maxPages=100, each page has 1 event
	if len(got) != 100 {
		t.Fatalf("expected 100 events, got %d", len(got))
	}
	if !strings.Contains(stderrOutput, "truncated") {
		t.Fatalf("expected truncation warning on stderr, got: %q", stderrOutput)
	}
}

func TestDoListAllJSON_NoWarningOnNormalPagination(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertAuth(t, r, "test-key")

		n := callCount.Add(1)
		if n == 1 {
			cursor := "page2"
			respondJSON(t, w, http.StatusOK, paginatedResponse{
				Data:       mustMarshal(t, []domain.RunEvent{{ID: "evt-1"}}),
				HasMore:    true,
				NextCursor: &cursor,
			})
		} else {
			respondJSON(t, w, http.StatusOK, paginatedResponse{
				Data:    mustMarshal(t, []domain.RunEvent{{ID: "evt-2"}}),
				HasMore: false,
			})
		}
	}))
	defer srv.Close()

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	c := mustClient(t, srv.URL)
	got, err := c.ListRunEvents(context.Background(), "run-1", "", "")

	_ = w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	stderrOutput := buf.String()

	if err != nil {
		t.Fatalf("ListRunEvents: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
	if strings.Contains(stderrOutput, "truncated") {
		t.Fatalf("should not warn on normal pagination, got: %q", stderrOutput)
	}
}
