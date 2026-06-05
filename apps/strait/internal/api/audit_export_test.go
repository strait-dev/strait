package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
	xhkdf "golang.org/x/crypto/hkdf"
)

func TestAuditExport_JSON_IncludesSignature(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			ev := &domain.AuditEvent{
				ID: "ev-1", ProjectID: "proj-1", ActorID: "user-1", ActorType: "user",
				Action: "job.created", ResourceType: "job", ResourceID: "job-1",
				Details: json.RawMessage(`{"name":"test"}`), CreatedAt: now,
			}
			return fn(ev)
		},
	}

	// The HMAC signature is derived from InternalSecret (not SecretEncryptionKey).
	// newTestServer sets InternalSecret="test-secret-value" which is sufficient.
	srv := newTestServer(t, ms, nil, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/audit-events/export?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z&format=ndjson", "", "proj-1")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

	sig := w.Header().Get("X-Audit-Signature")
	require.NotEqual(t, "", sig)
	require.True(
		t, strings.HasPrefix(sig, "sha256="))

}

func TestAuditExport_NoSigningKey_SkipsSignature(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			return fn(&domain.AuditEvent{ID: "ev-1", ProjectID: "proj-1", CreatedAt: time.Now()})
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}

	// Build a server with an empty InternalSecret so signing is disabled.
	// The export HMAC is derived from InternalSecret (not SecretEncryptionKey),
	// so only an empty InternalSecret should suppress the X-Audit-Signature header.
	// We call the handler directly (bypassing HTTP routing) so that auth middleware
	// does not reject the empty-InternalSecret server.
	cfg := &config.Config{
		InternalSecret:      "",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
		SecretEncryptionKey: "some-encryption-key",
	}
	srv := NewServer(ServerDeps{Config: cfg, Store: ms})
	t.Cleanup(srv.Close)

	w := httptest.NewRecorder()
	rawReq := httptest.NewRequest(http.MethodGet,
		"/v1/audit-events/export?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z&format=ndjson", nil)
	ctx := context.WithValue(rawReq.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxKeyResponseWriter, w)
	ctx = context.WithValue(ctx, ctxKeyRequest, rawReq.WithContext(ctx))

	input := &ExportAuditEventsInput{
		From:   "2026-01-01T00:00:00Z",
		To:     "2026-02-01T00:00:00Z",
		Format: "ndjson",
	}
	_, err := srv.handleExportAuditEvents(ctx, input)
	require.NoError(t, err)

	sig := w.Header().Get("X-Audit-Signature")
	require.Equal(t, "", sig)

}

func TestAuditExport_EnvironmentScopedKeyRejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(context.Context, string, string, string, time.Time, time.Time, func(*domain.AuditEvent) error) error {
			require.Fail(t,

				"environment-scoped audit export must be rejected before streaming")
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	rawReq := httptest.NewRequest(http.MethodGet,
		"/v1/audit-events/export?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z&format=ndjson", nil)
	ctx := context.WithValue(rawReq.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-staging")
	ctx = context.WithValue(ctx, ctxKeyResponseWriter, w)
	ctx = context.WithValue(ctx, ctxKeyRequest, rawReq.WithContext(ctx))

	_, err := srv.handleExportAuditEvents(ctx, &ExportAuditEventsInput{
		From:   "2026-01-01T00:00:00Z",
		To:     "2026-02-01T00:00:00Z",
		Format: "ndjson",
	})
	require.Error(t, err)
	require.True(
		t, strings.Contains(err.Error(),
			"project-wide key",
		))

}

func TestAuditExport_CreatesDurableAuditEventBeforeStreaming(t *testing.T) {
	t.Parallel()

	var auditCreated atomic.Bool
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			require.Equal(t, domain.AuditActionAuditExported,

				ev.Action)
			require.Equal(t, "proj-1", ev.
				ResourceID)

			auditCreated.Store(true)
			return nil
		},
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			require.True(
				t, auditCreated.
					Load())

			return fn(&domain.AuditEvent{ID: "ev-1", ProjectID: "proj-1", CreatedAt: time.Now()})
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/audit-events/export?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z&format=ndjson", "", "proj-1")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, auditCreated.
			Load())

}

func TestAuditExport_AuditWriteFailurePreventsStreaming(t *testing.T) {
	t.Parallel()

	var streamCalled atomic.Bool
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return errors.New("audit store unavailable")
		},
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, _ func(*domain.AuditEvent) error) error {
			streamCalled.Store(true)
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/audit-events/export?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z&format=ndjson", "", "proj-1")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
	require.False(t, streamCalled.
		Load())

}

func TestAuditExportCSV_EscapesFormulaCells(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			return fn(&domain.AuditEvent{
				ID:           "=ev-1",
				ProjectID:    "+proj-1",
				ActorID:      "-user-1",
				ActorType:    "@user",
				Action:       "job.created",
				ResourceType: "job",
				ResourceID:   "\tjob-1",
				Details:      json.RawMessage(`=HYPERLINK("https://attacker.test","x")`),
				CreatedAt:    now,
				RemoteIP:     "127.0.0.1",
				UserAgent:    "\rmalicious",
				RequestID:    "\nrequest",
				TraceID:      "trace-1",
			})
		},
	}
	srv := newTestServer(t, ms, nil, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/audit-events/export?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z&format=csv", "", "proj-1")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

	records, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	require.NoError(t, err)
	require.Len(t,
		records, 2)

	row := records[1]
	formulaColumns := map[int]string{
		0:  "id",
		1:  "project_id",
		2:  "actor_id",
		3:  "actor_type",
		6:  "resource_id",
		7:  "details",
		10: "user_agent",
		11: "request_id",
	}
	for idx := range formulaColumns {
		require.True(
			t, strings.HasPrefix(row[idx], "'"))

	}
}

func TestSanitizeCSVCell_EscapesFormulaAfterLeadingWhitespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "plain formula",
			value: "=HYPERLINK(\"https://attacker.test\",\"x\")",
			want:  "'=HYPERLINK(\"https://attacker.test\",\"x\")",
		},
		{
			name:  "leading space",
			value: " =HYPERLINK(\"https://attacker.test\",\"x\")",
			want:  "' =HYPERLINK(\"https://attacker.test\",\"x\")",
		},
		{
			name:  "leading unicode bom",
			value: "\ufeff=HYPERLINK(\"https://attacker.test\",\"x\")",
			want:  "'\ufeff=HYPERLINK(\"https://attacker.test\",\"x\")",
		},
		{
			name:  "leading tab and newline",
			value: "\t\n@SUM(1,1)",
			want:  "'\t\n@SUM(1,1)",
		},
		{
			name:  "safe leading whitespace text",
			value: " safe",
			want:  " safe",
		},
		{
			name:  "only whitespace",
			value: " \t\n",
			want:  " \t\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, sanitizeCSVCell(tt.
				value))

		})
	}
}

func TestAuditExport_SignatureVerifies(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			return fn(&domain.AuditEvent{
				ID: "ev-1", ProjectID: "proj-1", ActorID: "user-1", ActorType: "user",
				Action: "job.created", ResourceType: "job", ResourceID: "job-1",
				Details: json.RawMessage(`{"name":"test"}`), CreatedAt: now,
			})
		},
	}

	// The export HMAC is derived from InternalSecret, not SecretEncryptionKey.
	// newTestServer sets InternalSecret to "test-secret-value".
	srv := newTestServer(t, ms, nil, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/audit-events/export?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z&format=ndjson", "", "proj-1")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

	sig := w.Header().Get("X-Audit-Signature")
	require.True(
		t, strings.HasPrefix(sig, "sha256="))

	hexSig := strings.TrimPrefix(sig, "sha256=")

	// Recompute HMAC using the same key derivation: HKDF-SHA256 over InternalSecret.
	internalSecret := "test-secret-value"
	hkdfReader := xhkdf.New(sha256.New, []byte(internalSecret), []byte("audit-export-signing"), []byte("strait:v1:audit-export-hmac"))
	signingKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, signingKey); err != nil {
		require.Failf(t, "test failure",

			"failed to derive signing key: %v", err)
	}

	mac := hmac.New(sha256.New, signingKey)
	mac.Write(w.Body.Bytes())
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	require.Equal(t, expectedSig,
		hexSig)

}
