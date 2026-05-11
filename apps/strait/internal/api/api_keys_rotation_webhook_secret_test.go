package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"
)

type roundTripEncryptor struct{}

func (roundTripEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	out := make([]byte, len(plaintext)+1)
	out[0] = 0x55
	copy(out[1:], plaintext)
	return out, nil
}

func (roundTripEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 || ciphertext[0] != 0x55 {
		return nil, errBadCiphertext
	}
	out := make([]byte, len(ciphertext)-1)
	copy(out, ciphertext[1:])
	return out, nil
}

var errBadCiphertext = &decryptError{}

type decryptError struct{}

func (decryptError) Error() string { return "bad ciphertext" }

func newAPIKeyRotationWebhookTestServer(t *testing.T, enc Encryptor) (*Server, *domain.APIKey) {
	t.Helper()

	var captured domain.APIKey
	ms := &APIStoreMock{
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1", MaxKeyLifetimeDays: 365}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			captured = *key
			key.ID = "key-new"
			key.CreatedAt = time.Now()
			return nil
		},
	}

	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:    cfg,
		Store:     ms,
		Queue:     &mockQueue{},
		PubSub:    &mockPublisher{},
		Encryptor: enc,
		Edition:   domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv, &captured
}

func TestCreateAPIKey_RotationWebhookSecret_ReturnedOnce(t *testing.T) {
	t.Parallel()

	enc := roundTripEncryptor{}
	srv, captured := newAPIKeyRotationWebhookTestServer(t, enc)

	body := `{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":30,"rotation_interval_days":30,"rotation_webhook_url":"https://example.com/hook"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var resp CreateAPIKeyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(resp.RotationWebhookSecret, "whsec_") {
		t.Fatalf("rotation_webhook_secret missing whsec_ prefix: %q", resp.RotationWebhookSecret)
	}
	rawHex := strings.TrimPrefix(resp.RotationWebhookSecret, "whsec_")
	if len(rawHex) != 64 {
		t.Fatalf("rotation_webhook_secret hex length = %d, want 64; secret=%q", len(rawHex), resp.RotationWebhookSecret)
	}
	if _, err := hex.DecodeString(rawHex); err != nil {
		t.Fatalf("rotation_webhook_secret hex decode: %v", err)
	}

	if len(captured.RotationWebhookSecret) == 0 {
		t.Fatal("captured.RotationWebhookSecret is empty; should be encrypted ciphertext")
	}
	plaintext, err := enc.Decrypt(captured.RotationWebhookSecret)
	if err != nil {
		t.Fatalf("decrypt stored secret: %v", err)
	}
	if got := "whsec_" + hex.EncodeToString(plaintext); got != resp.RotationWebhookSecret {
		t.Fatalf("decrypted secret %q does not match response %q", got, resp.RotationWebhookSecret)
	}
	if captured.RotationWebhookURL != "https://example.com/hook" {
		t.Fatalf("captured RotationWebhookURL = %q, want https://example.com/hook", captured.RotationWebhookURL)
	}
}

func TestCreateAPIKey_NoRotationWebhookURL_NoSecret(t *testing.T) {
	t.Parallel()

	srv, captured := newAPIKeyRotationWebhookTestServer(t, roundTripEncryptor{})

	body := `{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":30}`
	req := httptest.NewRequest(http.MethodPost, "/v1/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var resp CreateAPIKeyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.RotationWebhookSecret != "" {
		t.Fatalf("rotation_webhook_secret should be empty when no URL provided; got %q", resp.RotationWebhookSecret)
	}
	if len(captured.RotationWebhookSecret) != 0 {
		t.Fatal("captured.RotationWebhookSecret should be empty when no URL provided")
	}
}

func TestCreateAPIKey_RotationWebhookURL_RequiresEncryptor(t *testing.T) {
	t.Parallel()

	srv, _ := newAPIKeyRotationWebhookTestServer(t, nil)

	body := `{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":30,"rotation_webhook_url":"https://example.com/hook"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when encryptor missing, got %d; body: %s", w.Code, w.Body.String())
	}
}
