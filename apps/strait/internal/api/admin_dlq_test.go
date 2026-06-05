package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleAdminListDLQ_OK(t *testing.T) {
	t.Parallel()
	mock := &APIStoreMock{
		ListDeadLetterRunsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			require.NotEmpty(t, projectID)

			return []domain.JobRun{{
				ID: "run-1", JobID: "job-1", ProjectID: projectID,
				Status: domain.StatusDeadLetter, CreatedAt: time.Now(),
			}}, nil
		},
	}
	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/admin/dlq", "")
	r.Header.Set("X-Project-Id", "proj-admin-list")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusOK,
		w.Code)
	require.Len(t, mock.ListDeadLetterRunsCalls(), 1)
}

func TestHandleAdminReplayDLQ_NotFound(t *testing.T) {
	t.Parallel()
	mock := &APIStoreMock{
		ReplayDeadLetterRunWithAuditFunc: func(_ context.Context, _ string, _ *domain.AuditEvent) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/dlq/missing/replay", ""))
	require.Equal(t, http.
		StatusNotFound,
		w.Code,
	)
}

func TestHandleAdminReplayDLQ_Conflict(t *testing.T) {
	t.Parallel()
	mock := &APIStoreMock{
		ReplayDeadLetterRunWithAuditFunc: func(_ context.Context, _ string, _ *domain.AuditEvent) (*domain.JobRun, error) {
			return nil, store.ErrRunConflict
		},
	}
	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/dlq/run-conflict/replay", ""))
	require.Equal(t, http.
		StatusConflict,
		w.Code,
	)
}

func TestHandleAdminReplayDLQ_OK_WritesAudit(t *testing.T) {
	t.Parallel()
	// The admin replay handler delegates the CAS + lineage + audit write
	// to a single store call in one transaction; verify the audit
	// envelope it hands in carries the expected action/resource and that
	// the resolved run is returned to the caller.
	var seenAudit *domain.AuditEvent
	mock := &APIStoreMock{
		ReplayDeadLetterRunWithAuditFunc: func(_ context.Context, id string, audit *domain.AuditEvent) (*domain.JobRun, error) {
			seenAudit = audit
			return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusQueued}, nil
		},
	}
	var enqueuedExisting string
	srv := newTestServer(t, mock, &mockQueue{
		enqueueExistingFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedExisting = run.ID
			return nil
		},
	}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/dlq/run-1/replay", ""))
	require.Equal(t, http.
		StatusOK,
		w.Code)
	require.Len(t, mock.ReplayDeadLetterRunWithAuditCalls(),
		1)
	require.NotNil(t, seenAudit)
	assert.Equal(t, "dlq.replay",
		seenAudit.
			Action,
	)
	assert.Equal(t, "job_run",
		seenAudit.
			ResourceType,
	)
	assert.Equal(t, "run-1",
		seenAudit.
			ResourceID,
	)
	assert.Equal(t, "run-1",
		enqueuedExisting,
	)
}

func TestHandleAdminUnmaskDLQ_Conflict(t *testing.T) {
	t.Parallel()
	mock := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "p", Status: domain.StatusDeadLetter}, nil
		},
		UnmaskDLQRunFunc: func(_ context.Context, _ string) error {
			return errors.New("wrap: " + store.ErrRunConflict.Error())
		},
	}
	// Wrap properly via the sentinel to exercise the 409 branch.
	mock.UnmaskDLQRunFunc = func(_ context.Context, _ string) error {
		return store.ErrRunConflict
	}
	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/dlq/run-x/unmask", ""))
	require.Equal(t, http.
		StatusConflict,
		w.Code,
	)
}

func TestHandleAdminUnmaskDLQ_AuditFailureFailsRequest(t *testing.T) {
	t.Parallel()
	unmaskCalls := 0
	mock := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", JobID: "job-1", Status: domain.StatusDeadLetter}, nil
		},
		UnmaskDLQRunFunc: func(_ context.Context, _ string) error {
			unmaskCalls++
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			require.Equal(t, "dlq.unmask",
				ev.
					Action)

			return errors.New("audit unavailable")
		},
	}
	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/dlq/run-1/unmask", ""))
	require.Equal(t, http.
		StatusInternalServerError,

		w.Code)
	require.Equal(t, 1, unmaskCalls)
}

func TestHandleAdminPurgeDLQ_OK(t *testing.T) {
	t.Parallel()
	purgeCalls := 0
	mock := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", JobID: "job-1", Status: domain.StatusDeadLetter}, nil
		},
		PurgeDLQRunFunc: func(_ context.Context, _ string) error {
			purgeCalls++
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			assert.Equal(t, "dlq.purge",
				ev.
					Action)

			return nil
		},
	}
	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/dlq/run-1/purge", ""))
	require.Equal(t, http.
		StatusOK,
		w.Code)
	require.Equal(t, 1, purgeCalls)
}

func TestHandleAdminPurgeDLQ_AuditFailureFailsRequest(t *testing.T) {
	t.Parallel()
	purgeCalls := 0
	mock := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", JobID: "job-1", Status: domain.StatusDeadLetter}, nil
		},
		PurgeDLQRunFunc: func(_ context.Context, _ string) error {
			purgeCalls++
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			require.Equal(t, "dlq.purge",
				ev.
					Action)

			return errors.New("audit unavailable")
		},
	}
	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/dlq/run-1/purge", ""))
	require.Equal(t, http.
		StatusInternalServerError,

		w.Code)
	require.Equal(t, 1, purgeCalls)
}

func TestHandleAdminListDLQ_ForbiddenWithEmptyScopes(t *testing.T) {
	t.Parallel()
	// An API key provisioned with an empty (but non-nil) scopes slice
	// must NOT bypass the admin scope check. Only internal-secret
	// callers, identified by a nil scopes context value, bypass.
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{})
	if _, err := srv.handleAdminListDLQ(ctx, &ListAdminDLQInput{}); err == nil {
		require.Fail(t,

			"expected 403 for empty-but-non-nil scopes on admin list")
	}
	if _, err := srv.handleAdminReplayDLQ(ctx, &AdminDLQRunInput{RunID: "r"}); err == nil {
		require.Fail(t,

			"expected 403 for empty-but-non-nil scopes on admin replay")
	}
	if _, err := srv.handleAdminUnmaskDLQ(ctx, &AdminDLQRunInput{RunID: "r"}); err == nil {
		require.Fail(t,

			"expected 403 for empty-but-non-nil scopes on admin unmask")
	}
	if _, err := srv.handleAdminPurgeDLQ(ctx, &AdminDLQRunInput{RunID: "r"}); err == nil {
		require.Fail(t,

			"expected 403 for empty-but-non-nil scopes on admin purge")
	}
}

func TestHandleAdminListDLQ_ForbiddenWithoutScope(t *testing.T) {
	t.Parallel()
	// This request authenticates as a bearer token carrying a limited scope
	// set that does NOT include dlq:read.
	// We synthesize the failure path by calling the handler directly with a
	// context that already has restrictive scopes.
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	// Prepare a context with scopes that exclude dlq:read.
	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeRunsRead})
	_, err := srv.handleAdminListDLQ(ctx, &ListAdminDLQInput{})
	require.Error(t, err)
}

func TestHandleAdminListDLQ_EnvironmentScopeFiltersForeignRuns(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListDeadLetterRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-prod", JobID: "job-prod", ProjectID: "proj-1", Status: domain.StatusDeadLetter, CreatedAt: time.Now().Add(-time.Minute)},
				{ID: "run-staging", JobID: "job-staging", ProjectID: "proj-1", Status: domain.StatusDeadLetter, CreatedAt: time.Now().Add(-2 * time.Minute)},
			}, nil
		},
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			switch jobID {
			case "job-prod":
				return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-prod"}, nil
			case "job-staging":
				return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
			default:
				return nil, nil
			}
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(envScopedRunCtx(), ctxScopesKey, []string{domain.ScopeDLQRead})

	out, err := srv.handleAdminListDLQ(ctx, &ListAdminDLQInput{Limit: "10"})
	require.NoError(t, err)

	runs, ok := out.Body.Data.([]domain.JobRun)
	require.True(t, ok)
	require.False(t, len(runs) != 1 ||
		runs[0].ID !=
			"run-prod",
	)
}

func TestHandleAdminListDLQ_FilteredEnvironmentScopeFiltersForeignRuns(t *testing.T) {
	t.Parallel()

	filteredCalls := 0
	unfilteredCalls := 0
	ms := &APIStoreMock{
		ListDeadLetterRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			unfilteredCalls++
			return nil, nil
		},
		ListDeadLetterRunsFilteredFunc: func(_ context.Context, _ string, _ *string, masked *bool, _ int, _ *time.Time) ([]domain.JobRun, error) {
			filteredCalls++
			require.False(t, masked ==
				nil ||
				!*masked)

			return []domain.JobRun{
				{ID: "run-prod", JobID: "job-prod", ProjectID: "proj-1", Status: domain.StatusDeadLetter, CreatedAt: time.Now().Add(-time.Minute)},
				{ID: "run-staging", JobID: "job-staging", ProjectID: "proj-1", Status: domain.StatusDeadLetter, CreatedAt: time.Now().Add(-2 * time.Minute)},
			}, nil
		},
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			switch jobID {
			case "job-prod":
				return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-prod"}, nil
			case "job-staging":
				return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
			default:
				return nil, nil
			}
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(envScopedRunCtx(), ctxScopesKey, []string{domain.ScopeDLQRead})

	out, err := srv.handleAdminListDLQ(ctx, &ListAdminDLQInput{Masked: "true", Limit: "10"})
	require.NoError(t, err)

	runs, ok := out.Body.Data.([]domain.JobRun)
	require.True(t, ok)
	require.False(t, len(runs) != 1 ||
		runs[0].ID !=
			"run-prod",
	)
	require.Equal(t, 1, filteredCalls)
	require.Equal(t, 0, unfilteredCalls)
}

// TestHandleAdminPurgeDLQ_AuditWriteFailure_FailsClosed verifies that
// audit durability is part of the purge transaction.
func TestHandleAdminPurgeDLQ_AuditWriteFailure_FailsClosed(t *testing.T) {
	// Not parallel: we swap the process-wide default slog handler.
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	mock := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", JobID: "job-1", Status: domain.StatusDeadLetter}, nil
		},
		PurgeDLQRunFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return errors.New("db dead")
		},
	}
	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/dlq/run-1/purge", ""))
	require.Equal(t, http.
		StatusInternalServerError,

		w.Code)
	require.Contains(t, buf.String(), "dlq purge failed")
	require.Contains(t, buf.String(), "run_id=run-1")
}

// TestHandleAdminListDLQ_MaskedFilter verifies the masked filter is
// pushed into SQL via ListDeadLetterRunsFiltered instead of silently
// being dropped on the client side.
func TestHandleAdminListDLQ_MaskedFilter(t *testing.T) {
	t.Parallel()
	filteredCalls := 0
	unfilteredCalls := 0
	mock := &APIStoreMock{
		ListDeadLetterRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			unfilteredCalls++
			return nil, nil
		},
		ListDeadLetterRunsFilteredFunc: func(_ context.Context, projectID string, jobID *string, masked *bool, _ int, _ *time.Time) ([]domain.JobRun, error) {
			filteredCalls++
			require.NotEmpty(t, projectID)
			require.False(t, masked ==
				nil ||
				*masked !=
					true)
			require.Nil(t, jobID)

			return []domain.JobRun{{
				ID: "run-masked", JobID: "job-1", ProjectID: projectID,
				Status: domain.StatusDeadLetter, CreatedAt: time.Now(),
			}}, nil
		},
	}
	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/admin/dlq?masked=true", "")
	r.Header.Set("X-Project-Id", "proj-masked")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusOK,
		w.Code)
	require.Equal(t, 1, filteredCalls)
	require.Equal(t, 0, unfilteredCalls)
}

// TestHandleAdminListDLQ_MaskedFilter_InvalidValue rejects values
// outside {"true","false",""} so typos don't silently return unfiltered
// results.
func TestHandleAdminListDLQ_MaskedFilter_InvalidValue(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/admin/dlq?masked=yes", "")
	r.Header.Set("X-Project-Id", "proj-x")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusBadRequest,
		w.Code,
	)
}
