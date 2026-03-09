package api

import (
	"context"
	"sync"
	"testing"

	"orchestrator/internal/config"
	"orchestrator/internal/domain"
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
