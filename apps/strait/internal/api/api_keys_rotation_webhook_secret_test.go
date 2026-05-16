package api

import (
	"bytes"
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

func TestCreateAPIKey_RotationWebhookURL_RejectsDeliveryInvalidURLs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		url  string
	}{
		{name: "plaintext", url: "http://example.com/hook"},
		{name: "loopback", url: "https://127.0.0.1/hook"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var createCalled bool
			ms := &APIStoreMock{
				GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
					return &store.ProjectQuota{ProjectID: "proj-1", MaxKeyLifetimeDays: 365}, nil
				},
				CreateAPIKeyFunc: func(_ context.Context, _ *domain.APIKey) error {
					createCalled = true
					return nil
				},
			}
			srv := NewServer(ServerDeps{
				Config: &config.Config{
					InternalSecret:      "test-secret-value",
					MaxBulkTriggerItems: 500,
					JWTSigningKey:       testJWTSigningKey,
				},
				Store:     ms,
				Queue:     &mockQueue{},
				PubSub:    &mockPublisher{},
				Encryptor: roundTripEncryptor{},
				Edition:   domain.EditionCloud,
			})
			t.Cleanup(srv.Close)

			body := `{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":30,"rotation_webhook_url":"` + tc.url + `"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/api-keys", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Internal-Secret", "test-secret-value")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
			}
			if createCalled {
				t.Fatal("CreateAPIKey must not run for rotation webhook URL rejected by delivery-time validation")
			}
		})
	}
}

func TestRotateAPIKey_PreservesRotationWebhookSecret(t *testing.T) {
	t.Parallel()

	rotationInterval := 30
	expiresAt := time.Now().Add(24 * time.Hour)
	oldSecret := []byte{0x55, 0x01, 0x02, 0x03}
	var created domain.APIKey
	ms := &APIStoreMock{
		GetAPIKeyByIDFunc: func(_ context.Context, id string) (*domain.APIKey, error) {
			if id != "key-old" {
				t.Fatalf("GetAPIKeyByID id = %q, want key-old", id)
			}
			return &domain.APIKey{
				ID:                    "key-old",
				ProjectID:             "proj-1",
				Name:                  "production key",
				Scopes:                []string{domain.ScopeJobsRead},
				ExpiresAt:             &expiresAt,
				RotationIntervalDays:  &rotationInterval,
				RotationWebhookURL:    "https://example.com/rotate",
				RotationWebhookSecret: oldSecret,
			}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			created = *key
			key.ID = "key-new"
			return nil
		},
		MarkAPIKeyRotatedFunc: func(_ context.Context, oldID, newID string, _ time.Time) error {
			if oldID != "key-old" || newID != "key-new" {
				t.Fatalf("MarkAPIKeyRotated ids = %q/%q, want key-old/key-new", oldID, newID)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleRotateAPIKey(ctx, &RotateAPIKeyInput{KeyID: "key-old"})
	if err != nil {
		t.Fatalf("handleRotateAPIKey returned error: %v", err)
	}
	if !bytes.Equal(created.RotationWebhookSecret, oldSecret) {
		t.Fatalf("created rotation webhook secret = %x, want %x", created.RotationWebhookSecret, oldSecret)
	}
	if created.RotationWebhookURL != "https://example.com/rotate" {
		t.Fatalf("created RotationWebhookURL = %q, want existing URL", created.RotationWebhookURL)
	}
	if created.RotationIntervalDays == nil || *created.RotationIntervalDays != rotationInterval {
		t.Fatalf("created RotationIntervalDays = %v, want %d", created.RotationIntervalDays, rotationInterval)
	}
}
