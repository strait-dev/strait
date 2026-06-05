package api

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
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
		InternalSecret:      "test-secret-value",
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
		Edition:   domain.EditionCommunity,
	})
	t.Cleanup(srv.Close)
	return srv
}

func requireBase64EncryptedSecretPlaintext(t *testing.T, enc Encryptor, encrypted, want string) {
	t.Helper()
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	require.NoError(t, err)

	plaintext, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, want, string(plaintext))
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

	body := `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["run.completed"]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhooks/subscriptions", body))
	require.Equal(t, http.StatusCreated,
		w.Code)
	require.NotNil(t, storedSub)
	require.False(t, len(storedSub.Secret) >= 6 &&
		storedSub.Secret[:6] == "whsec_",
	)

	// Stored secret must be text-safe encrypted ciphertext rather than the
	// plaintext whsec_ value the server returns once.

	// Decrypting the stored value yields the server-generated whsec_-prefixed
	// signing secret.
	ciphertext, err := base64.StdEncoding.DecodeString(storedSub.Secret)
	require.NoError(t, err)

	enc := &mockEncryptor{}
	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	require.False(t, len(decrypted) <
		6 || string(decrypted[:6]) != "whsec_",
	)
}

func TestHandleCreateWebhookSubscription_WithoutEncryptorFailsClosed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(_ context.Context, sub *domain.WebhookSubscription) error {
			require.Fail(t,

				"CreateWebhookSubscription should not be called without webhook secret encryption")
			return nil
		},
	}

	// No encryptor provided.
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["run.completed"]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhooks/subscriptions", body))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.NotEmpty(t, rotatedSecret)

	// The stored secret should be encrypted (starts with "encrypted:" prefix from mock).

	enc := &mockEncryptor{}
	ciphertext, err := base64.StdEncoding.DecodeString(rotatedSecret)
	require.NoError(t, err)

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)

	decryptedStr := string(decrypted)
	require.False(t, len(decryptedStr) < 6 || decryptedStr[:6] != "whsec_")
	require.False(t, len(rotatedSecret) >= 6 &&
		rotatedSecret[:6] ==
			"whsec_")

	// The stored value should NOT start with whsec_ (it should be encrypted).
}
