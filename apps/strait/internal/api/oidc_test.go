package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/store"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func mustOIDCKeyPair(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)

	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	return key, pubPEM
}

func mustOIDCPublicKeyPEM(t *testing.T, bits int) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	require.NoError(t, err)

	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
}

func mustSignOIDCToken(t *testing.T, key *rsa.PrivateKey, claims jwt.RegisteredClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(key)
	require.NoError(t, err)

	return signed
}

func TestNewOIDCVerifier_RejectsWeakRSAKey(t *testing.T) {
	t.Parallel()

	_, err := newOIDCVerifier(&config.Config{
		OIDCEnabled:      true,
		OIDCIssuer:       "https://issuer.example",
		OIDCAudience:     "strait-api",
		OIDCPublicKeyPEM: string(mustOIDCPublicKeyPEM(t, 1024)),
	})
	require.Error(t, err)

}

func TestOIDCAuth_AllowsValidToken(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-oidc-1",
		Issuer:    "https://issuer.example",
		Audience:  []string{"strait-api"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	ms := &APIStoreMock{}
	ms.UserHasProjectAccessFunc = func(_ context.Context, userID, projectID string) (bool, error) {
		if userID == "user-oidc-1" && projectID == "proj-oidc" {
			return true, nil
		}
		return false, nil
	}
	ms.QueueStatsFunc = func(ctx context.Context) (*store.QueueStats, error) {
		require.Equal(t, "user-oidc-1",
			actorFromContext(ctx))
		require.Equal(t, "proj-oidc",
			projectIDFromContext(ctx))

		return &store.QueueStats{}, nil
	}
	ms.GetUserPermissionsFunc = func(_ context.Context, projectID, userID string) ([]string, error) {
		require.False(t, projectID !=
			"proj-oidc" ||
			userID != "user-oidc-1",
		)

		return []string{"stats:read"}, nil
	}

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			OIDCEnabled:      true,
			OIDCIssuer:       "https://issuer.example",
			OIDCAudience:     "strait-api",
			OIDCPublicKeyPEM: string(pubPEM),
		},
		Store: ms,
	})
	t.Cleanup(srv.Close)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer "+signed)
	r.Header.Set("X-Project-Id", "proj-oidc")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

}

func TestOIDCAuth_RejectsExpiredToken(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-oidc-2",
		Issuer:    "https://issuer.example",
		Audience:  []string{"strait-api"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
	})

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			OIDCEnabled:      true,
			OIDCIssuer:       "https://issuer.example",
			OIDCAudience:     "strait-api",
			OIDCPublicKeyPEM: string(pubPEM),
		},
		Store: &APIStoreMock{},
	})
	t.Cleanup(srv.Close)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer "+signed)
	r.Header.Set("X-Project-Id", "proj-oidc")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)

}

func TestOIDCAuth_RejectsWrongAudience(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-oidc-3",
		Issuer:    "https://issuer.example",
		Audience:  []string{"other-audience"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			OIDCEnabled:      true,
			OIDCIssuer:       "https://issuer.example",
			OIDCAudience:     "strait-api",
			OIDCPublicKeyPEM: string(pubPEM),
		},
		Store: &APIStoreMock{},
	})
	t.Cleanup(srv.Close)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer "+signed)
	r.Header.Set("X-Project-Id", "proj-oidc")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)

}
