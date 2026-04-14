package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

type adminOutboxStoreMock struct {
	*APIStoreMock
	listFn func(ctx context.Context, projectID string, limit int, cursorConsumedAt *time.Time, cursorID string) ([]store.QuarantinedOutboxRow, error)
	getFn  func(ctx context.Context, projectID, id string) (*store.QuarantinedOutboxRow, error)
}

func (m *adminOutboxStoreMock) ListQuarantinedOutbox(ctx context.Context, projectID string, limit int, cursorConsumedAt *time.Time, cursorID string) ([]store.QuarantinedOutboxRow, error) {
	if m.listFn != nil {
		return m.listFn(ctx, projectID, limit, cursorConsumedAt, cursorID)
	}
	return nil, nil
}

func (m *adminOutboxStoreMock) GetQuarantinedOutbox(ctx context.Context, projectID, id string) (*store.QuarantinedOutboxRow, error) {
	if m.getFn != nil {
		return m.getFn(ctx, projectID, id)
	}
	return nil, store.ErrOutboxRowNotFound
}

func TestHandleAdminListOutbox_OK(t *testing.T) {
	t.Parallel()

	mock := &adminOutboxStoreMock{
		APIStoreMock: &APIStoreMock{},
		listFn: func(_ context.Context, projectID string, _ int, _ *time.Time, _ string) ([]store.QuarantinedOutboxRow, error) {
			return []store.QuarantinedOutboxRow{{
				ID:         "outbox-1",
				ProjectID:  projectID,
				JobID:      "job-1",
				Error:      "terminal failure",
				CreatedAt:  time.Now().Add(-time.Minute),
				ConsumedAt: time.Now(),
			}}, nil
		},
	}

	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/admin/outbox", "", "proj-outbox")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var rows []AdminOutboxRow
	decodePaginatedList(t, w.Body.Bytes(), &rows)
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].ID != "outbox-1" {
		t.Fatalf("row ID = %q, want %q", rows[0].ID, "outbox-1")
	}
}

func TestHandleAdminGetOutbox_NotFound(t *testing.T) {
	t.Parallel()

	mock := &adminOutboxStoreMock{
		APIStoreMock: &APIStoreMock{},
		getFn: func(context.Context, string, string) (*store.QuarantinedOutboxRow, error) {
			return nil, store.ErrOutboxRowNotFound
		},
	}

	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/admin/outbox/missing", "", "proj-outbox")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAdminOutbox_ForbiddenWithoutScope(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeRunsRead})
	if _, err := srv.handleAdminListOutbox(ctx, &ListAdminOutboxInput{}); err == nil {
		t.Fatal("expected forbidden when caller lacks outbox:read")
	}
	if _, err := srv.handleAdminGetOutbox(ctx, &GetAdminOutboxInput{OutboxID: "outbox-1"}); err == nil {
		t.Fatal("expected forbidden when caller lacks outbox:read")
	}
}
