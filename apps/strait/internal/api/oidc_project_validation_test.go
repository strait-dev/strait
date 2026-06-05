package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/store"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oidcProjectTestServer creates a Server configured for OIDC project validation tests.
// The caller must set UserHasProjectAccessFunc on the returned mock before making requests.
func oidcProjectTestServer(t *testing.T, pubPEM []byte, ms *APIStoreMock) *Server {
	t.Helper()
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			OIDCEnabled:      true,
			OIDCIssuer:       "https://issuer.test",
			OIDCAudience:     "aud-test",
			OIDCPublicKeyPEM: string(pubPEM),
		},
		Store: ms,
	})
	t.Cleanup(srv.Close)
	return srv
}

// TestOIDC_ProjectHeaderValidUser verifies that a user with project access
// receives a 200 response when passing the X-Project-Id header.
func TestOIDC_ProjectHeaderValidUser(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-valid",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	ms := &APIStoreMock{}
	ms.UserHasProjectAccessFunc = func(_ context.Context, userID, projectID string) (bool, error) {
		if userID == "user-valid" && projectID == "proj-valid" {
			return true, nil
		}
		return false, nil
	}
	ms.QueueStatsFunc = func(_ context.Context) (*store.QueueStats, error) {
		return &store.QueueStats{}, nil
	}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{"stats:read"}, nil
	}

	srv := oidcProjectTestServer(t, pubPEM, ms)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer "+signed)
	r.Header.Set("X-Project-Id", "proj-valid")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
}

// TestOIDC_ProjectHeaderNoAccess verifies that a user without project membership
// receives a 403 response.
func TestOIDC_ProjectHeaderNoAccess(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-noaccess",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	ms := &APIStoreMock{}
	ms.UserHasProjectAccessFunc = func(_ context.Context, _, _ string) (bool, error) {
		return false, nil
	}

	srv := oidcProjectTestServer(t, pubPEM, ms)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer "+signed)
	r.Header.Set("X-Project-Id", "proj-forbidden")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)
}

// TestOIDC_ProjectHeaderEmpty verifies that omitting the X-Project-Id header
// skips the project access check and proceeds normally.
func TestOIDC_ProjectHeaderEmpty(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-noheader",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	ms := &APIStoreMock{}
	// UserHasProjectAccessFunc should not be called when no header is set.
	ms.UserHasProjectAccessFunc = func(_ context.Context, _, _ string) (bool, error) {
		assert.Fail(t,

			"UserHasProjectAccess should not be called when X-Project-Id is empty")
		return false, nil
	}
	ms.QueueStatsFunc = func(_ context.Context) (*store.QueueStats, error) {
		return &store.QueueStats{}, nil
	}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return nil, nil
	}

	srv := oidcProjectTestServer(t, pubPEM, ms)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer "+signed)
	// No X-Project-Id header.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.NotEqual(t, http.StatusUnauthorized,

		w.Code)

	// The request proceeds (stats endpoint may return 200 or 403 depending on
	// permissions, but must not be 401 from missing token).
}

// TestOIDC_ProjectHeaderStoreError verifies that a store error during the access
// check results in a 403 (fail closed).
func TestOIDC_ProjectHeaderStoreError(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-storeerr",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	ms := &APIStoreMock{}
	ms.UserHasProjectAccessFunc = func(_ context.Context, _, _ string) (bool, error) {
		return false, errors.New("connection timeout")
	}

	srv := oidcProjectTestServer(t, pubPEM, ms)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer "+signed)
	r.Header.Set("X-Project-Id", "proj-err")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)
}

// TestOIDC_ProjectHeaderNullBytes verifies that null bytes in the X-Project-Id
// header result in a 403 (the store should not find a matching project).
func TestOIDC_ProjectHeaderNullBytes(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-null",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	ms := &APIStoreMock{}
	ms.UserHasProjectAccessFunc = func(_ context.Context, _, _ string) (bool, error) {
		return false, nil
	}

	srv := oidcProjectTestServer(t, pubPEM, ms)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer "+signed)
	r.Header.Set("X-Project-Id", "proj-\x00injected")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)
}

// TestOIDC_ProjectHeaderSQLInjection verifies that SQL injection attempts in the
// X-Project-Id header are handled safely (the parameterized query prevents injection).
func TestOIDC_ProjectHeaderSQLInjection(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-sqli",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	ms := &APIStoreMock{}
	ms.UserHasProjectAccessFunc = func(_ context.Context, _, _ string) (bool, error) {
		// Mock always returns false; the SQL injection string is treated as a literal value.
		return false, nil
	}

	srv := oidcProjectTestServer(t, pubPEM, ms)

	payloads := []string{
		"'; DROP TABLE projects; --",
		"' OR '1'='1",
		"proj-id' UNION SELECT 1--",
		"\" OR \"\"=\"",
	}

	for _, payload := range payloads {
		r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
		r.Header.Set("Authorization", "Bearer "+signed)
		r.Header.Set("X-Project-Id", payload)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		assert.Equal(
			t, http.StatusForbidden,
			w.Code,
		)
	}
}

// FuzzOIDCProjectHeader fuzzes the X-Project-Id header value to ensure the
// middleware never panics regardless of input.
func FuzzOIDCProjectHeader(f *testing.F) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		f.Fatalf("generate rsa key: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		f.Fatalf("marshal pub key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		Subject:   "user-fuzz",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
	})
	signed, err := token.SignedString(key)
	if err != nil {
		f.Fatalf("sign token: %v", err)
	}

	ms := &APIStoreMock{}
	ms.UserHasProjectAccessFunc = func(_ context.Context, _, _ string) (bool, error) {
		return false, nil
	}
	ms.QueueStatsFunc = func(_ context.Context) (*store.QueueStats, error) {
		return &store.QueueStats{}, nil
	}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return nil, nil
	}

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			OIDCEnabled:      true,
			OIDCIssuer:       "https://issuer.test",
			OIDCAudience:     "aud-test",
			OIDCPublicKeyPEM: string(pubPEM),
		},
		Store: ms,
	})
	f.Cleanup(srv.Close)

	f.Add("")
	f.Add("proj-123")
	f.Add("'; DROP TABLE projects; --")
	f.Add(strings.Repeat("A", 10000))
	f.Add("proj-\x00injected")

	f.Fuzz(func(t *testing.T, projectID string) {
		r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
		r.Header.Set("Authorization", "Bearer "+signed)
		if projectID != "" {
			r.Header.Set("X-Project-Id", projectID)
		}
		w := httptest.NewRecorder()
		// Must not panic.
		srv.ServeHTTP(w, r)
	})
}
