package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"strait/internal/config"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOIDCVerify_ValidToken verifies that a correctly signed JWT with valid
// claims passes verification and returns the expected subject.
func TestOIDCVerify_ValidToken(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-1",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	claims, err := v.verify(signed)
	require.NoError(t,
		err)
	require.Equal(t,
		"user-1", claims.
			Subject)
}

// TestOIDCVerify_ExpiredToken verifies that a token expired well beyond the
// 30-second leeway is rejected.
func TestOIDCVerify_ExpiredToken(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-expired",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-5 * time.Minute)),
	})

	_, err := v.verify(signed)
	require.Error(t,
		err)
}

// TestOIDCVerify_ExpiryBoundary checks behaviour around the 30-second leeway.
// A token expired 1s ago is within the 30s window and should be accepted.
// A token expired 31s ago is outside the window and should be rejected.
func TestOIDCVerify_ExpiryBoundary(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	t.Run("expired 1s ago accepted within 30s leeway", func(t *testing.T) {
		t.Parallel()
		signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
			Subject:   "user-boundary-past",
			Issuer:    "https://issuer.test",
			Audience:  []string{"aud-test"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Second)),
		})
		claims, err := v.verify(signed)
		require.NoError(t,
			err)
		require.Equal(t,
			"user-boundary-past",
			claims.
				Subject)
	})

	t.Run("expired 31s ago rejected beyond leeway", func(t *testing.T) {
		t.Parallel()
		signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
			Subject:   "user-boundary-past-31",
			Issuer:    "https://issuer.test",
			Audience:  []string{"aud-test"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-31 * time.Second)),
		})
		_, err := v.verify(signed)
		require.Error(t,
			err)
	})

	t.Run("expires 1s from now accepted", func(t *testing.T) {
		t.Parallel()
		signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
			Subject:   "user-boundary-future",
			Issuer:    "https://issuer.test",
			Audience:  []string{"aud-test"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Second)),
		})
		claims, err := v.verify(signed)
		require.NoError(t,
			err)
		require.Equal(t,
			"user-boundary-future",
			claims.
				Subject)
	})
}

// TestOIDCVerify_WrongIssuer verifies that a mismatched issuer is rejected.
func TestOIDCVerify_WrongIssuer(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-wrong-iss",
		Issuer:    "https://evil.example.com",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	_, err := v.verify(signed)
	require.Error(t,
		err)
}

// TestOIDCVerify_WrongAudience verifies that a mismatched audience is rejected.
func TestOIDCVerify_WrongAudience(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-wrong-aud",
		Issuer:    "https://issuer.test",
		Audience:  []string{"wrong-audience"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	_, err := v.verify(signed)
	require.Error(t,
		err)
}

// TestOIDCVerify_EmptySubject verifies that an empty sub claim is rejected
// even when the token is otherwise valid.
func TestOIDCVerify_EmptySubject(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	_, err := v.verify(signed)
	require.Error(t,
		err)
	require.Contains(t, err.Error(), "subject")
}

// TestOIDCVerify_InvalidSignature verifies that a tampered token payload is
// rejected due to signature mismatch.
func TestOIDCVerify_InvalidSignature(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-tampered",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	// Tamper with the payload section of the JWT.
	parts := strings.SplitN(signed, ".", 3)
	require.Len(t, parts,
		3)

	parts[1] += "AAAA"
	tampered := strings.Join(parts, ".")

	_, err := v.verify(tampered)
	require.Error(t,
		err)
}

// TestOIDCVerify_MalformedJWT verifies that random non-JWT strings are
// handled gracefully without panicking.
func TestOIDCVerify_MalformedJWT(t *testing.T) {
	t.Parallel()

	_, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	malformed := []string{
		"",
		"not-a-jwt",
		"a.b.c",
		"eyJhbGciOiJSUzI1NiJ9.bad-payload.bad-sig",
		strings.Repeat("A", 10000),
	}
	for _, tok := range malformed {
		_, err := v.verify(tok)
		assert.Error(t, err)
	}
}

// TestOIDCVerify_NullBytesInClaims verifies that null bytes embedded in claim
// values do not cause panics or bypass validation.
func TestOIDCVerify_NullBytesInClaims(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user\x00injected",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	// The token may or may not be accepted depending on the JWT library,
	// but it must not panic. If accepted, the subject should contain the
	// null byte faithfully.
	claims, err := v.verify(signed)
	if err != nil {
		return // Rejected is acceptable.
	}
	require.Equal(t,
		"user\x00injected",
		claims.Subject,
	)
}

// TestNewOIDCVerifier_InvalidPEM verifies that an invalid PEM string is
// rejected when creating a verifier.
func TestNewOIDCVerifier_InvalidPEM(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		OIDCEnabled:      true,
		OIDCIssuer:       "https://issuer.test",
		OIDCAudience:     "aud-test",
		OIDCPublicKeyPEM: "not-valid-pem-data",
	}
	_, err := newOIDCVerifier(cfg)
	require.Error(t,
		err)
}

// TestNewOIDCVerifier_EmptyConfig verifies that an empty PEM is rejected when
// OIDC is enabled, and a disabled verifier is returned when OIDC is off.
func TestNewOIDCVerifier_EmptyConfig(t *testing.T) {
	t.Parallel()

	t.Run("enabled with empty PEM", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{
			OIDCEnabled:      true,
			OIDCIssuer:       "https://issuer.test",
			OIDCAudience:     "aud-test",
			OIDCPublicKeyPEM: "",
		}
		_, err := newOIDCVerifier(cfg)
		require.Error(t,
			err)
	})

	t.Run("disabled returns verifier", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{OIDCEnabled: false}
		v, err := newOIDCVerifier(cfg)
		require.NoError(t,
			err)
		require.NotNil(t,
			v)
		require.False(t,
			v.enabled)
	})
}

// FuzzOIDCVerify_MalformedTokens fuzzes the verify function with random
// strings to ensure it never panics.
func FuzzOIDCVerify_MalformedTokens(f *testing.F) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		f.Fatalf("generate rsa key: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		f.Fatalf("marshal pub key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	pk, err := jwt.ParseRSAPublicKeyFromPEM(pubPEM)
	if err != nil {
		f.Fatalf("parse rsa public key: %v", err)
	}
	v := &oidcVerifier{
		enabled:   true,
		issuer:    "https://issuer.test",
		audience:  "aud-test",
		publicKey: pk,
	}

	f.Add("")
	f.Add("a.b.c")
	f.Add("eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiIxIn0.bad")
	f.Add(strings.Repeat("x", 8192))

	f.Fuzz(func(t *testing.T, token string) {
		// Must not panic regardless of input.
		_, _ = v.verify(token)
	})
}

// mustOIDCVerifier is a test helper that creates an enabled oidcVerifier from
// a PEM-encoded public key.
func mustOIDCVerifier(t *testing.T, pubPEM []byte, issuer, audience string) *oidcVerifier {
	t.Helper()
	cfg := &config.Config{
		OIDCEnabled:      true,
		OIDCIssuer:       issuer,
		OIDCAudience:     audience,
		OIDCPublicKeyPEM: string(pubPEM),
	}
	v, err := newOIDCVerifier(cfg)
	require.NoError(t,
		err)

	return v
}
