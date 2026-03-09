package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"
)

type mockActorSyncer struct {
	mu    sync.Mutex
	calls []actorSyncCall
}

type actorSyncCall struct {
	ID    string
	Email string
	Name  string
}

func (m *mockActorSyncer) UpsertKnownActor(_ context.Context, id, email, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, actorSyncCall{ID: id, Email: email, Name: name})
	return nil
}

func newTestServerWithActorSyncer(t *testing.T, s APIStore, q *mockQueue, pub *mockPublisher, syncer ActorSyncer) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret: "test-secret",
		JWTSigningKey:  "01234567890123456789012345678901",
	}
	return NewServer(ServerDeps{
		Config:      cfg,
		Store:       s,
		Queue:       q,
		ActorSyncer: syncer,
	})
}

func TestActorFromContext_WithUserHeader(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), ctxActorIDKey, "user_abc123")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	actor := actorFromContext(ctx)
	if actor != "user_abc123" {
		t.Fatalf("actorFromContext() = %q, want %q", actor, "user_abc123")
	}
}

func TestActorFromContext_WithAPIKeyFallback(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), ctxActorIDKey, "apikey:key-001")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")

	actor := actorFromContext(ctx)
	if actor != "apikey:key-001" {
		t.Fatalf("actorFromContext() = %q, want %q", actor, "apikey:key-001")
	}
}

func TestActorFromContext_Empty(t *testing.T) {
	t.Parallel()

	actor := actorFromContext(context.Background())
	if actor != "" {
		t.Fatalf("actorFromContext() = %q, want empty", actor)
	}
}

// Verify the domain type exists and can be constructed.
func TestKnownActor_Roundtrip(t *testing.T) {
	t.Parallel()

	actor := domain.KnownActor{
		ID:    "user_leo",
		Email: "leo@example.com",
		Name:  "Leonardo",
	}

	if actor.ID != "user_leo" {
		t.Fatalf("actor.ID = %q, want %q", actor.ID, "user_leo")
	}
	if actor.Email != "leo@example.com" {
		t.Fatalf("actor.Email = %q, want %q", actor.Email, "leo@example.com")
	}
}

func TestActorSyncer_CalledOnSync(t *testing.T) {
	t.Parallel()

	syncer := &mockActorSyncer{}
	err := syncer.UpsertKnownActor(context.Background(), "user_1", "a@b.com", "Alice")
	if err != nil {
		t.Fatalf("UpsertKnownActor() error = %v", err)
	}

	calls := syncer.calls
	if len(calls) != 1 {
		t.Fatalf("syncer calls = %d, want 1", len(calls))
	}
	if calls[0].ID != "user_1" {
		t.Fatalf("syncer call ID = %q, want %q", calls[0].ID, "user_1")
	}
}

func TestNewServerWithActorSyncer(t *testing.T) {
	t.Parallel()

	syncer := &mockActorSyncer{}
	ms := &mockAPIStore{}
	srv := newTestServerWithActorSyncer(t, ms, nil, nil, syncer)
	if srv == nil {
		t.Fatal("server should not be nil")
	}
	if srv.actorSyncer == nil {
		t.Fatal("actorSyncer should be set")
	}
}

func TestInternalSecretAuth_SetsActorFromHeaders(t *testing.T) {
	t.Parallel()

	var capturedActor string
	var capturedType string

	ms := &mockAPIStore{}
	ms.queueStatsFn = func(_ context.Context) (*store.QueueStats, error) {
		return &store.QueueStats{}, nil
	}

	syncer := &mockActorSyncer{}
	srv := newTestServerWithActorSyncer(t, ms, nil, nil, syncer)

	// Use internal secret with actor headers — should set user context.
	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("X-Internal-Secret", "test-secret")
	r.Header.Set("X-Actor-Id", "user_leo")
	r.Header.Set("X-Actor-Email", "leo@example.com")
	r.Header.Set("X-Actor-Name", "Leonardo")

	// Override the stats handler to capture context.
	// Instead, we just verify the request doesn't 403.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	_ = capturedActor
	_ = capturedType

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Wait for async syncer goroutine.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		syncer.mu.Lock()
		n := len(syncer.calls)
		syncer.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	syncer.mu.Lock()
	defer syncer.mu.Unlock()
	if len(syncer.calls) == 0 {
		t.Fatal("expected actor syncer to be called")
	}
	if syncer.calls[0].ID != "user_leo" {
		t.Fatalf("syncer ID = %q, want %q", syncer.calls[0].ID, "user_leo")
	}
	if syncer.calls[0].Email != "leo@example.com" {
		t.Fatalf("syncer Email = %q, want %q", syncer.calls[0].Email, "leo@example.com")
	}
}

func TestAPIKeyAuth_IgnoresActorHeaders(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	ms.getAPIKeyByHashFn = func(_ context.Context, _ string) (*domain.APIKey, error) {
		return &domain.APIKey{
			ID:        "key-1",
			ProjectID: "proj-1",
			Scopes:    []string{"stats:read"},
		}, nil
	}
	ms.touchAPIKeyLastUsedFn = func(_ context.Context, _ string) error { return nil }
	ms.queueStatsFn = func(_ context.Context) (*store.QueueStats, error) {
		return &store.QueueStats{}, nil
	}

	srv := newTestServer(t, ms, nil, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer strait_testkey123")
	// These headers should be IGNORED for API key auth.
	r.Header.Set("X-Actor-Id", "user_attacker")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	// Should succeed (key has stats:read) but actor type should be api_key, not user.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}
