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
)

func TestCreateJob_WithRetryPriorityBoost(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			captured = job
			job.ID = "job-boost-1"
			job.Version = 1
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "Boost Job",
		"slug": "boost-job",
		"endpoint_url": "https://example.com/callback",
		"retry_priority_boost": 3
	}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if captured == nil {
		t.Fatal("CreateJob was not called")
	}
	if captured.RetryPriorityBoost != 3 {
		t.Fatalf("expected retry_priority_boost=3, got %d", captured.RetryPriorityBoost)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["retry_priority_boost"] != float64(3) {
		t.Fatalf("expected retry_priority_boost=3 in response, got %v", resp["retry_priority_boost"])
	}
}

func TestCreateJob_DefaultRetryPriorityBoost(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			captured = job
			job.ID = "job-default-boost"
			job.Version = 1
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "Default Boost Job",
		"slug": "default-boost-job",
		"endpoint_url": "https://example.com/callback"
	}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if captured == nil {
		t.Fatal("CreateJob was not called")
	}
	// When omitted from request, the handler defaults to 1 (matching DB default).
	if captured.RetryPriorityBoost != 1 {
		t.Fatalf("expected retry_priority_boost=1 (handler default), got %d", captured.RetryPriorityBoost)
	}
}

func TestCreateJob_RetryPriorityBoostZeroDefaultsToOne(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			captured = job
			job.ID = "job-zero-boost"
			job.Version = 1
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Sending 0 on create is treated the same as omitting (defaults to 1).
	// To disable boost, create with default then update to 0 via PATCH.
	body := `{
		"project_id": "proj-1",
		"name": "Zero Boost Job",
		"slug": "zero-boost-job",
		"endpoint_url": "https://example.com/callback",
		"retry_priority_boost": 0
	}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if captured == nil {
		t.Fatal("CreateJob was not called")
	}
	if captured.RetryPriorityBoost != 1 {
		t.Fatalf("expected retry_priority_boost=1 (default, 0 is indistinguishable from omitted), got %d", captured.RetryPriorityBoost)
	}
}

func TestUpdateJob_RetryPriorityBoost(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:                 id,
				ProjectID:          "proj-1",
				Name:               "Original Job",
				Slug:               "original-job",
				EndpointURL:        "https://example.com/callback",
				RetryPriorityBoost: 1,
				Enabled:            true,
				Version:            1,
				CreatedAt:          time.Now(),
				UpdatedAt:          time.Now(),
			}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
		UpdateJobFunc: func(_ context.Context, job *domain.Job) error {
			captured = job
			job.Version = 2
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"retry_priority_boost": 5}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", body))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if captured == nil {
		t.Fatal("UpdateJob was not called")
	}
	if captured.RetryPriorityBoost != 5 {
		t.Fatalf("expected retry_priority_boost=5, got %d", captured.RetryPriorityBoost)
	}
}

func TestUpdateJob_RetryPriorityBoostToZero(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:                 id,
				ProjectID:          "proj-1",
				Name:               "Boost Job",
				Slug:               "boost-job",
				EndpointURL:        "https://example.com/callback",
				RetryPriorityBoost: 3,
				Enabled:            true,
				Version:            1,
				CreatedAt:          time.Now(),
				UpdatedAt:          time.Now(),
			}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
		UpdateJobFunc: func(_ context.Context, job *domain.Job) error {
			captured = job
			job.Version = 2
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"retry_priority_boost": 0}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-456", body))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if captured == nil {
		t.Fatal("UpdateJob was not called")
	}
	if captured.RetryPriorityBoost != 0 {
		t.Fatalf("expected retry_priority_boost=0, got %d", captured.RetryPriorityBoost)
	}
}

func TestCreateJob_RejectNegativeBoost(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "Negative Boost Job",
		"slug": "negative-boost-job",
		"endpoint_url": "https://example.com/callback",
		"retry_priority_boost": -1
	}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for negative boost, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateJob_RejectBoostOver10(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "Over Max Boost Job",
		"slug": "over-max-boost-job",
		"endpoint_url": "https://example.com/callback",
		"retry_priority_boost": 11
	}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for boost > 10, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateJob_RejectNegativeBoost(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Test Job",
				Slug:        "test-job",
				EndpointURL: "https://example.com/callback",
				Enabled:     true,
				Version:     1,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"retry_priority_boost": -1}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-789", body))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for negative boost, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateJob_RejectBoostOver10(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Test Job",
				Slug:        "test-job",
				EndpointURL: "https://example.com/callback",
				Enabled:     true,
				Version:     1,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"retry_priority_boost": 11}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-789", body))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for boost > 10, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBatchCreateJobs_RetryPriorityBoost(t *testing.T) {
	t.Parallel()

	var captured []*domain.Job
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			cp := *job
			captured = append(captured, &cp)
			job.ID = fmt.Sprintf("job-batch-%d", len(captured))
			job.Version = 1
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"jobs": [
			{
				"project_id": "proj-1",
				"name": "Batch Job 1",
				"slug": "batch-job-1",
				"endpoint_url": "https://example.com/callback",
				"retry_priority_boost": 5
			},
			{
				"project_id": "proj-1",
				"name": "Batch Job 2",
				"slug": "batch-job-2",
				"endpoint_url": "https://example.com/callback",
				"retry_priority_boost": 0
			}
		]
	}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(captured) != 2 {
		t.Fatalf("expected 2 jobs created, got %d", len(captured))
	}
	if captured[0].RetryPriorityBoost != 5 {
		t.Fatalf("batch job 1: expected retry_priority_boost=5, got %d", captured[0].RetryPriorityBoost)
	}
	// Sending 0 on batch create defaults to 1 (same as omitting).
	if captured[1].RetryPriorityBoost != 1 {
		t.Fatalf("batch job 2: expected retry_priority_boost=1 (default), got %d", captured[1].RetryPriorityBoost)
	}
}

func TestBatchCreateJobs_RejectInvalidBoost(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			job.ID = "job-batch-valid"
			job.Version = 1
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"jobs": [
			{
				"project_id": "proj-1",
				"name": "Valid Job",
				"slug": "valid-job",
				"endpoint_url": "https://example.com/callback",
				"retry_priority_boost": 3
			},
			{
				"project_id": "proj-1",
				"name": "Invalid Job",
				"slug": "invalid-job",
				"endpoint_url": "https://example.com/callback",
				"retry_priority_boost": 15
			}
		]
	}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch", body))

	// Batch create should create the valid job and report error for invalid one
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	errs, _ := resp["errors"].([]any)
	if len(errs) == 0 {
		t.Fatal("expected validation error for invalid boost in batch, got none")
	}
}

func TestCloneJob_PreservesRetryPriorityBoost(t *testing.T) {
	t.Parallel()

	var cloned *domain.Job
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:                 id,
				ProjectID:          "proj-1",
				Name:               "Source Job",
				Slug:               "source-job",
				EndpointURL:        "https://example.com/callback",
				RetryPriorityBoost: 7,
				MaxAttempts:        5,
				TimeoutSecs:        60,
				Enabled:            true,
				Version:            1,
				CreatedAt:          time.Now(),
				UpdatedAt:          time.Now(),
			}, nil
		},
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			cp := *job
			cloned = &cp
			job.ID = "job-cloned"
			job.Version = 1
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"name": "Cloned Job", "slug": "cloned-job"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-source/clone", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if cloned == nil {
		t.Fatal("CreateJob was not called for clone")
	}
	if cloned.RetryPriorityBoost != 7 {
		t.Fatalf("expected cloned retry_priority_boost=7 (from source), got %d", cloned.RetryPriorityBoost)
	}
}

func TestCreateJob_RetryPriorityBoostBoundaryValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		boost      int
		wantStatus int
	}{
		{"zero_defaults_to_one", 0, http.StatusCreated},
		{"valid_one", 1, http.StatusCreated},
		{"valid_five", 5, http.StatusCreated},
		{"max_valid_ten", 10, http.StatusCreated},
		{"over_max_eleven", 11, http.StatusUnprocessableEntity},
		{"negative_one", -1, http.StatusUnprocessableEntity},
		{"large_negative", -100, http.StatusUnprocessableEntity},
		{"large_positive", 100, http.StatusUnprocessableEntity},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ms := &APIStoreMock{
				CreateJobFunc: func(_ context.Context, job *domain.Job) error {
					job.ID = "job-boundary"
					job.Version = 1
					job.CreatedAt = time.Now()
					job.UpdatedAt = time.Now()
					return nil
				},
			}
			srv := newTestServer(t, ms, &mockQueue{}, nil)

			body := fmt.Sprintf(`{
				"project_id": "proj-1",
				"name": "Boundary Job %d",
				"slug": "boundary-job-%d",
				"endpoint_url": "https://example.com/callback",
				"retry_priority_boost": %d
			}`, tc.boost, tc.boost, tc.boost)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

			if w.Code != tc.wantStatus {
				t.Fatalf("boost=%d: expected %d, got %d: %s", tc.boost, tc.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}
