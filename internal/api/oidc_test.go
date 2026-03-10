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

func TestOIDCAuth_AllowsValidToken(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal pub key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	claims := jwt.RegisteredClaims{
		Subject:   "user-oidc-1",
		Issuer:    "https://issuer.example",
		Audience:  []string{"strait-api"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	ms := &mockAPIStore{}
	ms.queueStatsFn = func(ctx context.Context) (*store.QueueStats, error) {
		if actor := actorFromContext(ctx); actor != "user-oidc-1" {
			t.Fatalf("actor = %q, want user-oidc-1", actor)
		}
		if projectID := projectIDFromContext(ctx); projectID != "proj-oidc" {
			t.Fatalf("project_id = %q, want proj-oidc", projectID)
		}
		return &store.QueueStats{}, nil
	}
	ms.getUserPermissionsFn = func(_ context.Context, projectID, userID string) ([]string, error) {
		if projectID != "proj-oidc" || userID != "user-oidc-1" {
			t.Fatalf("permission lookup args = (%s,%s)", projectID, userID)
		}
		return []string{"stats:read"}, nil
	}

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret",
			JWTSigningKey:    "01234567890123456789012345678901",
			OIDCEnabled:      true,
			OIDCIssuer:       "https://issuer.example",
			OIDCAudience:     "strait-api",
			OIDCPublicKeyPEM: string(pubPEM),
		},
		Store: ms,
	})

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer "+signed)
	r.Header.Set("X-Project-Id", "proj-oidc")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}
