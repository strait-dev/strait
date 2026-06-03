package api

import (
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TestOIDCVerifierWithOAuthProviderToken verifies that tokens signed by
// Better Auth's oauth-provider (RS256 with our RSA private key) pass the
// Go OIDC verifier. This simulates the full OAuth client flow:
//
//	Better Auth signs JWT with OIDC_PRIVATE_KEY_PEM -> Go verifies with OIDC_PUBLIC_KEY_PEM
func TestOIDCVerifierWithOAuthProviderToken(t *testing.T) {
	// Generate a fresh RSA key pair for the test (same as the real flow,
	// just not reading from env to keep the test self-contained).
	privateKey, err := rsa.GenerateKey(nil, 2048)
	if err != nil {
		// Use crypto/rand properly
		t.Fatalf("generate key: %v", err)
	}

	issuer := "http://localhost:5173/api/auth"
	audience := "http://localhost:8080"

	v := &oidcVerifier{
		enabled:   true,
		issuer:    issuer,
		audience:  audience,
		publicKey: &privateKey.PublicKey,
	}

	t.Run("valid token with all claims", func(t *testing.T) {
		token := signTestToken(t, privateKey, jwt.MapClaims{
			"iss":   issuer,
			"aud":   audience,
			"sub":   "user-123",
			"email": "user@example.com",
			"name":  "Test User",
			"iat":   time.Now().Unix(),
			"exp":   time.Now().Add(time.Hour).Unix(),
		})

		claims, err := v.verify(token)
		if err != nil {
			t.Fatalf("verify: %v", err)
		}
		if claims.Subject != "user-123" {
			t.Errorf("subject = %q, want %q", claims.Subject, "user-123")
		}
		if claims.Email != "user@example.com" {
			t.Errorf("email = %q, want %q", claims.Email, "user@example.com")
		}
		if claims.Name != "Test User" {
			t.Errorf("name = %q, want %q", claims.Name, "Test User")
		}
	})

	t.Run("valid token with minimal claims (sub only)", func(t *testing.T) {
		token := signTestToken(t, privateKey, jwt.MapClaims{
			"iss": issuer,
			"aud": audience,
			"sub": "user-456",
			"iat": time.Now().Unix(),
			"exp": time.Now().Add(time.Hour).Unix(),
		})

		claims, err := v.verify(token)
		if err != nil {
			t.Fatalf("verify: %v", err)
		}
		if claims.Subject != "user-456" {
			t.Errorf("subject = %q, want %q", claims.Subject, "user-456")
		}
	})

	t.Run("wrong issuer rejected", func(t *testing.T) {
		token := signTestToken(t, privateKey, jwt.MapClaims{
			"iss": "https://evil.com",
			"aud": audience,
			"sub": "user-123",
			"iat": time.Now().Unix(),
			"exp": time.Now().Add(time.Hour).Unix(),
		})

		_, err := v.verify(token)
		if err == nil {
			t.Fatal("expected error for wrong issuer")
		}
	})

	t.Run("wrong audience rejected", func(t *testing.T) {
		token := signTestToken(t, privateKey, jwt.MapClaims{
			"iss": issuer,
			"aud": "https://wrong-audience.com",
			"sub": "user-123",
			"iat": time.Now().Unix(),
			"exp": time.Now().Add(time.Hour).Unix(),
		})

		_, err := v.verify(token)
		if err == nil {
			t.Fatal("expected error for wrong audience")
		}
	})

	t.Run("expired token rejected", func(t *testing.T) {
		token := signTestToken(t, privateKey, jwt.MapClaims{
			"iss": issuer,
			"aud": audience,
			"sub": "user-123",
			"iat": time.Now().Add(-2 * time.Hour).Unix(),
			"exp": time.Now().Add(-1 * time.Hour).Unix(),
		})

		_, err := v.verify(token)
		if err == nil {
			t.Fatal("expected error for expired token")
		}
	})

	t.Run("missing subject rejected", func(t *testing.T) {
		token := signTestToken(t, privateKey, jwt.MapClaims{
			"iss": issuer,
			"aud": audience,
			"iat": time.Now().Unix(),
			"exp": time.Now().Add(time.Hour).Unix(),
		})

		_, err := v.verify(token)
		if err == nil {
			t.Fatal("expected error for missing subject")
		}
	})

	t.Run("wrong signing key rejected", func(t *testing.T) {
		// Sign with a different key
		wrongKey, err := rsa.GenerateKey(nil, 2048)
		if err != nil {
			t.Fatalf("generate wrong key: %v", err)
		}
		token := signTestToken(t, wrongKey, jwt.MapClaims{
			"iss": issuer,
			"aud": audience,
			"sub": "user-123",
			"iat": time.Now().Unix(),
			"exp": time.Now().Add(time.Hour).Unix(),
		})

		_, err = v.verify(token)
		if err == nil {
			t.Fatal("expected error for wrong signing key")
		}
	})

	t.Run("token within 30s leeway accepted", func(t *testing.T) {
		token := signTestToken(t, privateKey, jwt.MapClaims{
			"iss": issuer,
			"aud": audience,
			"sub": "user-123",
			"iat": time.Now().Unix(),
			"exp": time.Now().Add(-15 * time.Second).Unix(), // expired 15s ago, within 30s leeway
		})

		claims, err := v.verify(token)
		if err != nil {
			t.Fatalf("verify: %v (token within 30s leeway should be accepted)", err)
		}
		if claims.Subject != "user-123" {
			t.Errorf("subject = %q, want %q", claims.Subject, "user-123")
		}
	})
}

// TestOIDCScopeFiltering verifies that privileged scopes are stripped from
// OIDC tokens. Even if a JWT contains wildcard or admin scopes, the Go
// verifier must not honor them — these are reserved for internal API keys.
func TestOIDCScopeFiltering(t *testing.T) {
	t.Parallel()

	t.Run("wildcard scope stripped", func(t *testing.T) {
		c := &oidcClaims{Scope: "* jobs:read"}
		scopes := c.Scopes()
		for _, s := range scopes {
			if s == "*" {
				t.Fatal("wildcard scope must not pass through OIDC token filter")
			}
		}
		if len(scopes) != 1 || scopes[0] != "jobs:read" {
			t.Fatalf("scopes = %v, want [jobs:read]", scopes)
		}
	})

	t.Run("api-keys:manage stripped", func(t *testing.T) {
		c := &oidcClaims{Scope: "jobs:read api-keys:manage runs:read"}
		scopes := c.Scopes()
		for _, s := range scopes {
			if s == "api-keys:manage" {
				t.Fatal("api-keys:manage must not pass through OIDC token filter")
			}
		}
		if len(scopes) != 2 {
			t.Fatalf("scopes = %v, want [jobs:read runs:read]", scopes)
		}
	})

	t.Run("rbac:manage stripped", func(t *testing.T) {
		c := &oidcClaims{Scope: "rbac:manage stats:read"}
		scopes := c.Scopes()
		for _, s := range scopes {
			if s == "rbac:manage" {
				t.Fatal("rbac:manage must not pass through OIDC token filter")
			}
		}
		if len(scopes) != 1 || scopes[0] != "stats:read" {
			t.Fatalf("scopes = %v, want [stats:read]", scopes)
		}
	})

	t.Run("all privileged scopes stripped returns empty upper bound", func(t *testing.T) {
		c := &oidcClaims{Scope: "* api-keys:manage rbac:manage"}
		scopes := c.Scopes()
		if scopes == nil {
			t.Fatal("scopes = nil, want explicit empty upper bound")
		}
		if len(scopes) != 0 {
			t.Fatalf("scopes = %v, want empty upper bound", scopes)
		}
	})

	t.Run("absent scope returns nil", func(t *testing.T) {
		c := &oidcClaims{}
		if scopes := c.Scopes(); scopes != nil {
			t.Fatalf("scopes = %v, want nil for absent claim", scopes)
		}
	})

	t.Run("normal scopes pass through", func(t *testing.T) {
		c := &oidcClaims{Scope: "jobs:read jobs:write runs:read workflows:trigger"}
		scopes := c.Scopes()
		if len(scopes) != 4 {
			t.Fatalf("scopes = %v, want 4 scopes", scopes)
		}
	})

	t.Run("OIDC scopes stripped", func(t *testing.T) {
		c := &oidcClaims{Scope: "openid profile email jobs:read"}
		scopes := c.Scopes()
		if len(scopes) != 1 || scopes[0] != "jobs:read" {
			t.Fatalf("scopes = %v, want [jobs:read] (OIDC scopes filtered out)", scopes)
		}
	})
}

func signTestToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}
