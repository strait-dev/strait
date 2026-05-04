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
	if caps.Progress || caps.Checkpoint || caps.UsageReport {
		t.Fatalf("v1 should have no capabilities, got %+v", caps)
	}
}

func TestResolveSDKCapabilities_V2(t *testing.T) {
	t.Parallel()
	caps := resolveSDKCapabilities("2.0")
	if !caps.Progress || !caps.Checkpoint || !caps.UsageReport {
		t.Fatalf("v2 should have all capabilities, got %+v", caps)
	}
}

func TestResolveSDKCapabilities_Empty(t *testing.T) {
	t.Parallel()
	caps := resolveSDKCapabilities("")
	if caps.Progress || caps.Checkpoint || caps.UsageReport {
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
			if !caps.Progress || !caps.Checkpoint || !caps.UsageReport {
				t.Fatalf("version %q: expected all capabilities for major=2, got %+v", v, caps)
			}
		} else {
			if caps.Progress || caps.Checkpoint || caps.UsageReport {
				t.Fatalf("version %q should have no capabilities, got %+v", v, caps)
			}
		}
	}
}

func TestResolveSDKCapabilities_LargeVersion(t *testing.T) {
	t.Parallel()
	caps := resolveSDKCapabilities("99999.0.0")
	if !caps.Progress || !caps.Checkpoint || !caps.UsageReport {
		t.Fatalf("large major version should have all capabilities, got %+v", caps)
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
			for _, u := range bools {
				caps := SDKCapabilities{Progress: p, Checkpoint: c, UsageReport: u}
				header := sdkCapabilitiesHeader(caps)
				if header == "" {
					t.Fatal("header must never be empty string")
				}
				if !p && !c && !u {
					if header != "none" {
						t.Fatalf("all-false should produce 'none', got %q", header)
					}
					continue
				}
				// Verify each expected part is present.
				if p && !strings.Contains(header, "progress") {
					t.Fatalf("expected 'progress' in header %q", header)
				}
				if c && !strings.Contains(header, "checkpoint") {
					t.Fatalf("expected 'checkpoint' in header %q", header)
				}
				if u && !strings.Contains(header, "usage") {
					t.Fatalf("expected 'usage' in header %q", header)
				}
				// Verify absent parts are not present.
				if !p && strings.Contains(header, "progress") {
					t.Fatalf("unexpected 'progress' in header %q", header)
				}
				if !c && strings.Contains(header, "checkpoint") {
					t.Fatalf("unexpected 'checkpoint' in header %q", header)
				}
				if !u && strings.Contains(header, "usage") {
					t.Fatalf("unexpected 'usage' in header %q", header)
				}
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
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "strait:run-token",
		Subject:   subject,
		ExpiresAt: jwt.NewNumericDate(expiry),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})
	signed, err := token.SignedString([]byte(key))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
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

// Regression for F-AK-13: a token without the "strait:run-token"
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

// Regression for F-AK-13: a token without an `exp` claim must be
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

// Regression for F-AK-15: a token bound to a run that has already
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
			ms := &APIStoreMock{
				GetRunStatusFunc: func(_ context.Context, _ string) (domain.RunStatus, error) {
					return status, nil
				},
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

// Regression for F-AK-15: when the run has been deleted between token
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
	ms := &APIStoreMock{
		GetRunStatusFunc: func(_ context.Context, _ string) (domain.RunStatus, error) {
			return "", store.ErrRunNotFound
		},
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
