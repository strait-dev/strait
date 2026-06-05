package api

import (
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(
		t, err)

	// Use crypto/rand properly

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
		require.NoError(
			t, err)
		assert.Equal(t,
			"user-123",
			claims.
				Subject,
		)
		assert.Equal(t,
			"user@example.com",

			claims.Email)
		assert.Equal(t,
			"Test User",
			claims.
				Name)

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
		require.NoError(
			t, err)
		assert.Equal(t,
			"user-456",
			claims.
				Subject,
		)

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
		require.Error(t,
			err)

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
		require.Error(t,
			err)

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
		require.Error(t,
			err)

	})

	t.Run("missing subject rejected", func(t *testing.T) {
		token := signTestToken(t, privateKey, jwt.MapClaims{
			"iss": issuer,
			"aud": audience,
			"iat": time.Now().Unix(),
			"exp": time.Now().Add(time.Hour).Unix(),
		})

		_, err := v.verify(token)
		require.Error(t,
			err)

	})

	t.Run("wrong signing key rejected", func(t *testing.T) {
		// Sign with a different key
		wrongKey, err := rsa.GenerateKey(nil, 2048)
		require.NoError(
			t, err)

		token := signTestToken(t, wrongKey, jwt.MapClaims{
			"iss": issuer,
			"aud": audience,
			"sub": "user-123",
			"iat": time.Now().Unix(),
			"exp": time.Now().Add(time.Hour).Unix(),
		})

		_, err = v.verify(token)
		require.Error(t,
			err)

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
		require.NoError(
			t, err)
		assert.Equal(t,
			"user-123",
			claims.
				Subject,
		)

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
			require.NotEqual(t, "*", s)

		}
		require.False(t,
			len(scopes) != 1 ||

				scopes[0] != "jobs:read")

	})

	t.Run("api-keys:manage stripped", func(t *testing.T) {
		c := &oidcClaims{Scope: "jobs:read api-keys:manage runs:read"}
		scopes := c.Scopes()
		for _, s := range scopes {
			require.NotEqual(t, "api-keys:manage",

				s)

		}
		require.Len(t, scopes,
			2)

	})

	t.Run("rbac:manage stripped", func(t *testing.T) {
		c := &oidcClaims{Scope: "rbac:manage stats:read"}
		scopes := c.Scopes()
		for _, s := range scopes {
			require.NotEqual(t, "rbac:manage",

				s,
			)

		}
		require.False(t,
			len(scopes) != 1 ||

				scopes[0] != "stats:read")

	})

	t.Run("all privileged scopes stripped returns empty upper bound", func(t *testing.T) {
		c := &oidcClaims{Scope: "* api-keys:manage rbac:manage"}
		scopes := c.Scopes()
		require.NotNil(t,
			scopes)
		require.Len(t, scopes,
			0)

	})

	t.Run("absent scope returns nil", func(t *testing.T) {
		c := &oidcClaims{}
		require.Nil(t, c.Scopes())

	})

	t.Run("normal scopes pass through", func(t *testing.T) {
		c := &oidcClaims{Scope: "jobs:read jobs:write runs:read workflows:trigger"}
		scopes := c.Scopes()
		require.Len(t, scopes,
			4)

	})

	t.Run("OIDC scopes stripped", func(t *testing.T) {
		c := &oidcClaims{Scope: "openid profile email jobs:read"}
		scopes := c.Scopes()
		require.False(t,
			len(scopes) != 1 ||

				scopes[0] != "jobs:read")

	})
}

func signTestToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(key)
	require.NoError(
		t, err)

	return signed
}
