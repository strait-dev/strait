package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

type adminOutboxStoreMock struct {
	*APIStoreMock
	listFn  func(ctx context.Context, projectID string, limit int, cursorConsumedAt *time.Time, cursorID string) ([]store.QuarantinedOutboxRow, error)
	getFn   func(ctx context.Context, projectID, id string) (*store.QuarantinedOutboxRow, error)
	retryFn func(ctx context.Context, projectID, id string) (*store.OutboxRow, error)
	purgeFn func(ctx context.Context, projectID, id string) (*store.QuarantinedOutboxRow, error)
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

func (m *adminOutboxStoreMock) RetryQuarantinedOutbox(ctx context.Context, projectID, id string) (*store.OutboxRow, error) {
	if m.retryFn != nil {
		return m.retryFn(ctx, projectID, id)
	}
	return nil, store.ErrOutboxRowConflict
}

func (m *adminOutboxStoreMock) PurgeQuarantinedOutbox(ctx context.Context, projectID, id string) (*store.QuarantinedOutboxRow, error) {
	if m.purgeFn != nil {
		return m.purgeFn(ctx, projectID, id)
	}
	return nil, store.ErrOutboxRowNotFound
}

func TestHandleAdminListOutbox_OK(t *testing.T) {
	t.Parallel()

	lineage := "source-outbox"
	mock := &adminOutboxStoreMock{
		APIStoreMock: &APIStoreMock{},
		listFn: func(_ context.Context, projectID string, _ int, _ *time.Time, _ string) ([]store.QuarantinedOutboxRow, error) {
			return []store.QuarantinedOutboxRow{{
				ID:              "outbox-1",
				ProjectID:       projectID,
				JobID:           "job-1",
				Error:           "terminal failure",
				CreatedAt:       time.Now().Add(-time.Minute),
				ConsumedAt:      time.Now(),
				RetryOfOutboxID: &lineage,
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
	if rows[0].RetryOfOutboxID == nil || *rows[0].RetryOfOutboxID != lineage {
		t.Fatalf("row RetryOfOutboxID = %v, want %q", rows[0].RetryOfOutboxID, lineage)
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
	if _, err := srv.handleAdminRetryOutbox(ctx, &AdminOutboxMutationInput{OutboxID: "outbox-1"}); err == nil {
		t.Fatal("expected forbidden when caller lacks outbox:retry")
	}
	if _, err := srv.handleAdminPurgeOutbox(ctx, &AdminOutboxMutationInput{OutboxID: "outbox-1"}); err == nil {
		t.Fatal("expected forbidden when caller lacks outbox:purge")
	}
}

func TestHandleAdminOutbox_EnvironmentScopedCallerForbidden(t *testing.T) {
	t.Parallel()

	storeCalled := false
	mock := &adminOutboxStoreMock{
		APIStoreMock: &APIStoreMock{},
		listFn: func(context.Context, string, int, *time.Time, string) ([]store.QuarantinedOutboxRow, error) {
			storeCalled = true
			return nil, nil
		},
		getFn: func(context.Context, string, string) (*store.QuarantinedOutboxRow, error) {
			storeCalled = true
			return nil, store.ErrOutboxRowNotFound
		},
		retryFn: func(context.Context, string, string) (*store.OutboxRow, error) {
			storeCalled = true
			return nil, store.ErrOutboxRowConflict
		},
		purgeFn: func(context.Context, string, string) (*store.QuarantinedOutboxRow, error) {
			storeCalled = true
			return nil, store.ErrOutboxRowNotFound
		},
	}

	srv := newTestServer(t, mock, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-outbox")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-staging")

	readCtx := context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeOutboxRead})
	if _, err := srv.handleAdminListOutbox(readCtx, &ListAdminOutboxInput{}); err == nil {
		t.Fatal("expected environment-scoped outbox list to be forbidden")
	}
	if _, err := srv.handleAdminGetOutbox(readCtx, &GetAdminOutboxInput{OutboxID: "outbox-1"}); err == nil {
		t.Fatal("expected environment-scoped outbox get to be forbidden")
	}

	retryCtx := context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeOutboxRetry})
	if _, err := srv.handleAdminRetryOutbox(retryCtx, &AdminOutboxMutationInput{OutboxID: "outbox-1"}); err == nil {
		t.Fatal("expected environment-scoped outbox retry to be forbidden")
	}

	purgeCtx := context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeOutboxPurge})
	if _, err := srv.handleAdminPurgeOutbox(purgeCtx, &AdminOutboxMutationInput{OutboxID: "outbox-1"}); err == nil {
		t.Fatal("expected environment-scoped outbox purge to be forbidden")
	}

	if storeCalled {
		t.Fatal("environment-scoped outbox admin request reached the outbox store")
	}
}

func TestHandleAdminRetryOutbox_Unauthorized(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &adminOutboxStoreMock{APIStoreMock: &APIStoreMock{}}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/admin/outbox/outbox-1/retry", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAdminGetOutbox_OK_IncludesRetryLineage(t *testing.T) {
	t.Parallel()

	lineage := "source-outbox"
	mock := &adminOutboxStoreMock{
		APIStoreMock: &APIStoreMock{},
		getFn: func(_ context.Context, projectID, id string) (*store.QuarantinedOutboxRow, error) {
			return &store.QuarantinedOutboxRow{
				ID:              id,
				ProjectID:       projectID,
				JobID:           "job-1",
				Error:           "terminal failure",
				CreatedAt:       time.Now().Add(-time.Minute),
				ConsumedAt:      time.Now(),
				RetryOfOutboxID: &lineage,
			}, nil
		},
	}

	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/admin/outbox/outbox-1", "", "proj-outbox")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var out AdminOutboxRow
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.RetryOfOutboxID == nil || *out.RetryOfOutboxID != lineage {
		t.Fatalf("RetryOfOutboxID = %v, want %q", out.RetryOfOutboxID, lineage)
	}
}

func TestHandleAdminRetryOutbox_OK_WritesAudit(t *testing.T) {
	t.Parallel()

	idempotencyKey := "idem-retry-secret-sk_live_raw_key"
	var gotAudit *domain.AuditEvent
	mock := &adminOutboxStoreMock{
		APIStoreMock: &APIStoreMock{
			CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
				gotAudit = ev
				return nil
			},
		},
		getFn: func(_ context.Context, projectID, id string) (*store.QuarantinedOutboxRow, error) {
			return &store.QuarantinedOutboxRow{
				ID:             id,
				ProjectID:      projectID,
				JobID:          "job-1",
				Payload:        json.RawMessage(`{"authorization":"Bearer retry-payload-token","body":"retry payload secret"}`),
				Metadata:       json.RawMessage(`{"api_key":"retry-metadata-secret"}`),
				IdempotencyKey: &idempotencyKey,
				Error:          "terminal failure with retry-secret-error",
				CreatedAt:      time.Now().Add(-time.Minute),
				ConsumedAt:     time.Now(),
			}, nil
		},
		retryFn: func(_ context.Context, projectID, id string) (*store.OutboxRow, error) {
			retryOf := id
			return &store.OutboxRow{
				ID:              "retry-1",
				ProjectID:       projectID,
				JobID:           "job-1",
				CreatedAt:       time.Now(),
				RetryOfOutboxID: &retryOf,
			}, nil
		},
	}

	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/admin/outbox/outbox-1/retry", "", "proj-outbox")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var out struct {
		OutboxID      string `json:"outbox_id"`
		RetryOutboxID string `json:"retry_outbox_id"`
		OK            bool   `json:"ok"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.OutboxID != "outbox-1" || out.RetryOutboxID != "retry-1" || !out.OK {
		t.Fatalf("unexpected response: %+v", out)
	}
	if gotAudit == nil {
		t.Fatal("expected audit event to be written")
	}
	if gotAudit.Action != "outbox.retry" {
		t.Fatalf("audit action = %s, want outbox.retry", gotAudit.Action)
	}
	if gotAudit.ResourceType != "enqueue_outbox" || gotAudit.ResourceID != "outbox-1" {
		t.Fatalf("unexpected audit resource: %s/%s", gotAudit.ResourceType, gotAudit.ResourceID)
	}
	assertOutboxAuditDetailsRedacted(t, gotAudit.Details, []string{
		"retry-payload-token",
		"retry payload secret",
		"retry-metadata-secret",
		idempotencyKey,
		"retry-secret-error",
	})
}

func TestHandleAdminRetryOutbox_ConflictWhenActiveCloneExists(t *testing.T) {
	t.Parallel()

	mock := &adminOutboxStoreMock{
		APIStoreMock: &APIStoreMock{},
		getFn: func(_ context.Context, projectID, id string) (*store.QuarantinedOutboxRow, error) {
			return &store.QuarantinedOutboxRow{
				ID:         id,
				ProjectID:  projectID,
				JobID:      "job-1",
				Error:      "terminal failure",
				CreatedAt:  time.Now().Add(-time.Minute),
				ConsumedAt: time.Now(),
			}, nil
		},
		retryFn: func(_ context.Context, _ string, _ string) (*store.OutboxRow, error) {
			return nil, store.ErrOutboxRowConflict
		},
	}

	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/admin/outbox/outbox-1/retry", "", "proj-outbox")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAdminPurgeOutbox_OK_WritesAudit(t *testing.T) {
	t.Parallel()

	idempotencyKey := "idem-purge-secret-sk_live_raw_key"
	var gotAudit *domain.AuditEvent
	mock := &adminOutboxStoreMock{
		APIStoreMock: &APIStoreMock{
			CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
				gotAudit = ev
				return nil
			},
		},
		purgeFn: func(_ context.Context, projectID, id string) (*store.QuarantinedOutboxRow, error) {
			return &store.QuarantinedOutboxRow{
				ID:             id,
				ProjectID:      projectID,
				JobID:          "job-1",
				Payload:        json.RawMessage(`{"authorization":"Bearer purge-payload-token","body":"purge payload secret"}`),
				Metadata:       json.RawMessage(`{"api_key":"purge-metadata-secret"}`),
				IdempotencyKey: &idempotencyKey,
				Error:          "terminal failure with purge-secret-error",
				CreatedAt:      time.Now().Add(-time.Minute),
				ConsumedAt:     time.Now(),
			}, nil
		},
	}

	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/admin/outbox/outbox-1/purge", "", "proj-outbox")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var out struct {
		OutboxID string `json:"outbox_id"`
		OK       bool   `json:"ok"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.OutboxID != "outbox-1" || !out.OK {
		t.Fatalf("unexpected response: %+v", out)
	}
	if gotAudit == nil {
		t.Fatal("expected audit event to be written")
	}
	if gotAudit.Action != "outbox.purge" {
		t.Fatalf("audit action = %s, want outbox.purge", gotAudit.Action)
	}
	assertOutboxAuditDetailsRedacted(t, gotAudit.Details, []string{
		"purge-payload-token",
		"purge payload secret",
		"purge-metadata-secret",
		idempotencyKey,
		"purge-secret-error",
	})
}

func assertOutboxAuditDetailsRedacted(t *testing.T, details json.RawMessage, forbidden []string) {
	t.Helper()

	raw := string(details)
	for _, value := range forbidden {
		if strings.Contains(raw, value) {
			t.Fatalf("audit details leaked %q: %s", value, raw)
		}
	}
	for _, required := range []string{
		"payload_sha256",
		"metadata_sha256",
		"payload_bytes",
		"metadata_bytes",
		"idempotency_key_present",
		"error_present",
		"error_bytes",
	} {
		if !strings.Contains(raw, required) {
			t.Fatalf("audit details missing %q: %s", required, raw)
		}
	}
}

func TestHandleAdminPurgeOutbox_NotFound(t *testing.T) {
	t.Parallel()

	mock := &adminOutboxStoreMock{
		APIStoreMock: &APIStoreMock{},
		purgeFn: func(_ context.Context, _ string, _ string) (*store.QuarantinedOutboxRow, error) {
			return nil, store.ErrOutboxRowNotFound
		},
	}

	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/admin/outbox/outbox-1/purge", "", "proj-outbox")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAdminPurgeOutbox_ConflictForNonQuarantinedRow(t *testing.T) {
	t.Parallel()

	mock := &adminOutboxStoreMock{
		APIStoreMock: &APIStoreMock{},
		purgeFn: func(_ context.Context, _ string, _ string) (*store.QuarantinedOutboxRow, error) {
			return nil, store.ErrOutboxRowConflict
		},
	}

	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/admin/outbox/outbox-1/purge", "", "proj-outbox")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}
