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

	"github.com/stretchr/testify/require"
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
	require.Equal(t, "user_abc123",
		actor)

}

func TestActorFromContext_WithAPIKeyFallback(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), ctxActorIDKey, "apikey:key-001")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")

	actor := actorFromContext(ctx)
	require.Equal(t, "apikey:key-001",
		actor)

}

func TestActorFromContext_Empty(t *testing.T) {
	t.Parallel()

	actor := actorFromContext(context.Background())
	require.Equal(t, "", actor)

}

// Verify the domain type exists and can be constructed.
func TestKnownActor_Roundtrip(t *testing.T) {
	t.Parallel()

	actor := domain.KnownActor{
		ID:    "user_leo",
		Email: "leo@example.com",
		Name:  "Leonardo",
	}
	require.Equal(t, "user_leo",
		actor.ID)
	require.Equal(t, "leo@example.com",
		actor.Email,
	)

}

func TestActorSyncer_CalledOnSync(t *testing.T) {
	t.Parallel()

	syncer := &mockActorSyncer{}
	err := syncer.UpsertKnownActor(context.Background(), "user_1", "a@b.com", "Alice")
	require.NoError(t, err)

	calls := syncer.calls
	require.Len(t,
		calls, 1)
	require.Equal(t, "user_1",
		calls[0].ID)

}

func TestNewServerWithActorSyncer(t *testing.T) {
	t.Parallel()

	syncer := &mockActorSyncer{}
	ms := &APIStoreMock{}
	srv := newTestServerWithActorSyncer(t, ms, nil, nil, syncer)
	require.NotNil(t, srv)
	require.NotNil(t, srv.actorSyncer)

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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Equal(t, "user_leo",
		capturedActor)
	require.Equal(t, "user",
		capturedType)
	require.Equal(t, "proj-internal",
		capturedProjectID,
	)

	// Verify actor context was set on the request for downstream handlers.

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
	require.NotEmpty(t, syncer.
		calls)
	require.Equal(t, "user_leo",
		syncer.calls[0].ID,
	)
	require.Equal(t, "leo@example.com",
		syncer.calls[0].Email,
	)

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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Equal(t, "apikey:key-1",
		capturedActor,
	)
	require.Equal(t, "api_key",
		capturedType)

	// Verify actor is the API key, NOT the attacker headers.

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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Equal(t, "", capturedActor)

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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.NotEqual(t, "user",
		capturedType)

	// Empty X-Actor-Id should NOT set actor type to "user".

}

func TestAPIKeyAuth_MissingAuthHeader(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	// No Authorization header, no X-Internal-Secret.

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,
		w.Code,
	)

}

func TestAPIKeyAuth_InvalidBearerPrefix(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer invalid_not_strait_prefix")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,
		w.Code,
	)

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
	require.Equal(t, http.StatusOK,
		w.Code)

	// Should not panic even though syncer is nil.

}
