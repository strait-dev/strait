package scheduler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

type mockAutoRotateStore struct {
	mockReaperStore
	listDueRotationFn  func(ctx context.Context) ([]domain.APIKey, error)
	createAPIKeyFn     func(ctx context.Context, key *domain.APIKey) error
	createRotatedFn    func(ctx context.Context, oldKeyID string, newKey *domain.APIKey, graceExpiresAt time.Time) error
	getProjectQuotaFn  func(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	markRotatedFn      func(ctx context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error
	revokeAPIKeyFn     func(ctx context.Context, id string) error
	disableRotationFn  func(ctx context.Context, id string) error
	createAuditEventFn func(ctx context.Context, ev *domain.AuditEvent) error
}

func (m *mockAutoRotateStore) ListAPIKeysDueRotation(ctx context.Context) ([]domain.APIKey, error) {
	if m.listDueRotationFn != nil {
		return m.listDueRotationFn(ctx)
	}
	return nil, nil
}

func (m *mockAutoRotateStore) CreateAPIKey(ctx context.Context, key *domain.APIKey) error {
	if m.createAPIKeyFn != nil {
		return m.createAPIKeyFn(ctx, key)
	}
	return nil
}

func (m *mockAutoRotateStore) CreateRotatedAPIKey(ctx context.Context, oldKeyID string, newKey *domain.APIKey, graceExpiresAt time.Time) error {
	if m.createRotatedFn != nil {
		return m.createRotatedFn(ctx, oldKeyID, newKey, graceExpiresAt)
	}
	if err := m.CreateAPIKey(ctx, newKey); err != nil {
		return err
	}
	return m.MarkAPIKeyRotated(ctx, oldKeyID, newKey.ID, graceExpiresAt)
}

func (m *mockAutoRotateStore) GetProjectQuota(ctx context.Context, projectID string) (*store.ProjectQuota, error) {
	if m.getProjectQuotaFn != nil {
		return m.getProjectQuotaFn(ctx, projectID)
	}
	return &store.ProjectQuota{ProjectID: projectID, MaxKeyLifetimeDays: 90}, nil
}

func (m *mockAutoRotateStore) MarkAPIKeyRotated(ctx context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error {
	if m.markRotatedFn != nil {
		return m.markRotatedFn(ctx, oldKeyID, newKeyID, graceExpiresAt)
	}
	return nil
}

func (m *mockAutoRotateStore) RevokeAPIKey(ctx context.Context, id string) error {
	if m.revokeAPIKeyFn != nil {
		return m.revokeAPIKeyFn(ctx, id)
	}
	return nil
}

func (m *mockAutoRotateStore) DisableAPIKeyAutoRotation(ctx context.Context, id string) error {
	if m.disableRotationFn != nil {
		return m.disableRotationFn(ctx, id)
	}
	return nil
}

func (m *mockAutoRotateStore) CreateAuditEvent(ctx context.Context, ev *domain.AuditEvent) error {
	if m.createAuditEventFn != nil {
		return m.createAuditEventFn(ctx, ev)
	}
	return nil
}

func TestAutoRotateAPIKeys_RotatesExpiredKey(t *testing.T) {
	t.Parallel()

	var createdKey *domain.APIKey
	var markedOldID string
	var markedNewID string
	var auditAction string
	var webhookPayload map[string]any
	webhookServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&webhookPayload); err != nil {
			t.Fatalf("decode webhook payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	rotationDays30 := 30
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return []domain.APIKey{
				{
					ID:                    "old-key-1",
					ProjectID:             "proj-1",
					OrgID:                 "org-1",
					Name:                  "My Key",
					Scopes:                []string{"jobs:read", "jobs:write"},
					EnvironmentID:         "env-prod",
					RotationIntervalDays:  &rotationDays30,
					RotationWebhookURL:    webhookServer.URL,
					RotationWebhookSecret: rotationWebhookSecretForTest(),
				},
			}, nil
		},
		createAPIKeyFn: func(_ context.Context, key *domain.APIKey) error {
			createdKey = key
			key.ID = "new-key-1" // simulate DB generating ID
			return nil
		},
		markRotatedFn: func(_ context.Context, oldID, newID string, _ time.Time) error {
			markedOldID = oldID
			markedNewID = newID
			return nil
		},
		createAuditEventFn: func(_ context.Context, ev *domain.AuditEvent) error {
			auditAction = ev.Action
			return nil
		},
	}

	r := newSignedAutoRotateReaper(ms)
	r.rotationWebhookClient = webhookServer.Client()
	r.autoRotateAPIKeys(context.Background())

	if createdKey == nil {
		t.Fatal("expected new key to be created")
	}
	if createdKey.ProjectID != "proj-1" {
		t.Fatalf("new key project = %q, want proj-1", createdKey.ProjectID)
	}
	if createdKey.OrgID != "org-1" {
		t.Fatalf("new key org = %q, want org-1", createdKey.OrgID)
	}
	if createdKey.EnvironmentID != "env-prod" {
		t.Fatalf("new key env = %q, want env-prod", createdKey.EnvironmentID)
	}
	if len(createdKey.Scopes) != 2 || createdKey.Scopes[0] != "jobs:read" {
		t.Fatalf("new key scopes = %v, want [jobs:read jobs:write]", createdKey.Scopes)
	}
	if createdKey.NextRotationAt == nil {
		t.Fatal("new key should have next_rotation_at set")
	}
	if createdKey.ExpiresAt == nil {
		t.Fatal("new key should have policy-capped expires_at")
	}
	if createdKey.ExpiresAt.After(time.Now().Add(91 * 24 * time.Hour)) {
		t.Fatalf("new key expires_at = %v, want within default project policy", createdKey.ExpiresAt)
	}
	if createdKey.KeyPrefix == "" || len(createdKey.KeyPrefix) != 12 {
		t.Fatalf("new key prefix = %q, want 12-char prefix", createdKey.KeyPrefix)
	}
	if createdKey.KeyHash == "" {
		t.Fatal("new key hash should be set")
	}
	if markedOldID != "old-key-1" {
		t.Fatalf("marked old key = %q, want old-key-1", markedOldID)
	}
	if markedNewID != "new-key-1" {
		t.Fatalf("marked new key = %q, want new-key-1", markedNewID)
	}
	if auditAction != "api_key.auto_rotated" {
		t.Fatalf("audit action = %q, want api_key.auto_rotated", auditAction)
	}
	if webhookPayload["new_key"] == "" {
		t.Fatalf("rotation webhook payload did not include new_key: %+v", webhookPayload)
	}
	if webhookPayload["new_key_prefix"] != createdKey.KeyPrefix {
		t.Fatalf("webhook prefix = %v, want %s", webhookPayload["new_key_prefix"], createdKey.KeyPrefix)
	}
}

func TestAutoRotateAPIKeys_SkipsKeyWithoutWebhook(t *testing.T) {
	t.Parallel()

	var created atomic.Int32
	var disabledID string
	rotationDays := 30
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return []domain.APIKey{{
				ID:                   "old-key-1",
				ProjectID:            "proj-1",
				Name:                 "My Key",
				RotationIntervalDays: &rotationDays,
			}}, nil
		},
		createAPIKeyFn: func(context.Context, *domain.APIKey) error {
			created.Add(1)
			return nil
		},
		disableRotationFn: func(_ context.Context, id string) error {
			disabledID = id
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil).WithAllowPrivateEndpoints(true)
	r.rotationWebhookClient = successfulRotationWebhookClient()
	r.autoRotateAPIKeys(context.Background())

	if created.Load() != 0 {
		t.Fatalf("created keys = %d, want 0 without rotation webhook", created.Load())
	}
	if disabledID != "old-key-1" {
		t.Fatalf("disabled key id = %q, want old-key-1", disabledID)
	}
}

func TestAutoRotateAPIKeys_SkipsKeyWithoutSigningSecret(t *testing.T) {
	t.Parallel()

	var created atomic.Int32
	rotationDays := 30
	webhookURL := rotationWebhookURLForTest(t)
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return []domain.APIKey{{
				ID:                   "old-key-1",
				ProjectID:            "proj-1",
				Name:                 "legacy-unsigned",
				RotationIntervalDays: &rotationDays,
				RotationWebhookURL:   webhookURL,
			}}, nil
		},
		createAPIKeyFn: func(context.Context, *domain.APIKey) error {
			created.Add(1)
			return nil
		},
	}

	r := newSignedAutoRotateReaper(ms)
	r.rotationWebhookClient = successfulRotationWebhookClient()
	r.autoRotateAPIKeys(context.Background())

	if created.Load() != 0 {
		t.Fatalf("created keys = %d, want 0 without rotation webhook signing secret", created.Load())
	}
}

func TestAutoRotateAPIKeys_SkipsNoExpiryWhenNoLifetimePolicy(t *testing.T) {
	t.Parallel()

	var created atomic.Int32
	rotationDays := 30
	webhookURL := rotationWebhookURLForTest(t)
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return []domain.APIKey{{
				ID:                    "old-key-1",
				ProjectID:             "proj-1",
				Name:                  "legacy-no-expiry",
				RotationIntervalDays:  &rotationDays,
				RotationWebhookURL:    webhookURL,
				RotationWebhookSecret: rotationWebhookSecretForTest(),
			}}, nil
		},
		getProjectQuotaFn: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID, MaxKeyLifetimeDays: 0}, nil
		},
		createAPIKeyFn: func(context.Context, *domain.APIKey) error {
			created.Add(1)
			return nil
		},
	}

	r := newSignedAutoRotateReaper(ms)
	r.rotationWebhookClient = successfulRotationWebhookClient()
	r.autoRotateAPIKeys(context.Background())

	if created.Load() != 0 {
		t.Fatalf("created keys = %d, want 0 when no-expiry key violates lifetime policy", created.Load())
	}
}

func TestAutoRotateAPIKeys_SkipsOverlongExpiry(t *testing.T) {
	t.Parallel()

	var created atomic.Int32
	rotationDays := 30
	webhookURL := rotationWebhookURLForTest(t)
	overlong := time.Now().Add(365 * 24 * time.Hour)
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return []domain.APIKey{{
				ID:                    "old-key-1",
				ProjectID:             "proj-1",
				Name:                  "overlong-expiry",
				ExpiresAt:             &overlong,
				RotationIntervalDays:  &rotationDays,
				RotationWebhookURL:    webhookURL,
				RotationWebhookSecret: rotationWebhookSecretForTest(),
			}}, nil
		},
		getProjectQuotaFn: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID, MaxKeyLifetimeDays: 30}, nil
		},
		createAPIKeyFn: func(context.Context, *domain.APIKey) error {
			created.Add(1)
			return nil
		},
	}

	r := newSignedAutoRotateReaper(ms)
	r.rotationWebhookClient = successfulRotationWebhookClient()
	r.autoRotateAPIKeys(context.Background())

	if created.Load() != 0 {
		t.Fatalf("created keys = %d, want 0 when old key expiry exceeds project policy", created.Load())
	}
}

func TestReaperMaintenanceCycleRunsAutoRotate(t *testing.T) {
	t.Parallel()

	var listed atomic.Int32
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			listed.Add(1)
			return nil, nil
		},
	}
	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)

	r.runMaintenanceCycle(context.Background())

	if listed.Load() != 1 {
		t.Fatalf("auto-rotation listed due keys %d times, want 1", listed.Load())
	}
}

func TestAutoRotateAPIKeys_NoDueKeys(t *testing.T) {
	t.Parallel()
	var created atomic.Int32
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return nil, nil
		},
		createAPIKeyFn: func(context.Context, *domain.APIKey) error {
			created.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.autoRotateAPIKeys(context.Background())

	if created.Load() != 0 {
		t.Fatalf("expected no keys created, got %d", created.Load())
	}
}

func TestAutoRotateAPIKeys_CreateFails_SkipsKey(t *testing.T) {
	t.Parallel()
	rotationDays7 := 7
	webhookURL := rotationWebhookURLForTest(t)
	var markCalled atomic.Int32
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return []domain.APIKey{
				{ID: "key-1", ProjectID: "proj-1", RotationIntervalDays: &rotationDays7, RotationWebhookURL: webhookURL, RotationWebhookSecret: rotationWebhookSecretForTest()},
			}, nil
		},
		createAPIKeyFn: func(context.Context, *domain.APIKey) error {
			return context.DeadlineExceeded // simulate DB error
		},
		markRotatedFn: func(context.Context, string, string, time.Time) error {
			markCalled.Add(1)
			return nil
		},
	}

	r := newSignedAutoRotateReaper(ms)
	r.autoRotateAPIKeys(context.Background())

	if markCalled.Load() != 0 {
		t.Fatal("should not mark rotated when create fails")
	}
}

func TestAutoRotateAPIKeys_MarkRotatedFailsRevokesStandaloneKey(t *testing.T) {
	t.Parallel()

	rotationDays := 30
	webhookURL := rotationWebhookURLForTest(t)
	var created atomic.Int32
	var revokeCalled atomic.Int32
	var auditCalled atomic.Int32
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return []domain.APIKey{{
				ID:                    "old-key-1",
				ProjectID:             "proj-1",
				Name:                  "My Key",
				RotationIntervalDays:  &rotationDays,
				RotationWebhookURL:    webhookURL,
				RotationWebhookSecret: rotationWebhookSecretForTest(),
			}}, nil
		},
		createAPIKeyFn: func(context.Context, *domain.APIKey) error {
			created.Add(1)
			return nil
		},
		markRotatedFn: func(context.Context, string, string, time.Time) error {
			return context.Canceled
		},
		revokeAPIKeyFn: func(_ context.Context, id string) error {
			revokeCalled.Add(1)
			return nil
		},
		createAuditEventFn: func(context.Context, *domain.AuditEvent) error {
			auditCalled.Add(1)
			return nil
		},
	}

	r := newSignedAutoRotateReaper(ms)
	r.rotationWebhookClient = successfulRotationWebhookClient()
	r.autoRotateAPIKeys(context.Background())

	if created.Load() != 1 {
		t.Fatalf("CreateAPIKey calls = %d, want 1", created.Load())
	}
	if revokeCalled.Load() != 1 {
		t.Fatalf("RevokeAPIKey calls = %d, want 1 for the standalone new key", revokeCalled.Load())
	}
	if auditCalled.Load() != 0 {
		t.Fatal("audit event emitted despite failed rotation link")
	}
}

func TestAutoRotateAPIKeys_MultipleKeys(t *testing.T) {
	t.Parallel()
	days7, days14, days30 := 7, 14, 30
	webhookURL := rotationWebhookURLForTest(t)
	var created atomic.Int32
	var marked atomic.Int32
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return []domain.APIKey{
				{ID: "key-1", ProjectID: "proj-1", RotationIntervalDays: &days7, RotationWebhookURL: webhookURL, RotationWebhookSecret: rotationWebhookSecretForTest()},
				{ID: "key-2", ProjectID: "proj-2", RotationIntervalDays: &days14, RotationWebhookURL: webhookURL, RotationWebhookSecret: rotationWebhookSecretForTest()},
				{ID: "key-3", ProjectID: "proj-1", RotationIntervalDays: &days30, RotationWebhookURL: webhookURL, RotationWebhookSecret: rotationWebhookSecretForTest()},
			}, nil
		},
		createAPIKeyFn: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "new-" + key.ProjectID
			created.Add(1)
			return nil
		},
		markRotatedFn: func(context.Context, string, string, time.Time) error {
			marked.Add(1)
			return nil
		},
		createAuditEventFn: func(context.Context, *domain.AuditEvent) error { return nil },
	}

	r := newSignedAutoRotateReaper(ms)
	r.rotationWebhookClient = successfulRotationWebhookClient()
	r.autoRotateAPIKeys(context.Background())

	if created.Load() != 3 {
		t.Fatalf("created = %d, want 3", created.Load())
	}
	if marked.Load() != 3 {
		t.Fatalf("marked = %d, want 3", marked.Load())
	}
}

func TestAutoRotateAPIKeys_NilRotationDays_NoNextRotation(t *testing.T) {
	t.Parallel()
	var createdKey *domain.APIKey
	webhookURL := rotationWebhookURLForTest(t)
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return []domain.APIKey{
				{ID: "key-1", ProjectID: "proj-1", RotationIntervalDays: nil, RotationWebhookURL: webhookURL, RotationWebhookSecret: rotationWebhookSecretForTest()},
			}, nil
		},
		createAPIKeyFn: func(_ context.Context, key *domain.APIKey) error {
			createdKey = key
			key.ID = "new-key"
			return nil
		},
		markRotatedFn:      func(context.Context, string, string, time.Time) error { return nil },
		createAuditEventFn: func(context.Context, *domain.AuditEvent) error { return nil },
	}

	r := newSignedAutoRotateReaper(ms)
	r.rotationWebhookClient = successfulRotationWebhookClient()
	r.autoRotateAPIKeys(context.Background())

	if createdKey.NextRotationAt != nil {
		t.Fatal("NextRotationAt should be nil when RotationIntervalDays is nil")
	}
}

func rotationWebhookURLForTest(t *testing.T) string {
	t.Helper()
	return "https://rotation.example.test/hook"
}

func rotationWebhookSecretForTest() []byte {
	return []byte("ciphertext")
}

func newSignedAutoRotateReaper(s ReaperStore) *Reaper {
	return NewReaper(s, time.Second, 30*time.Second, 0, 0, false, nil).
		WithAllowPrivateEndpoints(true).
		WithRotationSecretDecryptor(stubSecretDecryptor{plaintext: []byte("rotation-secret")})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func successfulRotationWebhookClient() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})}
}

func TestAutoRotateAPIKeys_StoreNotImplemented_Noop(t *testing.T) {
	t.Parallel()
	// mockReaperStore does NOT implement AutoRotateAPIKeysStore
	ms := &mockReaperStore{}
	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.autoRotateAPIKeys(context.Background()) // should not panic
}

func TestNotifyRotationWebhook_BlocksPrivateURL(t *testing.T) {
	t.Parallel()

	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	r := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, false, nil)
	if err := r.notifyRotationWebhook(context.Background(), server.URL, nil, "old-key", "new-key", "strait_secret", "strait_secre", "proj-1"); err == nil {
		t.Fatal("expected private plaintext rotation webhook to be blocked")
	}

	if called.Load() != 0 {
		t.Fatalf("private rotation webhook was called %d times, want 0", called.Load())
	}
}

func TestNotifyRotationWebhook_BlocksPlaintextEvenWithPrivateEndpoints(t *testing.T) {
	t.Parallel()

	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	r := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, false, nil).WithAllowPrivateEndpoints(true)
	if err := r.notifyRotationWebhook(context.Background(), server.URL, nil, "old-key", "new-key", "strait_secret", "strait_secre", "proj-1"); err == nil {
		t.Fatal("expected plaintext rotation webhook to be blocked")
	}

	if called.Load() != 0 {
		t.Fatalf("rotation webhook was called %d times, want 0", called.Load())
	}
}

func TestAutoRotateAPIKeys_WebhookFailureRevokesNewKeyKeepsOldActive(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	rotationDays := 30
	var marked atomic.Int32
	var created atomic.Int32
	var revoked atomic.Int32
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return []domain.APIKey{{
				ID:                    "old-key-1",
				ProjectID:             "proj-1",
				OrgID:                 "org-1",
				Name:                  "My Key",
				RotationIntervalDays:  &rotationDays,
				RotationWebhookURL:    server.URL,
				RotationWebhookSecret: rotationWebhookSecretForTest(),
			}}, nil
		},
		createAPIKeyFn: func(_ context.Context, key *domain.APIKey) error {
			if key.OrgID != "org-1" {
				t.Fatalf("new key org = %q, want org-1", key.OrgID)
			}
			created.Add(1)
			return nil
		},
		markRotatedFn: func(context.Context, string, string, time.Time) error {
			marked.Add(1)
			return nil
		},
		revokeAPIKeyFn: func(_ context.Context, id string) error {
			revoked.Add(1)
			return nil
		},
	}

	r := newSignedAutoRotateReaper(ms)
	r.rotationWebhookClient = server.Client()
	r.autoRotateAPIKeys(context.Background())

	if marked.Load() != 0 {
		t.Fatalf("marked old key rotated %d times, want 0 when delivery fails", marked.Load())
	}
	if created.Load() != 1 {
		t.Fatalf("created rotated keys = %d, want 1 because the key is persisted before webhook delivery", created.Load())
	}
	if revoked.Load() != 1 {
		t.Fatalf("revoked rotated keys = %d, want 1 because delivery failed after persistence", revoked.Load())
	}
}

func TestAutoRotateAPIKeys_MarksOldKeyOnlyAfterWebhookDelivery(t *testing.T) {
	t.Parallel()

	var delivered atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		delivered.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rotationDays := 30
	var marked atomic.Int32
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return []domain.APIKey{{
				ID:                    "old-key-1",
				ProjectID:             "proj-1",
				Name:                  "My Key",
				RotationIntervalDays:  &rotationDays,
				RotationWebhookURL:    server.URL,
				RotationWebhookSecret: rotationWebhookSecretForTest(),
			}}, nil
		},
		createAPIKeyFn: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "new-key-1"
			return nil
		},
		markRotatedFn: func(context.Context, string, string, time.Time) error {
			if delivered.Load() != 1 {
				t.Fatal("old key was marked rotated before webhook delivery")
			}
			marked.Add(1)
			return nil
		},
		createAuditEventFn: func(context.Context, *domain.AuditEvent) error { return nil },
	}

	r := newSignedAutoRotateReaper(ms)
	r.rotationWebhookClient = server.Client()
	r.autoRotateAPIKeys(context.Background())

	if marked.Load() != 1 {
		t.Fatalf("marked old key rotated %d times, want 1", marked.Load())
	}
}
