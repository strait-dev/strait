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
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.autoRotateAPIKeys(context.Background())

	if createdKey == nil {
		t.Fatal("expected new key to be created")
	}
	if createdKey.ProjectID != "proj-1" {
		t.Fatalf("new key project = %q, want proj-1", createdKey.ProjectID)
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

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.autoRotateAPIKeys(context.Background())

	if created.Load() != 0 {
		t.Fatalf("created keys = %d, want 0 without rotation webhook", created.Load())
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

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
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

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.autoRotateAPIKeys(context.Background())

	if createdKey.NextRotationAt != nil {
		t.Fatal("NextRotationAt should be nil when RotationIntervalDays is nil")
	}
}

func rotationWebhookURLForTest(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func TestAutoRotateAPIKeys_StoreNotImplemented_Noop(t *testing.T) {
	t.Parallel()
	// mockReaperStore does NOT implement AutoRotateAPIKeysStore
	ms := &mockReaperStore{}
	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.autoRotateAPIKeys(context.Background()) // should not panic
}
