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

	"github.com/stretchr/testify/require"
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
	require.Equal(t, http.StatusCreated,

		w.Code)
	require.NotNil(t, captured)
	require.EqualValues(t, 3, captured.RetryPriorityBoost)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.Equal(t, float64(3), resp["retry_priority_boost"])

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
	require.Equal(t, http.StatusCreated,

		w.Code)
	require.NotNil(t, captured)
	require.EqualValues(t, 1, captured.RetryPriorityBoost)

	// When omitted from request, the handler defaults to 1 (matching DB default).

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
	require.Equal(t, http.StatusCreated,

		w.Code)
	require.NotNil(t, captured)
	require.EqualValues(t, 1, captured.RetryPriorityBoost)

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
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.NotNil(t, captured)
	require.EqualValues(t, 5, captured.RetryPriorityBoost)

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
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.NotNil(t, captured)
	require.EqualValues(t, 0, captured.RetryPriorityBoost)

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
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)

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
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)

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
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)

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
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)

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
	require.Equal(t, http.StatusCreated,

		w.Code)
	require.Len(t,
		captured, 2)
	require.EqualValues(t, 5, captured[0].RetryPriorityBoost)
	require.EqualValues(t, 1, captured[1].RetryPriorityBoost)

	// Sending 0 on batch create defaults to 1 (same as omitting).

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
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))

	errs, _ := resp["errors"].([]any)
	require.NotEmpty(t, errs)

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
	require.Equal(t, http.StatusCreated,

		w.Code)
	require.NotNil(t, cloned)
	require.EqualValues(t, 7, cloned.RetryPriorityBoost)

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
			require.Equal(t, tc.wantStatus,
				w.Code,
			)

		})
	}
}
