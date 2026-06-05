package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleResetIdempotencyKey_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ResetRunIdempotencyKeyFunc: func(_ context.Context, runID string) error {
			require.Equal(t, "run-abc", runID)

			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-abc/idempotency-key", ""))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Contains(
		t, w.Body.String(), `"status":"reset"`)
}

func TestHandleResetIdempotencyKey_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ResetRunIdempotencyKeyFunc: func(_ context.Context, _ string) error {
			return store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-missing/idempotency-key", ""))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

func TestHandleRescheduleRun_Success(t *testing.T) {
	t.Parallel()
	scheduledAt := time.Now().Add(1 * time.Hour).Truncate(time.Second)
	ms := &APIStoreMock{
		RescheduleRunFunc: func(_ context.Context, runID string, at time.Time, _ json.RawMessage) error {
			require.Equal(t, "run-r1", runID)

			return nil
		},
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:          id,
				JobID:       "job-1",
				ProjectID:   "proj-1",
				Status:      domain.StatusDelayed,
				ScheduledAt: &scheduledAt,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	body := `{"scheduled_at":"` + scheduledAt.Format(time.RFC3339) + `"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-r1/reschedule", body))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Contains(
		t, w.Body.String(), "run-r1")
}

func TestHandleRescheduleRun_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
		RescheduleRunFunc: func(_ context.Context, _ string, _ time.Time, _ json.RawMessage) error {
			return store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	body := `{"scheduled_at":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-gone/reschedule", body))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

func TestHandleRescheduleRun_InvalidBody(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-x/reschedule", ""))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}

func TestHandleBulkTrigger_WithTTL(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var enqueuedRuns []*domain.JobRun

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			mu.Lock()
			enqueuedRuns = append(enqueuedRuns, run)
			mu.Unlock()
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	body := `{"items":[{"ttl_secs":120}]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t,
		enqueuedRuns, 1,
	)

	run := enqueuedRuns[0]
	require.NotNil(t, run.ExpiresAt)

	// TTL of 120s means ExpiresAt should be ~120s from now, give generous tolerance.
	diff := time.Until(*run.ExpiresAt)
	require.False(t, diff < 100*time.
		Second ||
		diff > 130*time.Second,
	)
}

func TestHandleListRuns_TriggeredByFilter(t *testing.T) {
	t.Parallel()

	var capturedTriggeredBy *string
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, triggeredBy, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedTriggeredBy = triggeredBy
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?triggered_by=api", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.NotNil(t, capturedTriggeredBy)
	require.Equal(t, "api", *capturedTriggeredBy)
}

func TestHandleListRuns_ExecutionModeFilter_HTTP(t *testing.T) {
	t.Parallel()

	var capturedMode *domain.ExecutionMode
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, em *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedMode = em
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?execution_mode=http", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.False(t, capturedMode ==
		nil || *capturedMode !=
		domain.
			ExecutionModeHTTP,
	)
}

func TestHandleListRuns_ExecutionModeFilter_Invalid(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?execution_mode=invalid", "", "proj-1"))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

func TestHandleListRuns_ExecutionModeFilter_NoFilter(t *testing.T) {
	t.Parallel()

	var capturedMode *domain.ExecutionMode
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, em *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedMode = em
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Nil(t, capturedMode)
}

func TestHandleListRuns_StatusesMultiValueFiltersResults_Unit(t *testing.T) {
	t.Parallel()

	var capturedStatus *domain.RunStatus
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, status *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedStatus = status
			return []domain.JobRun{
				{ID: "run-failed", Status: domain.StatusFailed, CreatedAt: time.Now().Add(-time.Minute)},
				{ID: "run-timed-out", Status: domain.StatusTimedOut, CreatedAt: time.Now().Add(-2 * time.Minute)},
				{ID: "run-completed", Status: domain.StatusCompleted, CreatedAt: time.Now().Add(-3 * time.Minute)},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?statuses[]=failed&statuses[]=timed_out", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Nil(t, capturedStatus)
	require.Empty(t,
		ms.ListRunsByTagCalls())

	var resp struct {
		Data []domain.JobRun `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t,
		resp.Data, 2)
	require.False(t, resp.Data[0].
		ID != "run-failed" ||
		resp.Data[1].ID != "run-timed-out",
	)
}

func TestHandleListRuns_TagFilterComposesWithExecutionMode(t *testing.T) {
	t.Parallel()

	var capturedMode *domain.ExecutionMode
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, executionMode *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedMode = executionMode
			return []domain.JobRun{
				{ID: "run-infra", Status: domain.StatusFailed, Tags: map[string]string{"team": "infra"}, CreatedAt: time.Now().Add(-time.Minute)},
				{ID: "run-app", Status: domain.StatusFailed, Tags: map[string]string{"team": "app"}, CreatedAt: time.Now().Add(-2 * time.Minute)},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?tag_key=team&tag_value=infra&execution_mode=worker", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.False(t, capturedMode ==
		nil || *capturedMode !=
		domain.
			ExecutionModeWorker,
	)
	require.Empty(t,
		ms.ListRunsByTagCalls())

	var resp struct {
		Data []domain.JobRun `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.False(t, len(resp.Data) != 1 || resp.
		Data[0].ID != "run-infra",
	)
}

func TestHandleBulkTrigger_WithConcurrencyKey(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var enqueuedRuns []*domain.JobRun

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			mu.Lock()
			enqueuedRuns = append(enqueuedRuns, run)
			mu.Unlock()
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	body := `{"items":[{"concurrency_key":"tenant-42"}]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t,
		enqueuedRuns, 1,
	)
	require.Equal(t, "tenant-42",
		enqueuedRuns[0].ConcurrencyKey)
}

func TestHandleTrigger_DefaultRunMetadataMerge(t *testing.T) {
	t.Parallel()
	var enqueued *domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:                 id,
				ProjectID:          "proj-1",
				Enabled:            true,
				TimeoutSecs:        60,
				DefaultRunMetadata: map[string]string{"env": "prod", "dependency_key": "default-dep"},
			}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = run
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	// Payload includes dependency_key which becomes run metadata; it should win over the job default.
	body := `{"payload":{"dependency_key":"user-dep"}}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, enqueued)
	require.Equal(t, "prod", enqueued.
		Metadata["env"])
	require.Equal(t, "user-dep", enqueued.
		Metadata["dependency_key"])
}

func TestHandleBulkTrigger_BatchIDSet(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var enqueuedRuns []*domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			mu.Lock()
			enqueuedRuns = append(enqueuedRuns, run)
			mu.Unlock()
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	body := `{"items":[{},{}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t,
		enqueuedRuns, 2,
	)
	require.NotEmpty(t, enqueuedRuns[0].BatchID)
	require.Equal(t, enqueuedRuns[1].BatchID,
		enqueuedRuns[0].BatchID,
	)
}

func TestHandleCreateJob_MaxConcurrencyPerKey(t *testing.T) {
	t.Parallel()
	var created *domain.Job
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			job.ID = "job-new"
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			created = job
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{
		"project_id": "proj-1",
		"name": "Job with PerKey",
		"slug": "job-per-key",
		"endpoint_url": "https://example.com/callback",
		"max_concurrency_per_key": 5
	}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, created)
	require.Equal(t, 5, created.MaxConcurrencyPerKey)
}

func TestParseBracketParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		param  string
		prefix string
		wantK  string
		wantOK bool
	}{
		{"metadata[env]", "metadata", "env", true},
		{"metadata[customer_id]", "metadata", "customer_id", true},
		{"tags[team]", "tags", "team", true},
		{"metadata[]", "metadata", "", false},
		{"metadata", "metadata", "", false},
		{"other[key]", "metadata", "", false},
		{"metadata[key", "metadata", "", false},
		{"status", "metadata", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.param, func(t *testing.T) {
			t.Parallel()
			k, ok := parseBracketParam(tt.param, tt.prefix)
			assert.False(
				t, ok != tt.wantOK ||
					k != tt.
						wantK)
		})
	}
}

func TestHandlePauseRun_HTTPRun_CanBePaused(t *testing.T) {
	t.Parallel()

	getCalls := 0
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting, ExecutionMode: domain.ExecutionModeHTTP}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusPaused, ExecutionMode: domain.ExecutionModeHTTP}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/pause", ""))
	require.Equal(t, http.StatusOK,
		w.Code)
}

func TestHandlePauseRun_AlreadyPaused(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusPaused, ExecutionMode: domain.ExecutionModeHTTP}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/pause", ""))
	require.Equal(t, http.StatusOK,
		w.Code)
}

func TestHandlePauseRun_TerminalRun(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusCompleted, ExecutionMode: domain.ExecutionModeHTTP}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/pause", ""))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

func TestHandleResumeRun_RequeuesRun(t *testing.T) {
	t.Parallel()

	getCalls := 0
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusPaused, ExecutionMode: domain.ExecutionModeHTTP}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeHTTP}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			require.False(t, from != domain.
				StatusPaused ||
				to != domain.StatusQueued,
			)

			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/resume", ""))
	require.Equal(t, http.StatusOK,
		w.Code)
}

func TestHandleResumeRun_NotPaused(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting, ExecutionMode: domain.ExecutionModeHTTP}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/resume", ""))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

func TestHandleRestartRun_WrongStatus_Rejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusCompleted, ExecutionMode: domain.ExecutionModeHTTP}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/restart", `{}`))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

func TestHandleResumeRun_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-gone/resume", ""))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

func TestHandleResumeRun_AlreadyQueued(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusQueued}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/resume", ""))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

func TestHandlePauseRun_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-gone/pause", ""))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

func TestHandleListRuns_ErrorClassFilter(t *testing.T) {
	t.Parallel()
	var capturedErrorClass *string
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, errorClass *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedErrorClass = errorClass
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?error_class=timeout", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.False(t, capturedErrorClass ==
		nil ||
		*capturedErrorClass !=
			"timeout",
	)
}

func TestHandleListRuns_ErrorClassFilterEmpty(t *testing.T) {
	t.Parallel()
	var capturedErrorClass *string
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, errorClass *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			capturedErrorClass = errorClass
			return []domain.JobRun{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Nil(t, capturedErrorClass)
}

func TestHandleListRuns_ErrorClassFilterInvalid(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs?error_class=invalid_class", "", "proj-1"))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}
