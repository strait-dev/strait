package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestHandleAdminListDLQ_OK(t *testing.T) {
	t.Parallel()
	mock := &APIStoreMock{
		ListDeadLetterRunsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			if projectID == "" {
				t.Fatalf("projectID must be propagated")
			}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(mock.ListDeadLetterRunsCalls()) != 1 {
		t.Fatalf("expected one store call, got %d", len(mock.ListDeadLetterRunsCalls()))
	}
}

func TestHandleAdminReplayDLQ_NotFound(t *testing.T) {
	t.Parallel()
	mock := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/dlq/missing/replay", ""))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAdminReplayDLQ_Conflict(t *testing.T) {
	t.Parallel()
	mock := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "p", Status: domain.StatusQueued}, nil
		},
		ReplayDeadLetterRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunConflict
		},
	}
	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/dlq/run-conflict/replay", ""))
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAdminReplayDLQ_OK_WritesAudit(t *testing.T) {
	t.Parallel()
	auditCalls := 0
	markReplayedCalls := 0
	mock := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusDeadLetter}, nil
		},
		ReplayDeadLetterRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusQueued}, nil
		},
		MarkRunReplayedFunc: func(_ context.Context, _, _ string) error {
			markReplayedCalls++
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			auditCalls++
			if ev.Action != "dlq.replay" {
				t.Errorf("unexpected action: %s", ev.Action)
			}
			return nil
		},
	}
	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/dlq/run-1/replay", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if auditCalls != 1 {
		t.Errorf("expected 1 audit call, got %d", auditCalls)
	}
	if markReplayedCalls != 1 {
		t.Errorf("expected 1 MarkRunReplayed call, got %d", markReplayedCalls)
	}
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
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
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
			if ev.Action != "dlq.purge" {
				t.Errorf("unexpected action: %s", ev.Action)
			}
			return nil
		},
	}
	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/dlq/run-1/purge", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if purgeCalls != 1 {
		t.Fatalf("expected 1 purge call, got %d", purgeCalls)
	}
}

func TestHandleAdminListDLQ_ForbiddenWithEmptyScopes(t *testing.T) {
	t.Parallel()
	// An API key provisioned with an empty (but non-nil) scopes slice
	// must NOT bypass the admin scope check. Only internal-secret
	// callers, identified by a nil scopes context value, bypass.
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{})
	if _, err := srv.handleAdminListDLQ(ctx, &ListAdminDLQInput{}); err == nil {
		t.Fatal("expected 403 for empty-but-non-nil scopes on admin list")
	}
	if _, err := srv.handleAdminReplayDLQ(ctx, &AdminDLQRunInput{RunID: "r"}); err == nil {
		t.Fatal("expected 403 for empty-but-non-nil scopes on admin replay")
	}
	if _, err := srv.handleAdminUnmaskDLQ(ctx, &AdminDLQRunInput{RunID: "r"}); err == nil {
		t.Fatal("expected 403 for empty-but-non-nil scopes on admin unmask")
	}
	if _, err := srv.handleAdminPurgeDLQ(ctx, &AdminDLQRunInput{RunID: "r"}); err == nil {
		t.Fatal("expected 403 for empty-but-non-nil scopes on admin purge")
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
	if err == nil {
		t.Fatal("expected forbidden error when caller lacks dlq:read scope")
	}
}

// TestHandleAdminPurgeDLQ_AuditWriteFailure_LogsButSucceeds verifies that
// when the audit write fails after a successful mutation, the handler
// still returns 200 (the mutation committed and cannot be rolled back)
// and emits a structured error log so operators can reconcile.
func TestHandleAdminPurgeDLQ_AuditWriteFailure_LogsButSucceeds(t *testing.T) {
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
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (audit failure must not fail the mutation), got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(buf.String(), "audit write failed") {
		t.Fatalf("expected 'audit write failed' log entry, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "run_id=run-1") {
		t.Fatalf("expected run_id in log entry, got: %s", buf.String())
	}
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
			if projectID == "" {
				t.Fatalf("projectID must be propagated")
			}
			if masked == nil || *masked != true {
				t.Fatalf("expected masked=true filter, got %v", masked)
			}
			if jobID != nil {
				t.Fatalf("did not expect jobID filter, got %v", *jobID)
			}
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
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if filteredCalls != 1 {
		t.Fatalf("expected 1 filtered call, got %d", filteredCalls)
	}
	if unfilteredCalls != 0 {
		t.Fatalf("expected 0 unfiltered calls, got %d", unfilteredCalls)
	}
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
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid masked value, got %d: %s", w.Code, w.Body.String())
	}
}
