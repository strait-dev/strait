package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
