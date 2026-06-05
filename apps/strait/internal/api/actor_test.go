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
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:      cfg,
		Store:       s,
		Queue:       q,
		ActorSyncer: syncer,
	})
	t.Cleanup(srv.Close)
	return srv
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
	ms := &APIStoreMock{}
	srv := newTestServerWithActorSyncer(t, ms, nil, nil, syncer)
	if srv == nil {
		t.Fatal("server should not be nil")
		return
	}
	if srv.actorSyncer == nil {
		t.Fatal("actorSyncer should be set")
	}
}

func TestInternalSecretAuth_SetsActorFromHeaders(t *testing.T) {
	t.Parallel()

	var capturedActor string
	var capturedType string
	var capturedProjectID string

	ms := &APIStoreMock{}
	ms.QueueStatsFunc = func(ctx context.Context) (*store.QueueStats, error) {
		// Capture actor/project context set by middleware.
		capturedActor = actorFromContext(ctx)
		capturedProjectID = projectIDFromContext(ctx)
		if v, ok := ctx.Value(ctxActorTypeKey).(string); ok {
			capturedType = v
		}
		return &store.QueueStats{}, nil
	}

	syncer := &mockActorSyncer{}
	srv := newTestServerWithActorSyncer(t, ms, nil, nil, syncer)

	// Use internal secret with actor headers — should set user context.
	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("X-Internal-Secret", "test-secret-value")
	r.Header.Set("X-Actor-Id", "user_leo")
	r.Header.Set("X-Actor-Email", "leo@example.com")
	r.Header.Set("X-Actor-Name", "Leonardo")
	r.Header.Set("X-Project-Id", "proj-internal")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify actor context was set on the request for downstream handlers.
	if capturedActor != "user_leo" {
		t.Fatalf("actorFromContext = %q, want %q", capturedActor, "user_leo")
	}
	if capturedType != "user" {
		t.Fatalf("actorType = %q, want %q", capturedType, "user")
	}
	if capturedProjectID != "proj-internal" {
		t.Fatalf("projectID = %q, want %q", capturedProjectID, "proj-internal")
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

	var capturedActor string
	var capturedType string

	ms := &APIStoreMock{}
	ms.GetAPIKeyByHashFunc = func(_ context.Context, _ string) (*domain.APIKey, error) {
		return &domain.APIKey{
			ID:        "key-1",
			ProjectID: "proj-1",
			Scopes:    []string{"stats:read"},
		}, nil
	}
	ms.TouchAPIKeyLastUsedFunc = func(_ context.Context, _ string) error { return nil }
	ms.QueueStatsFunc = func(ctx context.Context) (*store.QueueStats, error) {
		capturedActor = actorFromContext(ctx)
		if v, ok := ctx.Value(ctxActorTypeKey).(string); ok {
			capturedType = v
		}
		return &store.QueueStats{}, nil
	}

	srv := newTestServer(t, ms, nil, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer strait_testkey123")
	// These headers should be IGNORED for API key auth.
	r.Header.Set("X-Actor-Id", "user_attacker")
	r.Header.Set("X-Actor-Email", "attacker@evil.com")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify actor is the API key, NOT the attacker headers.
	if capturedActor != "apikey:key-1" {
		t.Fatalf("actorFromContext = %q, want %q (should be API key, not impersonated user)", capturedActor, "apikey:key-1")
	}
	if capturedType != "api_key" {
		t.Fatalf("actorType = %q, want %q", capturedType, "api_key")
	}
}

func TestInternalSecretAuth_NoActorHeaders(t *testing.T) {
	t.Parallel()

	var capturedActor string

	ms := &APIStoreMock{}
	ms.QueueStatsFunc = func(ctx context.Context) (*store.QueueStats, error) {
		capturedActor = actorFromContext(ctx)
		return &store.QueueStats{}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("X-Internal-Secret", "test-secret-value")
	// No X-Actor-Id header.

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturedActor != "" {
		t.Fatalf("actorFromContext = %q, want empty (no actor headers)", capturedActor)
	}
}

func TestInternalSecretAuth_EmptyActorID(t *testing.T) {
	t.Parallel()

	var capturedType string

	ms := &APIStoreMock{}
	ms.QueueStatsFunc = func(ctx context.Context) (*store.QueueStats, error) {
		if v, ok := ctx.Value(ctxActorTypeKey).(string); ok {
			capturedType = v
		}
		return &store.QueueStats{}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("X-Internal-Secret", "test-secret-value")
	r.Header.Set("X-Actor-Id", "") // Empty string.

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	// Empty X-Actor-Id should NOT set actor type to "user".
	if capturedType == "user" {
		t.Fatal("empty X-Actor-Id should not result in actor type 'user'")
	}
}

func TestAPIKeyAuth_MissingAuthHeader(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	// No Authorization header, no X-Internal-Secret.

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAPIKeyAuth_InvalidBearerPrefix(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer invalid_not_strait_prefix")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestActorSyncer_NilSyncer(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.QueueStatsFunc = func(_ context.Context) (*store.QueueStats, error) {
		return &store.QueueStats{}, nil
	}
	// Create server without ActorSyncer (nil).
	srv := newTestServer(t, ms, nil, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("X-Internal-Secret", "test-secret-value")
	r.Header.Set("X-Actor-Id", "user_leo")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	// Should not panic even though syncer is nil.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (nil syncer should not panic)", w.Code, http.StatusOK)
	}
}
