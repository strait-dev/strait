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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	require.NoError(t, json.NewDecoder(w.Body).
		Decode(&resp))
	require.NotEqual(t, "", resp.Token)
	assert.False(
		t, resp.ExpiresAt.
			Before(time.
				Now()))
	assert.False(
		t, resp.ExpiresAt.
			After(time.
				Now().Add(6*time.Minute)))

	claims := srv.parseSSEToken(resp.Token)
	require.NotNil(t, claims)
	require.Equal(t, "proj-1", claims.
		ProjectID,
	)

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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(w.Body).
		Decode(&resp))

	claims := srv.parseSSEToken(resp.Token)
	require.NotNil(t, claims)
	require.Equal(t, "env-prod", claims.
		EnvironmentID,
	)

}

func TestHandleCreateSSEToken_UserRBACPermissionsMintUsableToken(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetUserPermissionsFunc: func(_ context.Context, projectID, actorID string) ([]string, error) {
			require.False(t, projectID !=
				"proj-1" ||
				actorID != "user-1")

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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(w.Body).
		Decode(&resp))

	claims := srv.parseSSEToken(resp.Token)
	require.NotNil(t, claims)
	require.True(
		t, domain.HasScopeStrict(claims.
			Scopes, domain.ScopeRunsRead,
		),
	)

	tokenCtx := context.WithValue(context.Background(), ctxProjectIDKey, claims.ProjectID)
	tokenCtx = context.WithValue(tokenCtx, ctxActorTypeKey, "sse_token")
	tokenCtx = context.WithValue(tokenCtx, ctxActorIDKey, "sse:proj-1")
	tokenCtx = context.WithValue(tokenCtx, ctxScopesKey, claims.Scopes)
	tokenReq := httptest.NewRequest(http.MethodGet, "/v1/runs/run-1/stream", nil).WithContext(tokenCtx)
	tokenW := httptest.NewRecorder()
	srv.requirePermission(domain.ScopeRunsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(tokenW, tokenReq)
	require.Equal(t, http.StatusNoContent,
		tokenW.
			Code)

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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(w.Body).
		Decode(&resp))

	claims := srv.parseSSEToken(resp.Token)
	require.NotNil(t, claims)
	require.False(t, len(claims.Scopes) != 1 ||
		claims.Scopes[0] !=
			domain.ScopeRunsRead,
	)

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
	require.NoError(t, err)

	parsed := srv.parseSSEToken(signed)
	require.NotNil(t, parsed)
	assert.Equal(
		t, "proj-1", parsed.
			ProjectID,
	)
	assert.False(
		t, len(parsed.Scopes) != 1 ||
			parsed.Scopes[0] !=
				domain.ScopeRunsRead,
	)

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
	assert.Nil(t, parsed)

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
	assert.Nil(t, parsed)

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
	assert.Nil(t, parsed)

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
		assert.Nil(t, srv.parseSSEToken(input))

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
	assert.NotEqual(t, http.StatusUnauthorized,

		w.Code)

	// Should not be 401 (unauthenticated) -- the SSE token should have authenticated.

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
	require.NoError(t, err)

	handler := srv.sseTokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "proj-1", projectIDFromContext(r.Context()))
		require.Equal(t, "env-prod", environmentIDFromContext(r.Context()))

		if got := scopesFromContext(r.Context()); len(got) != 1 || got[0] != domain.ScopeJobsRead {
			require.Failf(t, "test failure",

				"scopes = %v, want [%s]", got, domain.ScopeJobsRead)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/events/test-key/stream?token="+signed, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent,
		w.Code,
	)

}

func TestSSETokenAuth_RawAPIKeyQueryParamRejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(context.Context, string) (*domain.APIKey, error) {
			require.Fail(t,

				"raw API key query token must not be promoted into Authorization")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/test-key/stream?token=strait_someapikey", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(
		t, http.StatusUnauthorized,
		w.
			Code)
	require.Len(t,
		ms.GetAPIKeyByHashCalls(),
		0)

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
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)

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
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)

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
	require.Equal(t, http.StatusOK,
		w.Code)

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
