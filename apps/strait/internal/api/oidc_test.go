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
)

func mustOIDCKeyPair(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal pub key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	return key, pubPEM
}

func mustOIDCPublicKeyPEM(t *testing.T, bits int) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal pub key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
}

func mustSignOIDCToken(t *testing.T, key *rsa.PrivateKey, claims jwt.RegisteredClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
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
	if err == nil {
		t.Fatal("expected weak RSA key to be rejected")
	}
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
		if actor := actorFromContext(ctx); actor != "user-oidc-1" {
			t.Fatalf("actor = %q, want user-oidc-1", actor)
		}
		if projectID := projectIDFromContext(ctx); projectID != "proj-oidc" {
			t.Fatalf("project_id = %q, want proj-oidc", projectID)
		}
		return &store.QueueStats{}, nil
	}
	ms.GetUserPermissionsFunc = func(_ context.Context, projectID, userID string) ([]string, error) {
		if projectID != "proj-oidc" || userID != "user-oidc-1" {
			t.Fatalf("permission lookup args = (%s,%s)", projectID, userID)
		}
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

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
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

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
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

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
