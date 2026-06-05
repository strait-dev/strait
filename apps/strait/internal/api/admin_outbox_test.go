package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var rows []AdminOutboxRow
	decodePaginatedList(t, w.Body.Bytes(), &rows)
	require.Len(t,
		rows, 1)
	require.Equal(t, "outbox-1", rows[0].ID)
	require.False(t, rows[0].RetryOfOutboxID ==
		nil || *rows[0].RetryOfOutboxID !=
		lineage,
	)

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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)

}

func TestHandleAdminOutbox_ForbiddenWithoutScope(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeRunsRead})
	if _, err := srv.handleAdminListOutbox(ctx, &ListAdminOutboxInput{}); err == nil {
		require.Fail(t,

			"expected forbidden when caller lacks outbox:read")
	}
	if _, err := srv.handleAdminGetOutbox(ctx, &GetAdminOutboxInput{OutboxID: "outbox-1"}); err == nil {
		require.Fail(t,

			"expected forbidden when caller lacks outbox:read")
	}
	if _, err := srv.handleAdminRetryOutbox(ctx, &AdminOutboxMutationInput{OutboxID: "outbox-1"}); err == nil {
		require.Fail(t,

			"expected forbidden when caller lacks outbox:retry")
	}
	if _, err := srv.handleAdminPurgeOutbox(ctx, &AdminOutboxMutationInput{OutboxID: "outbox-1"}); err == nil {
		require.Fail(t,

			"expected forbidden when caller lacks outbox:purge")
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
		require.Fail(t,

			"expected environment-scoped outbox list to be forbidden")
	}
	if _, err := srv.handleAdminGetOutbox(readCtx, &GetAdminOutboxInput{OutboxID: "outbox-1"}); err == nil {
		require.Fail(t,

			"expected environment-scoped outbox get to be forbidden")
	}

	retryCtx := context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeOutboxRetry})
	if _, err := srv.handleAdminRetryOutbox(retryCtx, &AdminOutboxMutationInput{OutboxID: "outbox-1"}); err == nil {
		require.Fail(t,

			"expected environment-scoped outbox retry to be forbidden")
	}

	purgeCtx := context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeOutboxPurge})
	if _, err := srv.handleAdminPurgeOutbox(purgeCtx, &AdminOutboxMutationInput{OutboxID: "outbox-1"}); err == nil {
		require.Fail(t,

			"expected environment-scoped outbox purge to be forbidden")
	}
	require.False(t, storeCalled)

}

func TestHandleAdminRetryOutbox_Unauthorized(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &adminOutboxStoreMock{APIStoreMock: &APIStoreMock{}}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/admin/outbox/outbox-1/retry", nil)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)

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
	require.Equal(t, http.StatusOK,
		w.Code)

	var out AdminOutboxRow
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &out,
	))
	require.False(t, out.RetryOfOutboxID ==
		nil || *out.
		RetryOfOutboxID != lineage,
	)

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
	require.Equal(t, http.StatusOK,
		w.Code)

	var out struct {
		OutboxID      string `json:"outbox_id"`
		RetryOutboxID string `json:"retry_outbox_id"`
		OK            bool   `json:"ok"`
	}
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &out,
	))
	require.False(t, out.OutboxID !=
		"outbox-1" ||
		out.RetryOutboxID !=
			"retry-1" ||
		!out.
			OK)
	require.NotNil(t, gotAudit)
	require.Equal(t, "outbox.retry",
		gotAudit.
			Action)
	require.False(t, gotAudit.ResourceType !=
		"enqueue_outbox" ||
		gotAudit.ResourceID !=
			"outbox-1",
	)

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
	require.Equal(t, http.StatusConflict,
		w.
			Code)

}

func TestHandleAdminRetryOutbox_AuditFailureFailsRequest(t *testing.T) {
	t.Parallel()

	retryCalls := 0
	mock := &adminOutboxStoreMock{
		APIStoreMock: &APIStoreMock{
			CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
				require.Equal(t, "outbox.retry",
					ev.Action,
				)

				return errors.New("audit unavailable")
			},
		},
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
		retryFn: func(_ context.Context, projectID, id string) (*store.OutboxRow, error) {
			retryCalls++
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
	require.Equal(t, http.StatusInternalServerError,

		w.Code,
	)
	require.EqualValues(t, 1, retryCalls)

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
	require.Equal(t, http.StatusOK,
		w.Code)

	var out struct {
		OutboxID string `json:"outbox_id"`
		OK       bool   `json:"ok"`
	}
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &out,
	))
	require.False(t, out.OutboxID !=
		"outbox-1" ||
		!out.
			OK)
	require.NotNil(t, gotAudit)
	require.Equal(t, "outbox.purge",
		gotAudit.
			Action)

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
		require.False(t, strings.Contains(raw, value))

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
		require.True(
			t, strings.Contains(raw, required))

	}
}

func TestHandleAdminPurgeOutbox_AuditFailureFailsRequest(t *testing.T) {
	t.Parallel()

	purgeCalls := 0
	mock := &adminOutboxStoreMock{
		APIStoreMock: &APIStoreMock{
			CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
				require.Equal(t, "outbox.purge",
					ev.Action,
				)

				return errors.New("audit unavailable")
			},
		},
		purgeFn: func(_ context.Context, projectID, id string) (*store.QuarantinedOutboxRow, error) {
			purgeCalls++
			return &store.QuarantinedOutboxRow{
				ID:         id,
				ProjectID:  projectID,
				JobID:      "job-1",
				Error:      "terminal failure",
				CreatedAt:  time.Now().Add(-time.Minute),
				ConsumedAt: time.Now(),
			}, nil
		},
	}

	srv := newTestServer(t, mock, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/admin/outbox/outbox-1/purge", "", "proj-outbox")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusInternalServerError,

		w.Code,
	)
	require.EqualValues(t, 1, purgeCalls)

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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)

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
	require.Equal(t, http.StatusConflict,
		w.
			Code)

}
