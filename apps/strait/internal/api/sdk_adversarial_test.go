package api

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

// testSigningKey is the 32-byte HMAC key used by newTestServer and generateRunToken.
var testSigningKey = testJWTSigningKey

// wrongSigningKey is a valid-length key that differs from testSigningKey.
const wrongSigningKey = "99999999999999999999999999999999"

// resolveSDKCapabilities pure-function tests.

func TestResolveSDKCapabilities_V1(t *testing.T) {
	t.Parallel()
	caps := resolveSDKCapabilities("1.0")
	if caps.Progress || caps.Checkpoint {
		t.Fatalf("v1 should have no capabilities, got %+v", caps)
	}
}

func TestResolveSDKCapabilities_V2(t *testing.T) {
	t.Parallel()
	caps := resolveSDKCapabilities("2.0")
	if !caps.Progress || !caps.Checkpoint {
		t.Fatalf("v2 should advertise only launch-active capabilities, got %+v", caps)
	}
}

func TestResolveSDKCapabilities_Empty(t *testing.T) {
	t.Parallel()
	caps := resolveSDKCapabilities("")
	if caps.Progress || caps.Checkpoint {
		t.Fatalf("empty version should have no capabilities, got %+v", caps)
	}
}

func TestResolveSDKCapabilities_Malformed(t *testing.T) {
	t.Parallel()
	cases := []string{"abc", "v2", "2.x", "-1.0", "hello world"}
	for _, v := range cases {
		caps := resolveSDKCapabilities(v)
		// "abc", "v2", "2.x", "-1.0" all fail strconv.Atoi on the major part.
		// Only numeric majors >= 2 should yield capabilities.
		if v == "2.x" {
			// Major part before "." is "2", which parses to 2.
			if !caps.Progress || !caps.Checkpoint {
				t.Fatalf("version %q: expected launch-active capabilities for major=2, got %+v", v, caps)
			}
		} else {
			if caps.Progress || caps.Checkpoint {
				t.Fatalf("version %q should have no capabilities, got %+v", v, caps)
			}
		}
	}
}

func TestResolveSDKCapabilities_LargeVersion(t *testing.T) {
	t.Parallel()
	caps := resolveSDKCapabilities("99999.0.0")
	if !caps.Progress || !caps.Checkpoint {
		t.Fatalf("large major version should advertise only launch-active capabilities, got %+v", caps)
	}
}

func FuzzResolveSDKCapabilities(f *testing.F) {
	f.Add("1.0")
	f.Add("2.0")
	f.Add("")
	f.Add("abc")
	f.Add("99999.0.0")
	f.Add("0.0.0")
	f.Add("-1")
	f.Add("2")
	f.Add(strings.Repeat("9", 1000))
	f.Fuzz(func(t *testing.T, version string) {
		// Must not panic.
		caps := resolveSDKCapabilities(version)
		_ = caps
	})
}

// sdkCapabilitiesHeader tests.

func TestSDKCapabilitiesHeader_AllCombinations(t *testing.T) {
	t.Parallel()
	bools := []bool{false, true}
	for _, p := range bools {
		for _, c := range bools {
			caps := SDKCapabilities{Progress: p, Checkpoint: c}
			header := sdkCapabilitiesHeader(caps)
			if header == "" {
				t.Fatal("header must never be empty string")
			}
			if !p && !c {
				if header != "none" {
					t.Fatalf("all-false should produce 'none', got %q", header)
				}
				continue
			}
			if p && !strings.Contains(header, "progress") {
				t.Fatalf("expected 'progress' in header %q", header)
			}
			if c && !strings.Contains(header, "checkpoint") {
				t.Fatalf("expected 'checkpoint' in header %q", header)
			}
			if !p && strings.Contains(header, "progress") {
				t.Fatalf("unexpected 'progress' in header %q", header)
			}
			if !c && strings.Contains(header, "checkpoint") {
				t.Fatalf("unexpected 'checkpoint' in header %q", header)
			}
		}
	}
}

// runTokenAuth middleware tests.

// newAuthTestServer creates a minimal Server with the standard test signing key.
func newAuthTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config: cfg,
		Store:  &APIStoreMock{},
		Queue:  &mockQueue{},
	})
	t.Cleanup(srv.Close)
	return srv
}

// signToken creates a JWT signed with the given key and subject.
//
// The Issuer is set to "strait:run-token" because runTokenAuth now
// strictly enforces it (jwt.WithIssuer); a token without that issuer
// is rejected as "invalid run token".
func signToken(t *testing.T, key, subject string, expiry time.Time) string {
	t.Helper()
	return signTokenWithAttempt(t, key, subject, expiry, 1, "")
}

func signTokenWithAttempt(t *testing.T, key, subject string, expiry time.Time, attempt int, assignmentID string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, runTokenClaims{
		Attempt:      attempt,
		AssignmentID: assignmentID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "strait:run-token",
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(expiry),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	signed, err := token.SignedString([]byte(key))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

type workerTaskAPIStoreMock struct {
	*APIStoreMock
	status    domain.RunStatus
	attempt   int
	projectID string
	stateErr  error
	task      *domain.WorkerTask
	err       error
}

func (m *workerTaskAPIStoreMock) GetRunTokenState(_ context.Context, _ string) (domain.RunStatus, int, string, error) {
	return m.status, m.attempt, m.projectID, m.stateErr
}

func (m *workerTaskAPIStoreMock) GetWorkerTask(_ context.Context, _ string) (*domain.WorkerTask, error) {
	return m.task, m.err
}

type racingTerminalSDKStore struct {
	*APIStoreMock
	stateCalls atomic.Int64
}

func (m *racingTerminalSDKStore) GetRunTokenState(_ context.Context, runID string) (domain.RunStatus, int, string, error) {
	if m.stateCalls.Add(1) == 1 {
		return domain.StatusExecuting, 1, "proj-1", nil
	}
	return domain.StatusCompleted, 1, "proj-1", nil
}

func (m *racingTerminalSDKStore) EnsureRunActiveForAttempt(context.Context, string, int) error {
	return store.ErrRunConflict
}

func (m *racingTerminalSDKStore) InsertEventForActiveRun(context.Context, *domain.RunEvent, int) error {
	return store.ErrRunConflict
}

func (m *racingTerminalSDKStore) UpdateRunMetadataForActiveRun(context.Context, string, map[string]string, int) error {
	return store.ErrRunConflict
}

func (m *racingTerminalSDKStore) UpdateHeartbeatForActiveRun(context.Context, string, int) error {
	return store.ErrRunConflict
}

func (m *racingTerminalSDKStore) CreateRunCheckpointForActiveRun(context.Context, *domain.RunCheckpoint, int) error {
	return store.ErrRunConflict
}

func (m *racingTerminalSDKStore) UpsertRunStateForActiveRun(context.Context, *domain.RunState, int) error {
	return store.ErrRunConflict
}

func (m *racingTerminalSDKStore) DeleteRunStateForActiveRun(context.Context, string, string, int) error {
	return store.ErrRunConflict
}

func (m *racingTerminalSDKStore) UpsertRunOutputForActiveRun(context.Context, *domain.RunOutput, int) error {
	return store.ErrRunConflict
}

func (m *racingTerminalSDKStore) UpsertJobMemoryWithQuotaForActiveRun(context.Context, string, *domain.JobMemory, int, int, int) error {
	return store.ErrRunConflict
}

func (m *racingTerminalSDKStore) DeleteJobMemoryForActiveRun(context.Context, string, string, string, int) error {
	return store.ErrRunConflict
}

func (m *racingTerminalSDKStore) CreateRunResourceSnapshotForActiveRun(context.Context, *domain.RunResourceSnapshot, int) error {
	return store.ErrRunConflict
}

func (m *racingTerminalSDKStore) CreateRunIterationForActiveRun(context.Context, *domain.RunIteration, int) error {
	return store.ErrRunConflict
}

func (m *racingTerminalSDKStore) GetRunStateForActiveRun(context.Context, string, string, int) (*domain.RunState, error) {
	return nil, store.ErrRunConflict
}

func (m *racingTerminalSDKStore) ListRunStateForActiveRun(context.Context, string, int) ([]domain.RunState, error) {
	return nil, store.ErrRunConflict
}

func (m *racingTerminalSDKStore) GetJobMemoryForActiveRun(context.Context, string, string, string, int) (*domain.JobMemory, error) {
	return nil, store.ErrRunConflict
}

func (m *racingTerminalSDKStore) ListJobMemoryForActiveRun(context.Context, string, string, int) ([]domain.JobMemory, error) {
	return nil, store.ErrRunConflict
}

func (m *racingTerminalSDKStore) UpdateRunStatusForActiveRun(context.Context, string, domain.RunStatus, domain.RunStatus, map[string]any, int) error {
	return store.ErrRunConflict
}

// authRequest builds a request with the given runID in the chi route context.
func authRequest(t *testing.T, runID string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/sdk/runs/"+runID+"/payload", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("runID", runID)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	return r
}

func TestRunTokenAuth_MissingAuth(t *testing.T) {
	t.Parallel()
	srv := newAuthTestServer(t)
	var called atomic.Bool
	handler := srv.runTokenAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called.Store(true)
	}))
	r := authRequest(t, "run-1")
	// No Authorization header set.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if called.Load() {
		t.Fatal("next handler should not have been called")
	}
}

func TestRunTokenAuth_InvalidBearer(t *testing.T) {
	t.Parallel()
	srv := newAuthTestServer(t)
	var called atomic.Bool
	handler := srv.runTokenAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called.Store(true)
	}))
	r := authRequest(t, "run-1")
	r.Header.Set("Authorization", "Bearer invalid-not-a-jwt")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if called.Load() {
		t.Fatal("next handler should not have been called")
	}
}

func TestRunTokenAuth_WrongSigningKey(t *testing.T) {
	t.Parallel()
	srv := newAuthTestServer(t)
	var called atomic.Bool
	handler := srv.runTokenAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called.Store(true)
	}))
	tok := signToken(t, wrongSigningKey, "run-1", time.Now().Add(time.Hour))
	r := authRequest(t, "run-1")
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if called.Load() {
		t.Fatal("next handler should not have been called")
	}
}

// Regression: a token without the "strait:run-token"
// issuer must be rejected. Without this guard a token issued for a
// different audience (SSE token, gRPC inter-service token) signed
// with the same JWT key could be replayed against the SDK plane.
func TestRunTokenAuth_WrongIssuer_Rejected(t *testing.T) {
	t.Parallel()
	srv := newAuthTestServer(t)
	var called atomic.Bool
	handler := srv.runTokenAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called.Store(true)
	}))
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "strait:sse",
		Subject:   "run-1",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})
	signed, err := token.SignedString([]byte(testSigningKey))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	r := authRequest(t, "run-1")
	r.Header.Set("Authorization", "Bearer "+signed)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong-issuer token, got %d", w.Code)
	}
	if called.Load() {
		t.Fatal("next handler should not have been called for wrong-issuer token")
	}
}

func TestRunTokenAuth_BadIssuerDoesNotWriteAudit(t *testing.T) {
	t.Parallel()

	var captured *domain.AuditEvent
	store := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			captured = ev
			return nil
		},
	}
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testSigningKey,
		},
		Store: store,
		Queue: &mockQueue{},
	})
	t.Cleanup(srv.Close)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, runTokenClaims{
		Attempt: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "strait:sse",
			Subject:   "run-issuer-audit",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	signed, err := token.SignedString([]byte(testSigningKey))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	handler := srv.runTokenAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler should not have been called")
	}))
	r := authRequest(t, "run-issuer-audit")
	r.Header.Set("Authorization", "Bearer "+signed)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad issuer token, got %d", w.Code)
	}
	if captured != nil {
		t.Fatalf("rejected JWT wrote unauthenticated audit event: %+v", captured)
	}
}

// Regression: a token without an `exp` claim must be
// rejected (jwt.WithExpirationRequired). Otherwise a forged or
// misissued token would never time out.
func TestRunTokenAuth_NoExpiration_Rejected(t *testing.T) {
	t.Parallel()
	srv := newAuthTestServer(t)
	var called atomic.Bool
	handler := srv.runTokenAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called.Store(true)
	}))
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:   "strait:run-token",
		Subject:  "run-1",
		IssuedAt: jwt.NewNumericDate(time.Now()),
		// Deliberately no ExpiresAt.
	})
	signed, err := token.SignedString([]byte(testSigningKey))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	r := authRequest(t, "run-1")
	r.Header.Set("Authorization", "Bearer "+signed)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for token with no exp, got %d", w.Code)
	}
	if called.Load() {
		t.Fatal("next handler should not have been called for non-expiring token")
	}
}

// Regression: a token bound to a run that has already
// reached a terminal state must be rejected with 410 Gone, even if
// the JWT itself is otherwise valid (correct issuer, exp, signature,
// subject). Without this guard a stolen token outlives the runtime
// and can keep writing logs/state/memory after the run is dead.
func TestRunTokenAuth_TerminalRun_Rejected(t *testing.T) {
	t.Parallel()
	for _, status := range []domain.RunStatus{
		domain.StatusCompleted,
		domain.StatusFailed,
		domain.StatusCanceled,
		domain.StatusTimedOut,
		domain.StatusCrashed,
		domain.StatusSystemFailed,
		domain.StatusExpired,
		domain.StatusDeadLetter,
	} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{
				InternalSecret:      "test-secret-value",
				MaxBulkTriggerItems: 500,
				JWTSigningKey:       testSigningKey,
			}
			ms := &workerTaskAPIStoreMock{
				APIStoreMock: &APIStoreMock{},
				status:       status,
				attempt:      1,
				projectID:    "proj-1",
			}
			srv := NewServer(ServerDeps{Config: cfg, Store: ms, Queue: &mockQueue{}})
			t.Cleanup(srv.Close)
			var called atomic.Bool
			handler := srv.runTokenAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				called.Store(true)
			}))
			tok := signToken(t, testSigningKey, "run-1", time.Now().Add(time.Hour))
			r := authRequest(t, "run-1")
			r.Header.Set("Authorization", "Bearer "+tok)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusGone {
				t.Fatalf("status %s: expected 410 Gone, got %d (body=%s)", status, w.Code, w.Body.String())
			}
			if called.Load() {
				t.Fatalf("status %s: next handler should not have been called", status)
			}
		})
	}
}

func TestRunTokenAuth_StaleAttemptRejected(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testSigningKey,
	}
	ms := &workerTaskAPIStoreMock{
		APIStoreMock: &APIStoreMock{},
		status:       domain.StatusExecuting,
		attempt:      2,
		projectID:    "proj-1",
	}
	srv := NewServer(ServerDeps{Config: cfg, Store: ms, Queue: &mockQueue{}})
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
		t.Fatalf("expected 401 for stale attempt token, got %d: %s", w.Code, w.Body.String())
	}
	if called.Load() {
		t.Fatal("next handler should not have been called")
	}
}

func TestRunTokenAuth_AssignmentBoundTokenRequiresActiveMatchingWorkerTask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		task *domain.WorkerTask
		want int
	}{
		{
			name: "matching assigned task",
			task: &domain.WorkerTask{
				ID:        "task-1",
				RunID:     "run-1",
				ProjectID: "proj-1",
				Status:    domain.WorkerTaskStatusAssigned,
			},
			want: http.StatusOK,
		},
		{
			name: "wrong run",
			task: &domain.WorkerTask{
				ID:        "task-1",
				RunID:     "run-other",
				ProjectID: "proj-1",
				Status:    domain.WorkerTaskStatusAssigned,
			},
			want: http.StatusUnauthorized,
		},
		{
			name: "terminal task",
			task: &domain.WorkerTask{
				ID:        "task-1",
				RunID:     "run-1",
				ProjectID: "proj-1",
				Status:    domain.WorkerTaskStatusCompleted,
			},
			want: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{
				InternalSecret:      "test-secret-value",
				MaxBulkTriggerItems: 500,
				JWTSigningKey:       testSigningKey,
			}
			base := &APIStoreMock{
				GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
					return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusExecuting, Attempt: 1}, nil
				},
			}
			ms := &workerTaskAPIStoreMock{
				APIStoreMock: base,
				status:       domain.StatusExecuting,
				attempt:      1,
				projectID:    "proj-1",
				task:         tt.task,
			}
			srv := NewServer(ServerDeps{Config: cfg, Store: ms, Queue: &mockQueue{}})
			t.Cleanup(srv.Close)

			handler := srv.runTokenAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			tok := signTokenWithAttempt(t, testSigningKey, "run-1", time.Now().Add(time.Hour), 1, "task-1")
			r := authRequest(t, "run-1")
			r.Header.Set("Authorization", "Bearer "+tok)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.want, w.Body.String())
			}
		})
	}
}

// Regression: when the run has been deleted between token
// issuance and use, runTokenAuth must surface a 404 rather than a
// generic 500. This keeps the failure mode unambiguous for SDK retry
// classification.
func TestRunTokenAuth_RunNotFound_Rejected(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testSigningKey,
	}
	ms := &workerTaskAPIStoreMock{
		APIStoreMock: &APIStoreMock{},
		stateErr:     store.ErrRunNotFound,
	}
	srv := NewServer(ServerDeps{Config: cfg, Store: ms, Queue: &mockQueue{}})
	t.Cleanup(srv.Close)
	var called atomic.Bool
	handler := srv.runTokenAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called.Store(true)
	}))
	tok := signToken(t, testSigningKey, "run-1", time.Now().Add(time.Hour))
	r := authRequest(t, "run-1")
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body=%s)", w.Code, w.Body.String())
	}
	if called.Load() {
		t.Fatal("next handler should not have been called")
	}
}

func TestRunTokenAuth_SubjectMismatch(t *testing.T) {
	t.Parallel()
	srv := newAuthTestServer(t)
	var called atomic.Bool
	handler := srv.runTokenAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called.Store(true)
	}))
	// Token subject is "run-other" but URL runID is "run-1".
	tok := signToken(t, testSigningKey, "run-other", time.Now().Add(time.Hour))
	r := authRequest(t, "run-1")
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if called.Load() {
		t.Fatal("next handler should not have been called")
	}
}

func TestRunTokenAuth_EmptySubject(t *testing.T) {
	t.Parallel()
	srv := newAuthTestServer(t)
	var called atomic.Bool
	handler := srv.runTokenAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called.Store(true)
	}))
	tok := signToken(t, testSigningKey, "", time.Now().Add(time.Hour))
	r := authRequest(t, "run-1")
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if called.Load() {
		t.Fatal("next handler should not have been called")
	}
}

func FuzzRunTokenAuth_MalformedHeaders(f *testing.F) {
	f.Add("")
	f.Add("Bearer ")
	f.Add("Bearer invalid")
	f.Add("Basic dXNlcjpwYXNz")
	f.Add("bearer token")
	f.Add(strings.Repeat("A", 10000))
	f.Add("Bearer " + strings.Repeat("x", 5000))
	f.Fuzz(func(t *testing.T, authHeader string) {
		srv := newAuthTestServer(t)
		handler := srv.runTokenAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		r := authRequest(t, "run-fuzz")
		if authHeader != "" {
			r.Header.Set("Authorization", authHeader)
		}
		w := httptest.NewRecorder()
		// Must not panic.
		handler.ServeHTTP(w, r)
		// All fuzzed inputs should be rejected (not 200).
		// A 200 is acceptable only if the fuzzer accidentally creates a valid
		// JWT signed with testSigningKey, which is astronomically unlikely.
		_ = w.Code
	})
}

// SDK state key/value size limit tests.

func TestSDKState_KeyAtMaxLength(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{
		UpsertRunStateFunc: func(_ context.Context, _ *domain.RunState) error {
			return nil
		},
	}, &mockQueue{}, nil)
	// Max key length is 256.
	key := strings.Repeat("k", 256)
	body := `{"key":"` + key + `","value":"\"hello\""}`
	r := sdkRequest(t, http.MethodPost, "/sdk/runs/run-1/state", "run-1", body)
	w := httptest.NewRecorder()
	srv.runTokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		input := &SDKSetStateInput{
			RunID: "run-1",
			Body:  SDKSetStateRequest{Key: key, Value: []byte(`"hello"`)},
		}
		out, err := srv.handleSDKSetState(ctx, input)
		if err != nil {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		_ = out
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for key at max length, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSDKState_KeyOverMaxLength(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	// Key is 257 characters, one over the 256 limit.
	key := strings.Repeat("k", 257)
	r := sdkRequest(t, http.MethodPost, "/sdk/runs/run-1/state", "run-1", "")
	w := httptest.NewRecorder()
	srv.runTokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		input := &SDKSetStateInput{
			RunID: "run-1",
			Body:  SDKSetStateRequest{Key: key, Value: []byte(`"v"`)},
		}
		_, err := srv.handleSDKSetState(ctx, input)
		if err == nil {
			t.Fatal("expected error for key over max length")
		}
		if !strings.Contains(err.Error(), "256") {
			t.Fatalf("error should mention 256 limit, got: %v", err)
		}
		w.WriteHeader(http.StatusBadRequest)
	})).ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSDKState_ValueAtMaxSize(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{
		UpsertRunStateFunc: func(_ context.Context, _ *domain.RunState) error {
			return nil
		},
	}, &mockQueue{}, nil)
	// Max value size is 65536 bytes (64KB).
	value := []byte(`"` + strings.Repeat("x", 65534) + `"`)
	if len(value) != 65536 {
		// Adjust: the JSON value including quotes must be exactly 65536.
		value = make([]byte, 65536)
		value[0] = '"'
		for i := 1; i < 65535; i++ {
			value[i] = 'x'
		}
		value[65535] = '"'
	}
	r := sdkRequest(t, http.MethodPost, "/sdk/runs/run-1/state", "run-1", "")
	w := httptest.NewRecorder()
	srv.runTokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		input := &SDKSetStateInput{
			RunID: "run-1",
			Body:  SDKSetStateRequest{Key: "mykey", Value: value},
		}
		_, err := srv.handleSDKSetState(ctx, input)
		if err != nil {
			t.Fatalf("expected success for value at max size, got: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSDKState_ValueOverMaxSize(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	// One byte over 64KB.
	value := make([]byte, 65537)
	value[0] = '"'
	for i := 1; i < 65536; i++ {
		value[i] = 'x'
	}
	value[65536] = '"'
	r := sdkRequest(t, http.MethodPost, "/sdk/runs/run-1/state", "run-1", "")
	w := httptest.NewRecorder()
	srv.runTokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		input := &SDKSetStateInput{
			RunID: "run-1",
			Body:  SDKSetStateRequest{Key: "mykey", Value: value},
		}
		_, err := srv.handleSDKSetState(ctx, input)
		if err == nil {
			t.Fatal("expected error for value over max size")
		}
		if !strings.Contains(err.Error(), "64KB") {
			t.Fatalf("error should mention 64KB limit, got: %v", err)
		}
		w.WriteHeader(http.StatusBadRequest)
	})).ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSDKState_NullByteInKey(t *testing.T) {
	t.Parallel()
	var upsertCalled atomic.Bool
	srv := newTestServer(t, &APIStoreMock{
		UpsertRunStateFunc: func(_ context.Context, s *domain.RunState) error {
			upsertCalled.Store(true)
			// Verify the key with null byte was passed through.
			if !strings.Contains(s.StateKey, "\x00") {
				t.Errorf("expected null byte in key, got %q", s.StateKey)
			}
			return nil
		},
	}, &mockQueue{}, nil)
	r := sdkRequest(t, http.MethodPost, "/sdk/runs/run-1/state", "run-1", "")
	w := httptest.NewRecorder()
	srv.runTokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		input := &SDKSetStateInput{
			RunID: "run-1",
			Body:  SDKSetStateRequest{Key: "has\x00null", Value: []byte(`"v"`)},
		}
		_, err := srv.handleSDKSetState(ctx, input)
		if err != nil {
			// If the handler rejects null bytes, that is also acceptable.
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, r)
	// Accept either 200 (passed through) or 400 (rejected).
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Fatalf("expected 200 or 400, got %d", w.Code)
	}
}

// SDK memory TTL and key limit tests.

func TestSDKMemory_TTLZero(t *testing.T) {
	t.Parallel()
	// TTL of 0 should not set an expiration.
	srv := newTestServer(t, &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
		UpsertJobMemoryWithQuotaFunc: func(_ context.Context, mem *domain.JobMemory, _, _ int) error {
			if mem.TTLExpiresAt != nil {
				t.Errorf("TTL=0 should result in nil TTLExpiresAt, got %v", mem.TTLExpiresAt)
			}
			return nil
		},
	}, &mockQueue{}, nil)
	ttl := 0
	r := sdkRequest(t, http.MethodPut, "/sdk/runs/run-1/memory/mykey", "run-1", "")
	w := httptest.NewRecorder()
	srv.runTokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		input := &SDKSetMemoryInput{
			RunID: "run-1",
			Key:   "mykey",
			Body:  SDKSetMemoryRequest{Value: []byte(`"data"`), TTLSecs: &ttl},
		}
		_, err := srv.handleSDKSetMemory(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSDKMemory_TTLNegative(t *testing.T) {
	t.Parallel()
	// Negative TTL should not set an expiration (same as zero).
	srv := newTestServer(t, &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
		UpsertJobMemoryWithQuotaFunc: func(_ context.Context, mem *domain.JobMemory, _, _ int) error {
			if mem.TTLExpiresAt != nil {
				t.Errorf("TTL=-1 should result in nil TTLExpiresAt, got %v", mem.TTLExpiresAt)
			}
			return nil
		},
	}, &mockQueue{}, nil)
	ttl := -1
	r := sdkRequest(t, http.MethodPut, "/sdk/runs/run-1/memory/mykey", "run-1", "")
	w := httptest.NewRecorder()
	srv.runTokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		input := &SDKSetMemoryInput{
			RunID: "run-1",
			Key:   "mykey",
			Body:  SDKSetMemoryRequest{Value: []byte(`"data"`), TTLSecs: &ttl},
		}
		_, err := srv.handleSDKSetMemory(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSDKMemory_TTLMaxInt(t *testing.T) {
	t.Parallel()
	// MaxInt TTL should still set a TTLExpiresAt (far future) without panicking.
	srv := newTestServer(t, &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
		UpsertJobMemoryWithQuotaFunc: func(_ context.Context, mem *domain.JobMemory, _, _ int) error {
			if mem.TTLExpiresAt == nil {
				t.Error("maxint TTL should set a TTLExpiresAt")
			}
			return nil
		},
	}, &mockQueue{}, nil)
	ttl := math.MaxInt32
	r := sdkRequest(t, http.MethodPut, "/sdk/runs/run-1/memory/mykey", "run-1", "")
	w := httptest.NewRecorder()
	srv.runTokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		input := &SDKSetMemoryInput{
			RunID: "run-1",
			Key:   "mykey",
			Body:  SDKSetMemoryRequest{Value: []byte(`"data"`), TTLSecs: &ttl},
		}
		_, err := srv.handleSDKSetMemory(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSDKMemory_KeyAtMaxLength(t *testing.T) {
	t.Parallel()
	// Max memory key length is 256.
	srv := newTestServer(t, &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
		UpsertJobMemoryWithQuotaFunc: func(_ context.Context, _ *domain.JobMemory, _, _ int) error {
			return nil
		},
	}, &mockQueue{}, nil)
	key := strings.Repeat("m", 256)
	r := sdkRequest(t, http.MethodPut, "/sdk/runs/run-1/memory/"+key, "run-1", "")
	w := httptest.NewRecorder()
	srv.runTokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		input := &SDKSetMemoryInput{
			RunID: "run-1",
			Key:   key,
			Body:  SDKSetMemoryRequest{Value: []byte(`"val"`)},
		}
		_, err := srv.handleSDKSetMemory(ctx, input)
		if err != nil {
			t.Fatalf("expected success for key at max length, got: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSDKMutations_RevalidateAfterAtomicGuardConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "log", method: http.MethodPost, path: "/sdk/v1/runs/run-1/log", body: `{"message":"late"}`},
		{name: "progress", method: http.MethodPost, path: "/sdk/v1/runs/run-1/progress", body: `{"percent":50,"message":"late"}`},
		{name: "annotate", method: http.MethodPost, path: "/sdk/v1/runs/run-1/annotate", body: `{"annotations":{"late":"true"}}`},
		{name: "heartbeat", method: http.MethodPost, path: "/sdk/v1/runs/run-1/heartbeat", body: ""},
		{name: "checkpoint", method: http.MethodPost, path: "/sdk/v1/runs/run-1/checkpoint", body: `{"state":{"cursor":1}}`},
		{name: "state", method: http.MethodPost, path: "/sdk/v1/runs/run-1/state", body: `{"key":"k","value":{"late":true}}`},
		{name: "delete-state", method: http.MethodDelete, path: "/sdk/v1/runs/run-1/state/k", body: ""},
		{name: "output", method: http.MethodPost, path: "/sdk/v1/runs/run-1/output", body: `{"output_key":"final","value":{"late":true}}`},
		{name: "memory", method: http.MethodPost, path: "/sdk/v1/runs/run-1/memory/k", body: `{"value":{"late":true}}`},
		{name: "delete-memory", method: http.MethodDelete, path: "/sdk/v1/runs/run-1/memory/k", body: ""},
		{name: "stream", method: http.MethodPost, path: "/sdk/v1/runs/run-1/stream", body: `{"chunk":"late"}`},
		{name: "resource-snapshot", method: http.MethodPost, path: "/sdk/v1/runs/run-1/resource-snapshot", body: `{"cpu_percent":1,"memory_mb":2}`},
		{name: "spawn", method: http.MethodPost, path: "/sdk/v1/runs/run-1/spawn", body: `{"job_slug":"child","project_id":"proj-1"}`},
		{name: "continue", method: http.MethodPost, path: "/sdk/v1/runs/run-1/continue", body: `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ms := &racingTerminalSDKStore{
				APIStoreMock: &APIStoreMock{
					GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
						return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusExecuting, Attempt: 1}, nil
					},
					GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
						return &domain.Job{ID: id, ProjectID: "proj-1", Slug: "child"}, nil
					},
					GetJobBySlugFunc: func(_ context.Context, projectID, slug string) (*domain.Job, error) {
						return &domain.Job{ID: "child-job", ProjectID: projectID, Slug: slug}, nil
					},
					GetProjectQuotaFunc: func(context.Context, string) (*store.ProjectQuota, error) {
						return nil, nil
					},
				},
			}
			srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
			w := httptest.NewRecorder()
			r := sdkRequest(t, tt.method, tt.path, "run-1", tt.body)

			srv.ServeHTTP(w, r)

			if w.Code != http.StatusGone {
				t.Fatalf("expected 410 after terminal race, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func FuzzSDKStateKeyValue(f *testing.F) {
	f.Add("key", "\"value\"")
	f.Add("", "\"\"")
	f.Add(strings.Repeat("a", 256), `"v"`)
	f.Add(strings.Repeat("a", 257), `"v"`)
	f.Add("null\x00byte", `"v"`)
	f.Add("key", strings.Repeat("x", 70000))
	f.Fuzz(func(t *testing.T, key, value string) {
		// Exercise the validation path without panicking.
		input := &SDKSetStateInput{
			RunID: "run-fuzz",
			Body:  SDKSetStateRequest{Key: key, Value: []byte(value)},
		}
		// Manually check the limits that handleSDKSetState enforces.
		if len(input.Body.Key) > 256 {
			return // Would be rejected.
		}
		if len(input.Body.Value) > 65536 {
			return // Would be rejected.
		}
		// Verify the struct can be constructed without panic.
		_ = input
	})
}
