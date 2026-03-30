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

func newTestServerWithEncryptionKey(t *testing.T, s APIStore, encryptionKey string) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       "01234567890123456789012345678901",
		SecretEncryptionKey: encryptionKey,
	}
	srv := NewServer(ServerDeps{
		Config: cfg,
		Store:  s,
	})
	t.Cleanup(srv.Close)
	return srv
}

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

	srv := newTestServerWithEncryptionKey(t, ms, "my-secret-encryption-key-32chars!")
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
	}

	srv := newTestServerWithEncryptionKey(t, ms, "")
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/audit-events/export?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z&format=ndjson", "", "proj-1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	sig := w.Header().Get("X-Audit-Signature")
	if sig != "" {
		t.Fatalf("expected no X-Audit-Signature when encryption key is empty, got %q", sig)
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

	encKey := "my-secret-encryption-key-32chars!"
	srv := newTestServerWithEncryptionKey(t, ms, encKey)
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

	// Recompute HMAC with same key derivation.
	hkdfReader := xhkdf.New(sha256.New, []byte(encKey), []byte("audit-export-signing"), nil)
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
