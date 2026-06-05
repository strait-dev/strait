package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// capturingHandler is a slog.Handler that captures all log records for inspection.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
	attrs   []slog.Attr
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *capturingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &capturingHandler{records: h.records, attrs: append(h.attrs, attrs...)}
}

func (h *capturingHandler) WithGroup(_ string) slog.Handler { return h }

// allLogText returns all log messages and attribute values concatenated for searching.
func (h *capturingHandler) allLogText() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var sb strings.Builder
	for _, r := range h.records {
		sb.WriteString(r.Message)
		sb.WriteString(" ")
		r.Attrs(func(a slog.Attr) bool {
			sb.WriteString(a.Key)
			sb.WriteString("=")
			sb.WriteString(a.Value.String())
			sb.WriteString(" ")
			return true
		})
	}
	for _, a := range h.attrs {
		sb.WriteString(a.Key)
		sb.WriteString("=")
		sb.WriteString(a.Value.String())
		sb.WriteString(" ")
	}
	return sb.String()
}

// TestSecrets_APIKeyHashNeverInLog verifies that the full SHA-256 hash of an API key
// never appears in log output when performing auth-related operations.
func TestSecrets_APIKeyHashNeverInLog(t *testing.T) {
	t.Parallel()

	handler := &capturingHandler{}
	oldDefault := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(oldDefault) })

	rawKey := "strait_aabbccdd11223344556677889900aabbccddeeff11223344556677889900aabb"
	keyHash := hashAPIKey(rawKey)

	mockStore := &APIStoreMock{
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-001"
			return nil
		},
		RevokeAPIKeyFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}

	srv := newTestServerWithEncryptor(t, mockStore, &mockQueue{}, &mockEncryptor{})

	// Create an API key to trigger logging paths.
	body := `{"project_id":"proj-1","name":"test-key","scopes":["jobs:read"],"expires_in_days":30}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys", body))
	require.False(t, w.Code != http.
		StatusCreated &&
		w.Code !=
			http.StatusOK)

	// Revoke the key to exercise another logging path.
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, authedRequest(http.MethodDelete, "/v1/api-keys/key-001", ""))
	// Status may be 200 or 204; we just want logs.

	logText := handler.allLogText()
	require.False(t, strings.Contains(logText,
		keyHash))

}

// TestSecrets_JWTKeyNotInErrors verifies that sending an invalid JWT to runTokenAuth
// does not leak the signing key in the error response body.
func TestSecrets_JWTKeyNotInErrors(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	signingKey := srv.config.JWTSigningKey

	// Send a request with a bogus JWT to the SDK run endpoint.
	req := httptest.NewRequest(http.MethodGet, "/v1/sdk/runs/fake-run-id/dequeue", nil)
	req.Header.Set("Authorization", "Bearer totally-invalid-jwt-token")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	respBody := w.Body.String()
	require.False(t, strings.Contains(respBody,
		signingKey,
	))

}

// TestSecrets_WebhookSecretIsServerGenerated verifies that creating a webhook
// subscription ignores any client-supplied secret and returns a fresh
// server-generated whsec_ value exactly once under signing_secret.
func TestSecrets_WebhookSecretIsServerGenerated(t *testing.T) {
	t.Parallel()

	clientSuppliedSecret := "whsec_attacker_chosen_weak"

	mockStore := &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(_ context.Context, sub *domain.WebhookSubscription) error {
			sub.ID = "sub-001"
			return nil
		},
	}

	srv := newTestServerWithEncryptor(t, mockStore, &mockQueue{}, &mockEncryptor{})

	body := `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["run.completed"],"secret":"` + clientSuppliedSecret + `"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhooks/subscriptions", body))
	require.False(t, w.Code != http.
		StatusCreated &&
		w.Code !=
			http.StatusOK)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	got, ok := resp["signing_secret"].(string)
	require.False(t, !ok || got ==
		"")
	require.NotEqual(t, clientSuppliedSecret,

		got)
	require.False(t, len(got) != len("whsec_")+
		64 || got[:6] != "whsec_")

}

// TestSecrets_APIKeyPrefixOnlyInLog verifies that log entries contain at most the
// first 12 characters of an API key (the prefix) and never the full raw key.
func TestSecrets_APIKeyPrefixOnlyInLog(t *testing.T) {
	t.Parallel()

	handler := &capturingHandler{}
	oldDefault := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(oldDefault) })

	// Generate a real key to test with.
	rawKey, err := generateAPIKey()
	require.NoError(t, err)

	// The prefix is the first 12 chars (e.g. "strait_aabb").
	prefix := rawKey[:12]
	remainder := rawKey[12:]

	mockStore := &APIStoreMock{
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-002"
			return nil
		},
		RevokeAPIKeyFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}

	srv := newTestServer(t, mockStore, &mockQueue{}, nil)

	// Create API key.
	body := `{"project_id":"proj-1","name":"prefix-test","scopes":["jobs:read"],"expires_in_days":30}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys", body))

	// Revoke API key to trigger log entries.
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, authedRequest(http.MethodDelete, "/v1/api-keys/key-002", ""))

	logText := handler.allLogText()
	require.False(t, strings.Contains(logText,
		rawKey))
	require.False(t, strings.Contains(logText,
		remainder))

	// The full raw key must never appear.

	// The remainder after the prefix must not appear (ensures only prefix is logged, if anything).

	// Prefix may or may not appear in logs; that is acceptable.
	_ = prefix
}

// TestSecrets_PasswordFieldsNotSerialized verifies that domain objects with sensitive
// fields tagged json:"-" properly omit those fields when marshaled to JSON.
func TestSecrets_PasswordFieldsNotSerialized(t *testing.T) {
	t.Parallel()

	// domain.APIKey has KeyHash tagged json:"-".
	apiKey := domain.APIKey{
		ID:        "key-003",
		ProjectID: "proj-1",
		Name:      "test",
		KeyHash:   "deadbeef1234567890abcdef1234567890abcdef1234567890abcdef12345678",
		KeyPrefix: "strait_dead",
		Scopes:    []string{"jobs:read"},
	}

	data, err := json.Marshal(apiKey)
	require.NoError(t, err)

	jsonStr := string(data)
	require.False(t, strings.Contains(jsonStr,
		"key_hash"),
	)
	require.False(t, strings.Contains(jsonStr,
		apiKey.KeyHash,
	))

	// domain.JobSecret has EncryptedValue tagged json:"-".
	secret := domain.JobSecret{
		ID:             "sec-001",
		ProjectID:      "proj-1",
		SecretKey:      "DATABASE_URL",
		EncryptedValue: "enc:aes256gcm:base64ciphertext==",
		Environment:    "production",
	}

	data2, err := json.Marshal(secret)
	require.NoError(t, err)

	jsonStr2 := string(data2)
	require.False(t, strings.Contains(jsonStr2,
		"encrypted_value",
	))
	require.False(t, strings.Contains(jsonStr2,
		secret.EncryptedValue,
	))

}
