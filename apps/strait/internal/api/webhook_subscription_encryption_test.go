package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/store"
)

// mockEncryptor is a deterministic encryptor for testing that prepends a known
// prefix so we can verify encryption happened without depending on crypto.
type mockEncryptor struct{}

func (m *mockEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	return append([]byte("encrypted:"), plaintext...), nil
}

func (m *mockEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	prefix := []byte("encrypted:")
	if len(ciphertext) > len(prefix) && string(ciphertext[:len(prefix)]) == string(prefix) {
		return ciphertext[len(prefix):], nil
	}
	return ciphertext, nil
}

func newTestServerWithEncryptor(t *testing.T, s APIStore, q *mockQueue, enc Encryptor) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      testInternalSecret,
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
		SecretEncryptionKey: "test-encryption-key-32-chars-ok",
	}
	var p pubsub.Publisher
	srv := NewServer(ServerDeps{
		Config:    cfg,
		Store:     s,
		Queue:     q,
		PubSub:    p,
		Encryptor: enc,
		Edition:   domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestHandleCreateWebhookSubscription_EncryptsSecret(t *testing.T) {
	t.Parallel()

	var storedSub *domain.WebhookSubscription
	ms := &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(_ context.Context, sub *domain.WebhookSubscription) error {
			storedSub = sub
			sub.ID = "sub-enc-1"
			sub.CreatedAt = time.Now().UTC()
			return nil
		},
	}

	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})

	body := `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["run.completed"],"secret":"my-plain-secret"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhooks/subscriptions", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	if storedSub == nil {
		t.Fatal("CreateWebhookSubscription was not called")
	}

	// The secret stored should be encrypted, not the plaintext input.
	if storedSub.Secret == "my-plain-secret" {
		t.Fatal("webhook subscription secret was stored in plaintext, expected encrypted value")
	}

	// Verify the encrypted value can be decrypted back to the original.
	enc := &mockEncryptor{}
	decrypted, err := enc.Decrypt([]byte(storedSub.Secret))
	if err != nil {
		t.Fatalf("failed to decrypt stored secret: %v", err)
	}
	if string(decrypted) != "my-plain-secret" {
		t.Fatalf("decrypted secret = %q, want %q", string(decrypted), "my-plain-secret")
	}
}

func TestHandleCreateWebhookSubscription_WithoutEncryptor_StoresRaw(t *testing.T) {
	t.Parallel()

	var storedSub *domain.WebhookSubscription
	ms := &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(_ context.Context, sub *domain.WebhookSubscription) error {
			storedSub = sub
			sub.ID = "sub-raw-1"
			sub.CreatedAt = time.Now().UTC()
			return nil
		},
	}

	// No encryptor provided.
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["run.completed"],"secret":"my-plain-secret"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhooks/subscriptions", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	if storedSub == nil {
		t.Fatal("CreateWebhookSubscription was not called")
	}

	// Without encryptor, secret is stored as-is (backward compatible).
	if storedSub.Secret != "my-plain-secret" {
		t.Fatalf("without encryptor, secret should be stored raw, got %q", storedSub.Secret)
	}
}

func TestHandleRotateWebhookSecret_EncryptsSecret(t *testing.T) {
	t.Parallel()

	var rotatedSecret string
	ms := &APIStoreMock{
		GetWebhookSubscriptionFunc: func(_ context.Context, id string) (*domain.WebhookSubscription, error) {
			if id == "sub-rotate-1" {
				return &domain.WebhookSubscription{
					ID:        "sub-rotate-1",
					ProjectID: "proj-1",
					Active:    true,
				}, nil
			}
			return nil, store.ErrWebhookSubscriptionNotFound
		},
		RotateWebhookSecretFunc: func(_ context.Context, _ string, newSecret string, _ time.Time) error {
			rotatedSecret = newSecret
			return nil
		},
	}

	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})

	body := `{"grace_period_minutes":60}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/webhooks/subscriptions/sub-rotate-1/rotate-secret", body, "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// The stored secret should be encrypted (starts with "encrypted:" prefix from mock).
	if rotatedSecret == "" {
		t.Fatal("RotateWebhookSecret was not called")
	}

	enc := &mockEncryptor{}
	decrypted, err := enc.Decrypt([]byte(rotatedSecret))
	if err != nil {
		t.Fatalf("failed to decrypt rotated secret: %v", err)
	}

	decryptedStr := string(decrypted)
	if len(decryptedStr) < 6 || decryptedStr[:6] != "whsec_" {
		t.Fatalf("decrypted rotated secret should start with whsec_, got %q", decryptedStr)
	}

	// The stored value should NOT start with whsec_ (it should be encrypted).
	if len(rotatedSecret) >= 6 && rotatedSecret[:6] == "whsec_" {
		t.Fatal("rotated secret was stored in plaintext, expected encrypted value")
	}
}
