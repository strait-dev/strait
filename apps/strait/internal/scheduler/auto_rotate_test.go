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
)

type mockAutoRotateStore struct {
	mockReaperStore
	listDueRotationFn  func(ctx context.Context) ([]domain.APIKey, error)
	createAPIKeyFn     func(ctx context.Context, key *domain.APIKey) error
	markRotatedFn      func(ctx context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error
	revokeAPIKeyFn     func(ctx context.Context, id string) error
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
					ID:                   "old-key-1",
					ProjectID:            "proj-1",
					OrgID:                "org-1",
					Name:                 "My Key",
					Scopes:               []string{"jobs:read", "jobs:write"},
					EnvironmentID:        "env-prod",
					RotationIntervalDays: &rotationDays30,
					RotationWebhookURL:   webhookServer.URL,
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

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil).WithAllowPrivateEndpoints(true)
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
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil).WithAllowPrivateEndpoints(true)
	r.rotationWebhookClient = successfulRotationWebhookClient()
	r.autoRotateAPIKeys(context.Background())

	if created.Load() != 0 {
		t.Fatalf("created keys = %d, want 0 without rotation webhook", created.Load())
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
				{ID: "key-1", ProjectID: "proj-1", RotationIntervalDays: &rotationDays7, RotationWebhookURL: webhookURL},
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

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.autoRotateAPIKeys(context.Background())

	if markCalled.Load() != 0 {
		t.Fatal("should not mark rotated when create fails")
	}
}

func TestAutoRotateAPIKeys_MarkFailsRevokesNewKey(t *testing.T) {
	t.Parallel()

	rotationDays := 30
	webhookURL := rotationWebhookURLForTest(t)
	var revokedID string
	var auditCalled atomic.Int32
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return []domain.APIKey{{
				ID:                   "old-key-1",
				ProjectID:            "proj-1",
				Name:                 "My Key",
				RotationIntervalDays: &rotationDays,
				RotationWebhookURL:   webhookURL,
			}}, nil
		},
		createAPIKeyFn: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "new-key-orphan"
			return nil
		},
		markRotatedFn: func(context.Context, string, string, time.Time) error {
			return context.Canceled
		},
		revokeAPIKeyFn: func(_ context.Context, id string) error {
			revokedID = id
			return nil
		},
		createAuditEventFn: func(context.Context, *domain.AuditEvent) error {
			auditCalled.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil).WithAllowPrivateEndpoints(true)
	r.rotationWebhookClient = successfulRotationWebhookClient()
	r.autoRotateAPIKeys(context.Background())

	if revokedID != "new-key-orphan" {
		t.Fatalf("revokedID = %q, want new-key-orphan", revokedID)
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
				{ID: "key-1", ProjectID: "proj-1", RotationIntervalDays: &days7, RotationWebhookURL: webhookURL},
				{ID: "key-2", ProjectID: "proj-2", RotationIntervalDays: &days14, RotationWebhookURL: webhookURL},
				{ID: "key-3", ProjectID: "proj-1", RotationIntervalDays: &days30, RotationWebhookURL: webhookURL},
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

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil).WithAllowPrivateEndpoints(true)
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
				{ID: "key-1", ProjectID: "proj-1", RotationIntervalDays: nil, RotationWebhookURL: webhookURL},
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

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil).WithAllowPrivateEndpoints(true)
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

func TestAutoRotateAPIKeys_WebhookFailureKeepsOldKeyActive(t *testing.T) {
	t.Parallel()

	rotationDays := 30
	var marked atomic.Int32
	var revokedID string
	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return []domain.APIKey{{
				ID:                   "old-key-1",
				ProjectID:            "proj-1",
				OrgID:                "org-1",
				Name:                 "My Key",
				RotationIntervalDays: &rotationDays,
				RotationWebhookURL:   "http://rotation.example.test/hook",
			}}, nil
		},
		createAPIKeyFn: func(_ context.Context, key *domain.APIKey) error {
			if key.OrgID != "org-1" {
				t.Fatalf("new key org = %q, want org-1", key.OrgID)
			}
			key.ID = "new-undelivered-key"
			return nil
		},
		markRotatedFn: func(context.Context, string, string, time.Time) error {
			marked.Add(1)
			return nil
		},
		revokeAPIKeyFn: func(_ context.Context, id string) error {
			revokedID = id
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.autoRotateAPIKeys(context.Background())

	if marked.Load() != 0 {
		t.Fatalf("marked old key rotated %d times, want 0 when delivery fails", marked.Load())
	}
	if revokedID != "new-undelivered-key" {
		t.Fatalf("revokedID = %q, want new-undelivered-key", revokedID)
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
				ID:                   "old-key-1",
				ProjectID:            "proj-1",
				Name:                 "My Key",
				RotationIntervalDays: &rotationDays,
				RotationWebhookURL:   server.URL,
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

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil).WithAllowPrivateEndpoints(true)
	r.rotationWebhookClient = server.Client()
	r.autoRotateAPIKeys(context.Background())

	if marked.Load() != 1 {
		t.Fatalf("marked old key rotated %d times, want 1", marked.Load())
	}
}
