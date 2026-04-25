package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"

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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	sig := w.Header().Get("X-Audit-Signature")
	if sig == "" {
		t.Fatal("expected X-Audit-Signature header to be present")
	}
	if !strings.HasPrefix(sig, "sha256=") {
		t.Fatalf("signature %q should start with sha256=", sig)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sig := w.Header().Get("X-Audit-Signature")
	if sig != "" {
		t.Fatalf("expected no X-Audit-Signature when InternalSecret is empty, got %q", sig)
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	sig := w.Header().Get("X-Audit-Signature")
	if !strings.HasPrefix(sig, "sha256=") {
		t.Fatalf("expected sha256= prefix, got %q", sig)
	}
	hexSig := strings.TrimPrefix(sig, "sha256=")

	// Recompute HMAC using the same key derivation: HKDF-SHA256 over InternalSecret.
	internalSecret := "test-secret-value"
	hkdfReader := xhkdf.New(sha256.New, []byte(internalSecret), []byte("audit-export-signing"), []byte("strait:v1:audit-export-hmac"))
	signingKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, signingKey); err != nil {
		t.Fatalf("failed to derive signing key: %v", err)
	}

	mac := hmac.New(sha256.New, signingKey)
	mac.Write(w.Body.Bytes())
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if hexSig != expectedSig {
		t.Fatalf("HMAC mismatch:\n  got:  %s\n  want: %s", hexSig, expectedSig)
	}
}
