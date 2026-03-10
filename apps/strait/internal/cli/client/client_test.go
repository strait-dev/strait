package client

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
