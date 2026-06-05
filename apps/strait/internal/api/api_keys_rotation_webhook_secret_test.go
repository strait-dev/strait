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

	"github.com/stretchr/testify/require"
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
	require.Equal(t, http.
		StatusCreated, w.
		Code)

	var resp CreateAPIKeyResponse
	require.NoError(t, json.
		NewDecoder(w.Body).Decode(&resp))
	require.True(t, strings.HasPrefix(resp.
		RotationWebhookSecret,
		"whsec_",
	))

	rawHex := strings.TrimPrefix(resp.RotationWebhookSecret, "whsec_")
	require.Len(t, rawHex,
		64)

	if _, err := hex.DecodeString(rawHex); err != nil {
		require.Failf(t, "test failure",

			"rotation_webhook_secret hex decode: %v", err)
	}
	require.NotEmpty(t, captured.
		RotationWebhookSecret,
	)

	plaintext, err := enc.Decrypt(captured.RotationWebhookSecret)
	require.NoError(t, err)
	require.Equal(t, resp.
		RotationWebhookSecret,
		string(plaintext))
	require.Equal(t, "https://example.com/hook",

		captured.RotationWebhookURL,
	)
}

func TestCreateAPIKey_RotationWebhookURL_RedactedInAudit(t *testing.T) {
	t.Parallel()

	auditCh := make(chan *domain.AuditEvent, 1)
	ms := &APIStoreMock{
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1", MaxKeyLifetimeDays: 365}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-new"
			key.CreatedAt = time.Now()
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			if ev.Action == domain.AuditActionAPIKeyCreated {
				auditCh <- ev
			}
			return nil
		},
	}

	cfg := &config.Config{
		InternalSecret:        "test-secret-value",
		MaxBulkTriggerItems:   500,
		JWTSigningKey:         testJWTSigningKey,
		AllowPrivateEndpoints: true,
	}
	srv := NewServer(ServerDeps{
		Config:    cfg,
		Store:     ms,
		Queue:     &mockQueue{},
		PubSub:    &mockPublisher{},
		Encryptor: roundTripEncryptor{},
		Edition:   domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	t.Cleanup(func() { globalAllowPrivateEndpoints.Store(false) })

	rawURL := "https://localhost/rotate/super-secret-path?token=opaque-shared-secret"
	body := `{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":30,"rotation_interval_days":30,"rotation_webhook_url":"` + rawURL + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusCreated, w.
		Code)

	var ev *domain.AuditEvent
	select {
	case ev = <-auditCh:
	case <-time.After(2 * time.Second):
		require.Fail(t, "timed out waiting for api key created audit event")
	}

	details := string(ev.Details)
	for _, forbidden := range []string{
		rawURL,
		"super-secret-path",
		"opaque-shared-secret",
		"rotation_webhook_url\"",
	} {
		require.NotContains(t, details, forbidden)
	}
	require.False(t, !strings.Contains(details,
		"rotation_webhook_url_host",
	) || !strings.Contains(details,
		"localhost",
	))
}

func TestCreateAPIKey_RotationIntervalRequiresWebhookURL(t *testing.T) {
	t.Parallel()

	srv, _ := newAPIKeyRotationWebhookTestServer(t, roundTripEncryptor{})

	body := `{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":30,"rotation_interval_days":30}`
	req := httptest.NewRequest(http.MethodPost, "/v1/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusBadRequest,
		w.Code)
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
	require.Equal(t, http.
		StatusCreated, w.
		Code)

	var resp CreateAPIKeyResponse
	require.NoError(t, json.
		NewDecoder(w.Body).Decode(&resp))
	require.Empty(t, resp.
		RotationWebhookSecret,
	)
	require.Empty(t, captured.
		RotationWebhookSecret)
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
	require.Equal(t, http.
		StatusInternalServerError,
		w.Code,
	)
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
			require.Equal(t, http.
				StatusBadRequest,
				w.Code)
			require.False(t, createCalled)
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
			require.Equal(t, "key-old",
				id)

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
			require.False(t, oldID !=
				"key-old" ||
				newID != "key-new",
			)

			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleRotateAPIKey(ctx, &RotateAPIKeyInput{KeyID: "key-old"})
	require.NoError(t, err)
	require.True(t, bytes.
		Equal(created.RotationWebhookSecret,

			oldSecret,
		))
	require.Equal(t, "https://example.com/rotate",

		created.
			RotationWebhookURL,
	)
	require.False(t, created.
		RotationIntervalDays ==
		nil ||
		*created.RotationIntervalDays !=
			rotationInterval,
	)
}
