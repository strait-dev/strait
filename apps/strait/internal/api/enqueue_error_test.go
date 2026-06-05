package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"

	"github.com/stretchr/testify/require"
)

func TestHandleReplayRun_EnqueueThrottledReturns429(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:         id,
				JobID:      "job-1",
				ProjectID:  "proj-1",
				Status:     domain.StatusFailed,
				CreatedAt:  now.Add(-time.Minute),
				FinishedAt: &now,
			}, nil
		},
		GetJobFunc: func(context.Context, string) (*domain.Job, error) {
			return &domain.Job{
				ID:          "job-1",
				ProjectID:   "proj-1",
				Name:        "job",
				Slug:        "job",
				Enabled:     true,
				TimeoutSecs: 60,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(context.Context, *domain.JobRun) error {
			return &queue.ThrottledError{ProjectID: "proj-1", RetryAfter: 1500 * time.Millisecond}
		},
	}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/runs/run-1/replay", `{}`, "proj-1")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusTooManyRequests,

		w.Code)
	require.Equal(t, "2", w.Header().Get("Retry-After"))

	var body struct {
		Error APIError `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, ErrorCodeEnqueueThrottled,

		body.Error.Code)
}

func TestHandleTriggerJob_EnqueueThrottledReturns429(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "job",
				Slug:        "job",
				Enabled:     true,
				TimeoutSecs: 60,
				MaxAttempts: 3,
			}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(context.Context, *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(context.Context, *domain.JobRun) error {
			return &queue.ThrottledError{ProjectID: "proj-1", RetryAfter: 2200 * time.Millisecond}
		},
	}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`, "proj-1")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusTooManyRequests,

		w.Code)
	require.Equal(t, "3", w.Header().Get("Retry-After"))

	var body struct {
		Error APIError `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, ErrorCodeEnqueueThrottled,

		body.Error.Code)
}

func TestHandleBulkTriggerJob_EnqueueThrottledReturns429(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "job",
				Slug:        "job",
				Enabled:     true,
				TimeoutSecs: 60,
				MaxAttempts: 3,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{
		enqueueBatchFn: func(context.Context, []*domain.JobRun) (int64, error) {
			return 0, &queue.ThrottledError{ProjectID: "proj-1", RetryAfter: 1500 * time.Millisecond}
		},
	}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", `{"items":[{"payload":{"n":1}}]}`, "proj-1")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusTooManyRequests,

		w.Code)
	require.Equal(t, "2", w.Header().Get("Retry-After"))

	var body struct {
		Error APIError `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, ErrorCodeEnqueueThrottled,

		body.Error.Code)
}
