package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"

	"github.com/golang-jwt/jwt/v5"
)

func TestHandleCreateSSEToken_Success(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeRunsRead)(
		TypedHandler(srv, http.StatusCreated, srv.handleCreateSSEToken),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/sse-token", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeRunsRead, domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("empty token")
	}
	if resp.ExpiresAt.Before(time.Now()) {
		t.Error("expires_at is in the past")
	}
	if resp.ExpiresAt.After(time.Now().Add(6 * time.Minute)) {
		t.Error("expires_at exceeds 5-minute TTL")
	}

	claims := srv.parseSSEToken(resp.Token)
	if claims == nil {
		t.Fatal("created token did not parse")
		return
	}
	if claims.ProjectID != "proj-1" {
		t.Fatalf("project_id = %q, want proj-1", claims.ProjectID)
	}
}

func TestHandleCreateSSEToken_PreservesEnvironmentScope(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeRunsRead)(
		TypedHandler(srv, http.StatusCreated, srv.handleCreateSSEToken),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/sse-token", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeRunsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	claims := srv.parseSSEToken(resp.Token)
	if claims == nil {
		t.Fatal("created token did not parse")
		return
	}
	if claims.EnvironmentID != "env-prod" {
		t.Fatalf("environment_id = %q, want env-prod", claims.EnvironmentID)
	}
}

func TestHandleCreateSSEToken_UserRBACPermissionsMintUsableToken(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetUserPermissionsFunc: func(_ context.Context, projectID, actorID string) ([]string, error) {
			if projectID != "proj-1" || actorID != "user-1" {
				t.Fatalf("unexpected permission lookup: project=%q actor=%q", projectID, actorID)
			}
			return []string{domain.ScopeRunsRead}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeRunsRead)(
		TypedHandler(srv, http.StatusCreated, srv.handleCreateSSEToken),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/sse-token", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	claims := srv.parseSSEToken(resp.Token)
	if claims == nil {
		t.Fatal("created token did not parse")
		return
	}
	if !domain.HasScopeStrict(claims.Scopes, domain.ScopeRunsRead) {
		t.Fatalf("minted token scopes %v do not include %s", claims.Scopes, domain.ScopeRunsRead)
	}

	tokenCtx := context.WithValue(context.Background(), ctxProjectIDKey, claims.ProjectID)
	tokenCtx = context.WithValue(tokenCtx, ctxActorTypeKey, "sse_token")
	tokenCtx = context.WithValue(tokenCtx, ctxActorIDKey, "sse:proj-1")
	tokenCtx = context.WithValue(tokenCtx, ctxScopesKey, claims.Scopes)
	tokenReq := httptest.NewRequest(http.MethodGet, "/v1/runs/run-1/stream", nil).WithContext(tokenCtx)
	tokenW := httptest.NewRecorder()
	srv.requirePermission(domain.ScopeRunsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(tokenW, tokenReq)

	if tokenW.Code != http.StatusNoContent {
		t.Fatalf("minted token should be usable for runs:read, got %d: %s", tokenW.Code, tokenW.Body.String())
	}
}

func TestHandleCreateSSEToken_UserScopesRespectExplicitOIDCUpperBound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetUserPermissionsFunc: func(_ context.Context, _, _ string) ([]string, error) {
			return []string{domain.ScopeAll}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeRunsRead)(
		TypedHandler(srv, http.StatusCreated, srv.handleCreateSSEToken),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/sse-token", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeRunsRead})
	ctx = context.WithValue(ctx, ctxOIDCScopeClaimPresentKey, true)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	claims := srv.parseSSEToken(resp.Token)
	if claims == nil {
		t.Fatal("created token did not parse")
		return
	}
	if len(claims.Scopes) != 1 || claims.Scopes[0] != domain.ScopeRunsRead {
		t.Fatalf("scopes = %v, want only [%s]", claims.Scopes, domain.ScopeRunsRead)
	}
}

func TestParseSSEToken_Valid(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret: "test-secret-value",
			JWTSigningKey:  testJWTSigningKey,
		},
		Store:  &APIStoreMock{},
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)

	claims := SSETokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sseTokenIssuer,
			Subject:   "proj-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		ProjectID: "proj-1",
		Scopes:    []string{domain.ScopeRunsRead},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testJWTSigningKey))
	if err != nil {
		t.Fatalf("sign error: %v", err)
	}

	parsed := srv.parseSSEToken(signed)
	if parsed == nil {
		t.Fatal("expected valid claims, got nil")
		return
	}
	if parsed.ProjectID != "proj-1" {
		t.Errorf("project_id = %q, want %q", parsed.ProjectID, "proj-1")
	}
	if len(parsed.Scopes) != 1 || parsed.Scopes[0] != domain.ScopeRunsRead {
		t.Errorf("scopes = %v, want [runs:read]", parsed.Scopes)
	}
}

func TestParseSSEToken_Expired(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret: "test-secret-value",
			JWTSigningKey:  testJWTSigningKey,
		},
		Store:  &APIStoreMock{},
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)

	claims := SSETokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sseTokenIssuer,
			Subject:   "proj-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-6 * time.Minute)),
		},
		ProjectID: "proj-1",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte(testJWTSigningKey))

	parsed := srv.parseSSEToken(signed)
	if parsed != nil {
		t.Error("expected nil for expired token")
	}
}

func TestParseSSEToken_WrongIssuer(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret: "test-secret-value",
			JWTSigningKey:  testJWTSigningKey,
		},
		Store:  &APIStoreMock{},
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)

	claims := SSETokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "wrong-issuer",
			Subject:   "proj-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		ProjectID: "proj-1",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte(testJWTSigningKey))

	parsed := srv.parseSSEToken(signed)
	if parsed != nil {
		t.Error("expected nil for wrong issuer")
	}
}

func TestParseSSEToken_WrongKey(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret: "test-secret-value",
			JWTSigningKey:  testJWTSigningKey,
		},
		Store:  &APIStoreMock{},
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)

	claims := SSETokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sseTokenIssuer,
			Subject:   "proj-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
		ProjectID: "proj-1",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte("wrong-key-that-is-32-chars-long!"))

	parsed := srv.parseSSEToken(signed)
	if parsed != nil {
		t.Error("expected nil for wrong signing key")
	}
}

func TestParseSSEToken_GarbageInput(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret: "test-secret-value",
			JWTSigningKey:  testJWTSigningKey,
		},
		Store:  &APIStoreMock{},
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)

	inputs := []string{"", "not-a-jwt", "a.b.c", "strait_realkey123", strings.Repeat("x", 1000)}
	for _, input := range inputs {
		if srv.parseSSEToken(input) != nil {
			t.Errorf("expected nil for garbage input %q", input[:min(len(input), 20)])
		}
	}
}

func TestSSETokenAuth_ShortLivedJWT_BypassesAPIKeyAuth(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret: "test-secret-value",
			JWTSigningKey:  testJWTSigningKey,
		},
		Store:   &APIStoreMock{},
		Queue:   &mockQueue{},
		PubSub:  &mockPublisher{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	// Create a valid SSE token.
	claims := SSETokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sseTokenIssuer,
			Subject:   "proj-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		ProjectID: "proj-1",
		Scopes:    []string{domain.ScopeJobsRead},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte(testJWTSigningKey))

	// Use the SSE token in the query param for an SSE endpoint.
	// This should authenticate without needing an API key.
	req := httptest.NewRequest(http.MethodGet, "/v1/events/test-key/stream?token="+signed, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// Should not be 401 (unauthenticated) -- the SSE token should have authenticated.
	if w.Code == http.StatusUnauthorized {
		t.Errorf("SSE token should bypass API key auth, got 401; body: %s", w.Body.String())
	}
}

func TestSSETokenAuth_RestoresEnvironmentScope(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret: "test-secret-value",
			JWTSigningKey:  testJWTSigningKey,
		},
		Store:  &APIStoreMock{},
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)

	claims := SSETokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sseTokenIssuer,
			Subject:   "proj-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		ProjectID:     "proj-1",
		EnvironmentID: "env-prod",
		Scopes:        []string{domain.ScopeJobsRead},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testJWTSigningKey))
	if err != nil {
		t.Fatalf("sign error: %v", err)
	}

	handler := srv.sseTokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := projectIDFromContext(r.Context()); got != "proj-1" {
			t.Fatalf("project_id = %q, want proj-1", got)
		}
		if got := environmentIDFromContext(r.Context()); got != "env-prod" {
			t.Fatalf("environment_id = %q, want env-prod", got)
		}
		if got := scopesFromContext(r.Context()); len(got) != 1 || got[0] != domain.ScopeJobsRead {
			t.Fatalf("scopes = %v, want [%s]", got, domain.ScopeJobsRead)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/events/test-key/stream?token="+signed, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
}

func TestSSETokenAuth_RawAPIKeyQueryParamRejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(context.Context, string) (*domain.APIKey, error) {
			t.Fatal("raw API key query token must not be promoted into Authorization")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/test-key/stream?token=strait_someapikey", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("raw API key query token: status = %d, want 401", w.Code)
	}
	if len(ms.GetAPIKeyByHashCalls()) != 0 {
		t.Fatal("raw API key query token reached API key lookup")
	}
}

func TestRequirePermission_SSETokenEmptyScopesRejected(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/events/key/stream", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "sse_token")
	ctx = context.WithValue(ctx, ctxActorIDKey, "sse:proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_SSETokenNilScopesRejected(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/events/key/stream", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "sse_token")
	ctx = context.WithValue(ctx, ctxActorIDKey, "sse:proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_SSETokenExplicitScopeAllowed(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/events/key/stream", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "sse_token")
	ctx = context.WithValue(ctx, ctxActorIDKey, "sse:proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsRead})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func FuzzParseSSEToken(f *testing.F) {
	f.Add("valid-looking.jwt.token")
	f.Add("")
	f.Add("strait_rawkey")
	f.Add(strings.Repeat("a", 10000))
	f.Add("eyJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJ3cm9uZyJ9.signature")

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret: "test-secret-value",
			JWTSigningKey:  testJWTSigningKey,
		},
		Store:  &APIStoreMock{},
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})

	f.Fuzz(func(t *testing.T, token string) {
		// Should never panic.
		srv.parseSSEToken(token)
	})
}
