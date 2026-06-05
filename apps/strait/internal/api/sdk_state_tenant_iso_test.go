package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"
)

// activeRunReadMock implements activeRunMutationStore plus runTokenStateGetter.
// It returns ErrRunConflict from every ForActiveRun method so the SDK guard
// branches into revalidateRunTokenState, which then surfaces 410 (terminal)
// or 401 (stale attempt) based on the configured token state.
type activeRunReadMock struct {
	*APIStoreMock
	tokenStatus  domain.RunStatus
	tokenAttempt int
}

func (m *activeRunReadMock) GetRunTokenState(_ context.Context, _ string) (domain.RunStatus, int, string, error) {
	return m.tokenStatus, m.tokenAttempt, "proj-1", nil
}

func (m *activeRunReadMock) EnsureRunActiveForAttempt(context.Context, string, int) error {
	return store.ErrRunConflict
}

func (m *activeRunReadMock) InsertEventForActiveRun(context.Context, *domain.RunEvent, int) error {
	return store.ErrRunConflict
}

func (m *activeRunReadMock) UpdateRunMetadataForActiveRun(context.Context, string, map[string]string, int) error {
	return store.ErrRunConflict
}

func (m *activeRunReadMock) UpdateHeartbeatForActiveRun(context.Context, string, int) error {
	return store.ErrRunConflict
}

func (m *activeRunReadMock) CreateRunCheckpointForActiveRun(context.Context, *domain.RunCheckpoint, int) error {
	return store.ErrRunConflict
}

func (m *activeRunReadMock) UpsertRunStateForActiveRun(context.Context, *domain.RunState, int) error {
	return store.ErrRunConflict
}

func (m *activeRunReadMock) GetRunStateForActiveRun(context.Context, string, string, int) (*domain.RunState, error) {
	return nil, store.ErrRunConflict
}

func (m *activeRunReadMock) ListRunStateForActiveRun(context.Context, string, int) ([]domain.RunState, error) {
	return nil, store.ErrRunConflict
}

func (m *activeRunReadMock) DeleteRunStateForActiveRun(context.Context, string, string, int) error {
	return store.ErrRunConflict
}

func (m *activeRunReadMock) UpsertRunOutputForActiveRun(context.Context, *domain.RunOutput, int) error {
	return store.ErrRunConflict
}

func (m *activeRunReadMock) UpsertJobMemoryWithQuotaForActiveRun(context.Context, string, *domain.JobMemory, int, int, int) error {
	return store.ErrRunConflict
}

func (m *activeRunReadMock) GetJobMemoryForActiveRun(context.Context, string, string, string, int) (*domain.JobMemory, error) {
	return nil, store.ErrRunConflict
}

func (m *activeRunReadMock) ListJobMemoryForActiveRun(context.Context, string, string, int) ([]domain.JobMemory, error) {
	return nil, store.ErrRunConflict
}

func (m *activeRunReadMock) DeleteJobMemoryForActiveRun(context.Context, string, string, string, int) error {
	return store.ErrRunConflict
}

func (m *activeRunReadMock) CreateRunResourceSnapshotForActiveRun(context.Context, *domain.RunResourceSnapshot, int) error {
	return store.ErrRunConflict
}

func (m *activeRunReadMock) CreateRunIterationForActiveRun(context.Context, *domain.RunIteration, int) error {
	return store.ErrRunConflict
}

func (m *activeRunReadMock) UpdateRunStatusForActiveRun(context.Context, string, domain.RunStatus, domain.RunStatus, map[string]any, int) error {
	return store.ErrRunConflict
}

func sdkCtxForRun(runID string, attempt int) context.Context {
	ctx := context.WithValue(context.Background(), ctxRunIDKey, runID)
	ctx = context.WithValue(ctx, ctxRunAttemptKey, attempt)
	return ctx
}

func TestTenantIso_ListRunState_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-bbb"}, nil
		},
		ListRunStateFunc: func(context.Context, string) ([]domain.RunState, error) {
			t.Fatal("ListRunState should not be reached when project guard rejects")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleListRunState(ctx, &ListRunStateInput{RunID: "run-x"})
	if err == nil {
		t.Fatal("expected error for cross-project list")
	}
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404 cross-project, got %v", err)
	}
}

func TestTenantIso_ListRunState_RejectsCrossEnv(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-aaa", JobID: "job-1"}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-aaa", EnvironmentID: "env-prod"}, nil
		},
		ListRunStateFunc: func(context.Context, string) ([]domain.RunState, error) {
			t.Fatal("ListRunState should not be reached when env guard rejects")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-staging")
	_, err := srv.handleListRunState(ctx, &ListRunStateInput{RunID: "run-x"})
	if err == nil {
		t.Fatal("expected error for cross-env list")
	}
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404 cross-env, got %v", err)
	}
}

func TestTenantIso_SDKGetState_RejectsTerminalRun(t *testing.T) {
	t.Parallel()
	ms := &activeRunReadMock{
		APIStoreMock: &APIStoreMock{},
		tokenStatus:  domain.StatusCompleted,
		tokenAttempt: 1,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := sdkCtxForRun("run-1", 1)
	_, err := srv.handleSDKGetState(ctx, &SDKGetStateInput{RunID: "run-1", Key: "k"})
	if err == nil {
		t.Fatal("expected error for terminal run")
	}
	if !isHumaStatusError(err, http.StatusGone) {
		t.Fatalf("expected 410 Gone, got %v", err)
	}
}

func TestTenantIso_SDKListState_RejectsTerminalRun(t *testing.T) {
	t.Parallel()
	ms := &activeRunReadMock{
		APIStoreMock: &APIStoreMock{},
		tokenStatus:  domain.StatusFailed,
		tokenAttempt: 1,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := sdkCtxForRun("run-1", 1)
	_, err := srv.handleSDKListState(ctx, &SDKRunIDInput{RunID: "run-1"})
	if err == nil {
		t.Fatal("expected error for terminal run")
	}
	if !isHumaStatusError(err, http.StatusGone) {
		t.Fatalf("expected 410 Gone, got %v", err)
	}
}

func TestTenantIso_SDKGetMemory_RejectsTerminalRun(t *testing.T) {
	t.Parallel()
	ms := &activeRunReadMock{
		APIStoreMock: &APIStoreMock{
			GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
				return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1"}, nil
			},
		},
		tokenStatus:  domain.StatusCanceled,
		tokenAttempt: 1,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := sdkCtxForRun("run-1", 1)
	_, err := srv.handleSDKGetMemory(ctx, &SDKGetMemoryInput{RunID: "run-1", Key: "cache"})
	if err == nil {
		t.Fatal("expected error for terminal run")
	}
	if !isHumaStatusError(err, http.StatusGone) {
		t.Fatalf("expected 410 Gone, got %v", err)
	}
}

func TestTenantIso_SDKListMemory_RejectsTerminalRun(t *testing.T) {
	t.Parallel()
	ms := &activeRunReadMock{
		APIStoreMock: &APIStoreMock{
			GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
				return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1"}, nil
			},
		},
		tokenStatus:  domain.StatusTimedOut,
		tokenAttempt: 1,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := sdkCtxForRun("run-1", 1)
	_, err := srv.handleSDKListMemory(ctx, &SDKRunIDInput{RunID: "run-1"})
	if err == nil {
		t.Fatal("expected error for terminal run")
	}
	if !isHumaStatusError(err, http.StatusGone) {
		t.Fatalf("expected 410 Gone, got %v", err)
	}
}

func TestTenantIso_SDKComplete_RejectsStaleAttempt(t *testing.T) {
	t.Parallel()
	ms := &activeRunReadMock{
		APIStoreMock: &APIStoreMock{
			GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
				return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusExecuting, Attempt: 2}, nil
			},
		},
		tokenStatus:  domain.StatusExecuting,
		tokenAttempt: 2,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := sdkCtxForRun("run-1", 1)
	_, err := srv.handleSDKComplete(ctx, &SDKCompleteInput{RunID: "run-1", Body: SDKCompleteRequest{Result: json.RawMessage(`{"ok":true}`)}})
	if err == nil {
		t.Fatal("expected error when token attempt is stale")
	}
	if !isHumaStatusError(err, http.StatusUnauthorized) {
		t.Fatalf("expected 401 stale attempt, got %v", err)
	}
}

func TestTenantIso_SDKFail_RejectsStaleAttempt(t *testing.T) {
	t.Parallel()
	ms := &activeRunReadMock{
		APIStoreMock: &APIStoreMock{
			GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
				return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusExecuting, Attempt: 2}, nil
			},
		},
		tokenStatus:  domain.StatusExecuting,
		tokenAttempt: 2,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := sdkCtxForRun("run-1", 1)
	_, err := srv.handleSDKFail(ctx, &SDKFailInput{RunID: "run-1", Body: SDKFailRequest{Error: "boom"}})
	if err == nil {
		t.Fatal("expected error when token attempt is stale")
	}
	if !isHumaStatusError(err, http.StatusUnauthorized) {
		t.Fatalf("expected 401 stale attempt, got %v", err)
	}
}

// noRunTokenStateStore wraps APIStoreMock but explicitly does NOT implement
// runTokenStateGetter, by hiding the auto-injected GetRunTokenState method
// behind a struct embedding indirection. Tests use this to simulate a
// misconfigured store and verify runTokenAuth fails closed.
type noRunTokenStateStore struct {
	APIStore
}

// TestTenantIso_SDKAuth_FallbackAttemptDoesNotBypass verifies that when the
// configured store does not implement runTokenStateGetter (a misconfigured
// test fake), runTokenAuth fails closed with 401 instead of letting attempt
// default to zero and bypassing the staleness check at sdk.go:218.
func TestTenantIso_SDKAuth_FallbackAttemptDoesNotBypass(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testSigningKey,
	}
	// Wrap the mock so the runTokenStateGetter type assertion fails. The
	// outer noRunTokenStateStore promotes only APIStore methods, leaving
	// GetRunTokenState behind on the embedded mock and unreachable through
	// the wrapper.
	store := &noRunTokenStateStore{APIStore: &APIStoreMock{}}
	srv := NewServer(ServerDeps{Config: cfg, Store: store, Queue: &mockQueue{}})
	t.Cleanup(srv.Close)

	var called atomic.Bool
	handler := srv.runTokenAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called.Store(true)
	}))
	tok := signTokenWithAttempt(t, testSigningKey, "run-1", time.Now().Add(time.Hour), 1, "")
	r := authRequest(t, "run-1")
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 fail-closed fallback, got %d: %s", w.Code, w.Body.String())
	}
	if called.Load() {
		t.Fatal("next handler should not have been called")
	}
}
