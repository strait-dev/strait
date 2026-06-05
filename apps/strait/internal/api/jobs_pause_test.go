package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// Pause endpoint tests.

func TestHandlePauseJob_Success(t *testing.T) {
	t.Parallel()
	pausedJob := &domain.Job{ID: "job-1", ProjectID: "proj-1", Enabled: true, Paused: true}
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, Paused: false}, nil
			}
			return pausedJob, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{"reason":"investigating timeout spike"}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&body))
	require.Equal(t, "job-1", body["id"])
	require.Equal(t, true, body["paused"])
}

func TestHandlePauseJob_ReturnsFullJobWithEnabledField(t *testing.T) {
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, Paused: false}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, Paused: true, PauseReason: "test"}, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{"reason":"test"}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&body))

	// The response must include both enabled and paused so the user can see the full state.
	if _, ok := body["enabled"]; !ok {
		require.Fail(t,

			"expected 'enabled' field in pause response")
	}
	if _, ok := body["paused"]; !ok {
		require.Fail(t,

			"expected 'paused' field in pause response")
	}
}

func TestHandlePauseJob_WithReason(t *testing.T) {
	t.Parallel()
	var capturedReason string
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false}, nil
		},
		PauseJobFunc: func(_ context.Context, _ string, reason string) error {
			capturedReason = reason
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{"reason":"deploy in progress"}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.Equal(t, "deploy in progress",

		capturedReason,
	)
}

func TestHandlePauseJob_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/nonexistent/pause", `{}`))
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestHandlePauseJob_AlreadyPaused(t *testing.T) {
	t.Parallel()
	var pauseCalled bool
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true, Enabled: false}, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			pauseCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.False(t, pauseCalled)

	// Even when idempotent, the response should return the full job
	// so the caller can see that enabled=false.
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&body))
	require.Equal(t, false, body["enabled"])
}

func TestHandlePauseJob_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false}, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return fmt.Errorf("db connection lost")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{}`))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}

func TestHandlePauseJob_EmitsAuditEvent(t *testing.T) {
	t.Parallel()
	var capturedEvent *domain.AuditEvent
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false}, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			capturedEvent = ev
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{"reason":"incident"}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.NotNil(t, capturedEvent)
	require.Equal(t, "job.paused",
		capturedEvent.
			Action,
	)
	require.Equal(t, "job", capturedEvent.
		ResourceType,
	)
	require.Equal(t, "job-1", capturedEvent.
		ResourceID,
	)

	var details map[string]any
	require.NoError(t, json.Unmarshal(capturedEvent.
		Details,
		&details))
	require.Equal(t, "incident", details["reason"])
}

// Resume endpoint tests.

func TestHandleResumeJob_Success(t *testing.T) {
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true, Enabled: true}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false, Enabled: true}, nil
		},
		ResumeJobFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&body))
	require.Equal(t, "job-1", body["id"])
	require.Equal(t, false, body["paused"])
}

func TestHandleResumeJob_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/nonexistent/resume", ""))
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestHandleResumeJob_NotPaused(t *testing.T) {
	t.Parallel()
	var resumeCalled bool
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false, Enabled: true}, nil
		},
		ResumeJobFunc: func(_ context.Context, _ string) error {
			resumeCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.False(t, resumeCalled)
}

func TestHandleResumeJob_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true}, nil
		},
		ResumeJobFunc: func(_ context.Context, _ string) error {
			return fmt.Errorf("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}

func TestHandleResumeJob_EmitsAuditEvent(t *testing.T) {
	t.Parallel()
	var capturedEvent *domain.AuditEvent
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true}, nil
		},
		ResumeJobFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			capturedEvent = ev
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.NotNil(t, capturedEvent)
	require.Equal(t, "job.resumed",
		capturedEvent.
			Action,
	)
	require.Equal(t, "job-1", capturedEvent.
		ResourceID,
	)
}

// GET includes pause state.

func TestGetJob_IncludesPauseState(t *testing.T) {
	t.Parallel()
	pausedAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Name: "test-job", Slug: "test-job",
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
				Enabled: true, Paused: true, PausedAt: &pausedAt, PauseReason: "incident investigation",
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-1", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&body))
	require.Equal(t, true, body["paused"])
	require.NotNil(t, body["paused_at"])
	require.Equal(t, "incident investigation",

		body["pause_reason"])
}

func TestGetJob_UnpausedOmitsPauseFields(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Name: "test-job", Slug: "test-job",
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
				Enabled: true, Paused: false,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-1", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&body))
	require.Equal(t, false, body["paused"])

	if _, ok := body["paused_at"]; ok {
		require.Fail(t,

			"expected paused_at to be omitted for unpaused job")
	}
	if _, ok := body["pause_reason"]; ok {
		require.Fail(t,

			"expected pause_reason to be omitted for unpaused job")
	}
}

func TestListJobs_IncludesPauseState(t *testing.T) {
	t.Parallel()
	pausedAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	ms := &APIStoreMock{
		ListJobsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.Job, error) {
			return []domain.Job{{
				ID: "job-1", ProjectID: "proj-1", Name: "paused-job", Slug: "paused-job",
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
				Enabled: true, Paused: true, PausedAt: &pausedAt, PauseReason: "deploy",
			}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/jobs", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&resp))
	require.NotEmpty(t, resp.Data)
	require.Equal(t, true, resp.Data[0]["paused"])
	require.Equal(t, "deploy", resp.
		Data[0]["pause_reason"])
}

// Edge case: trigger rejection for paused jobs.

func TestTriggerJob_RejectedWhenPaused(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, Paused: true,
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`))
	require.Equal(t, http.StatusConflict,

		w.Code)
}

func TestTriggerJob_AllowedWhenNotPaused(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, Paused: false,
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{MaxQueuedRuns: 1000}, nil
		},
		CountProjectQueuedRunsFunc: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		FindRecentRunByPayloadFunc: func(_ context.Context, _ string, _ json.RawMessage, _ time.Time) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
		CountRunsForJobSinceFunc: func(_ context.Context, _ string, _ time.Time) (int, error) {
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`))
	require.Equal(t, http.StatusCreated,

		w.Code)

	// Should succeed (not 409) since the job is not paused.
}

func TestBulkTriggerJob_RejectedWhenPaused(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, Paused: true,
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", `{"items":[{}]}`))
	require.Equal(t, http.StatusConflict,

		w.Code)
}

// Edge case: enabled + paused double gate.

func TestResumeJob_ShowsDisabledState(t *testing.T) {
	// A job that was paused AND disabled. Resuming clears pause but
	// the response must show enabled=false so the user knows it's still disabled.
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true, Enabled: false}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false, Enabled: false}, nil
		},
		ResumeJobFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&body))
	require.Equal(t, false, body["paused"])
	require.Equal(t, false, body["enabled"])
}

// Edge case: group pause + individual resume.

func TestGroupPause_IndividualResume(t *testing.T) {
	t.Parallel()

	// After group pause, individual resume should work and return the job.
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				// First call: job is group-paused.
				return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true, PauseReason: "group pause", Enabled: true}, nil
			}
			// After resume:
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false, Enabled: true}, nil
		},
		ResumeJobFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&body))
	require.Equal(t, false, body["paused"])
}

// Adversarial tests.

func TestTriggerJob_DryRunRejectedWhenPaused(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, Paused: true,
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"dry_run":true}`))
	require.NotEqual(t, http.StatusOK,
		w.
			Code)

	// Dry-run must also reject paused jobs. If it returned 200, the user
	// would think triggering is possible when it isn't.
}

func TestTriggerJob_DryRunAllowedWhenNotPaused(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, Paused: false,
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{MaxQueuedRuns: 1000}, nil
		},
		CountProjectQueuedRunsFunc: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		FindRecentRunByPayloadFunc: func(_ context.Context, _ string, _ json.RawMessage, _ time.Time) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
		CountRunsForJobSinceFunc: func(_ context.Context, _ string, _ time.Time) (int, error) {
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"dry_run":true}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
}

func TestPauseJob_IdempotentAlwaysReturnsFreshState(t *testing.T) {
	// When pausing an already-paused job, the response must come from a
	// fresh re-fetch, not from the initial check. This ensures consistent
	// state even under concurrent modifications.
	t.Parallel()
	freshPausedAt := time.Date(2026, 3, 24, 15, 0, 0, 0, time.UTC)
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			// Both calls return a paused job, but the second call returns
			// an updated timestamp (simulating concurrent modification).
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Paused: true,
				PausedAt: &freshPausedAt, PauseReason: "latest reason",
				Enabled: true,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&body))
	require.Equal(t, "latest reason",
		body["pause_reason"])

	// Verify the response has the fresh state.
}

func TestResumeJob_IdempotentAlwaysReturnsFreshState(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Paused: false, Enabled: true,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&body))
	require.Equal(t, false, body["paused"])
	require.Equal(t, true, body["enabled"])
}

func TestPauseJob_NoAuditEventWhenAlreadyPaused(t *testing.T) {
	t.Parallel()
	var auditCalled bool
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true, Enabled: true}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			auditCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{"reason":"duplicate"}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.False(t, auditCalled)
}

func TestResumeJob_NoAuditEventWhenNotPaused(t *testing.T) {
	t.Parallel()
	var auditCalled bool
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false, Enabled: true}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			auditCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.False(t, auditCalled)
}

func TestPauseJob_DisabledJobCanBePaused(t *testing.T) {
	// A disabled job should still be pausable. These are independent states.
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: false, Paused: false}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: false, Paused: true, PauseReason: "maintenance"}, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{"reason":"maintenance"}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&body))
	require.Equal(t, false, body["enabled"])
	require.Equal(t, true, body["paused"])
}

func TestPauseDisableResume_JobStillDisabled(t *testing.T) {
	// Lifecycle: pause -> disable -> resume. After resume, job should
	// be enabled=false, paused=false. Runs still won't dequeue because
	// enabled=false. The resume response makes this visible.
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: false, Paused: true}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: false, Paused: false}, nil
		},
		ResumeJobFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/resume", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&body))
	require.Equal(t, false, body["paused"])
	require.Equal(t, false, body["enabled"])
}

func TestTriggerJob_DisabledCheckBeforePausedCheck(t *testing.T) {
	// When a job is both disabled and paused, the disabled check should
	// fire first (400) rather than the paused check (409).
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: false, Paused: true,
				EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestPauseJob_EmptyReasonIsAccepted(t *testing.T) {
	t.Parallel()
	var capturedReason string
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true}, nil
		},
		PauseJobFunc: func(_ context.Context, _ string, reason string) error {
			capturedReason = reason
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.Empty(t, capturedReason)
}

func TestPauseJob_EmptyBodyIsAccepted(t *testing.T) {
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true}, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
}

func TestPauseJob_GetJobFailsAfterPause(t *testing.T) {
	// If the re-fetch after PauseJob fails, we should get 500.
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false}, nil
			}
			return nil, fmt.Errorf("db gone")
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", `{}`))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}

// Reason length validation.

func TestPauseJob_ReasonTooLong(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	longReason := strings.Repeat("a", 501)
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", fmt.Sprintf(`{"reason":%q}`, longReason)))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestPauseJob_ReasonExactly500Chars(t *testing.T) {
	t.Parallel()
	callCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			callCount++
			if callCount == 1 {
				return &domain.Job{ID: id, ProjectID: "proj-1", Paused: false}, nil
			}
			return &domain.Job{ID: id, ProjectID: "proj-1", Paused: true}, nil
		},
		PauseJobFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	reason500 := strings.Repeat("b", 500)
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/pause", fmt.Sprintf(`{"reason":%q}`, reason500)))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
}

func TestPauseJob_ReasonValidationBeforeGetJob(t *testing.T) {
	// Reason validation should happen before the DB call, so even a
	// nonexistent job should get 400 (not 404) for an oversized reason.
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	longReason := strings.Repeat("x", 501)
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/nonexistent/pause", fmt.Sprintf(`{"reason":%q}`, longReason)))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

// Event source dispatch paused bypass.

func TestEventDispatch_SkipsPausedJob(t *testing.T) {
	t.Parallel()
	var enqueueCalled bool
	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
			return &domain.EventSource{
				ID: "src-1", ProjectID: projectID, Name: name, Enabled: true,
			}, nil
		},
		ListEventSubscriptionsBySourceFunc: func(_ context.Context, _ string) ([]domain.EventSubscription, error) {
			return []domain.EventSubscription{
				{
					ID: "sub-1", SourceID: "src-1", TargetType: "job", TargetID: "job-1",
					Enabled: true,
				},
			}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, Paused: true,
				Version: 1, VersionID: "jv-1",
			}, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueueCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	body := `{"source":"my-source","project_id":"proj-1","payload":{"type":"deploy"}}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", body))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.False(t, enqueueCalled)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.Equal(t, 0, int(resp["dispatched"].(float64)))
}

func TestEventDispatch_EnqueuesWhenNotPaused(t *testing.T) {
	t.Parallel()
	var enqueueCalled bool
	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
			return &domain.EventSource{
				ID: "src-1", ProjectID: projectID, Name: name, Enabled: true,
			}, nil
		},
		ListEventSubscriptionsBySourceFunc: func(_ context.Context, _ string) ([]domain.EventSubscription, error) {
			return []domain.EventSubscription{
				{
					ID: "sub-1", SourceID: "src-1", TargetType: "job", TargetID: "job-1",
					Enabled: true,
				},
			}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, Paused: false,
				Version: 1, VersionID: "jv-1",
			}, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueueCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	body := `{"source":"my-source","project_id":"proj-1","payload":{"type":"deploy"}}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", body))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.True(
		t, enqueueCalled)
}
