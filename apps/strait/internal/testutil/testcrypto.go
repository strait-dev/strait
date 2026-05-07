package testutil

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/golang-jwt/jwt/v5"
)

// GenerateTestSecret generates a cryptographically random hex-encoded secret
// of the specified byte length. Use this instead of hardcoding test secrets
// to avoid gitleaks false positives and ensure tests use realistic key material.
func GenerateTestSecret(byteLen int) string {
	if byteLen <= 0 {
		panic("testutil.GenerateTestSecret: byteLen must be > 0")
	}
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("testutil.GenerateTestSecret: %v", err))
	}
	return hex.EncodeToString(b)
}

// GenerateTestWebhookSecret generates a webhook secret with the "whsec_" prefix,
// matching the format used in production.
func GenerateTestWebhookSecret() string {
	return "whsec_" + GenerateTestSecret(16)
}

// GenerateTestJWTKey generates a 32-byte (256-bit) key suitable for HMAC-SHA256
// JWT signing in tests.
func GenerateTestJWTKey() string {
	return GenerateTestSecret(32)
}

// GenerateTestInternalSecret generates a 32-byte secret suitable for the
// INTERNAL_SECRET config value (minimum 16 chars required).
func GenerateTestInternalSecret() string {
	return GenerateTestSecret(32)
}

// GenerateTestAPIKey generates a random API key with the "strait_" prefix,
// matching the production format.
func GenerateTestAPIKey() string {
	return "strait_" + GenerateTestSecret(32)
}

// GenerateTestEncryptionKey generates a 32-byte key suitable for AES-256
// encryption (ENCRYPTION_KEY or SECRET_ENCRYPTION_KEY config values).
func GenerateTestEncryptionKey() string {
	return GenerateTestSecret(32)
}

// GenerateTestDeviceCode generates a random 32-byte hex device code,
// matching the format from cli_auth.go's generateDeviceCode.
func GenerateTestDeviceCode() string {
	return GenerateTestSecret(32)
}

// GenerateTestUserCode generates an 8-character user code using the same
// alphabet as production (ABCDEFGHJKLMNPQRSTUVWXYZ23456789 -- no 0, 1, I, O
// to avoid confusion).
func GenerateTestUserCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("testutil.GenerateTestUserCode: %v", err))
	}
	code := make([]byte, 8)
	for i := range code {
		code[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(code)
}

// GenerateTestSignatureSecret generates a random 32-byte base64-encoded secret
// suitable for event source HMAC signature verification.
func GenerateTestSignatureSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("testutil.GenerateTestSignatureSecret: %v", err))
	}
	return base64.StdEncoding.EncodeToString(b)
}

// GenerateTestRunToken generates a signed JWT run token with the given run ID
// and signing key, matching the format used by the SDK auth system.
func GenerateTestRunToken(runID, signingKey string) string {
	claims := struct {
		Attempt int `json:"attempt,omitempty"`
		jwt.RegisteredClaims
	}{
		Attempt: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    domain.RunTokenIssuer,
			Subject:   runID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(signingKey))
	if err != nil {
		panic(fmt.Sprintf("testutil.GenerateTestRunToken: %v", err))
	}
	return signed
}

// GenerateTestSSEToken generates a signed JWT SSE token with the given project
// ID, scopes, and signing key, matching the format from sse_token.go.
func GenerateTestSSEToken(projectID string, scopes []string, signingKey string) string {
	type sseTokenClaims struct {
		jwt.RegisteredClaims
		ProjectID string   `json:"pid"`
		Scopes    []string `json:"scp,omitempty"`
	}
	claims := sseTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "strait:sse",
			Subject:   projectID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		ProjectID: projectID,
		Scopes:    scopes,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(signingKey))
	if err != nil {
		panic(fmt.Sprintf("testutil.GenerateTestSSEToken: %v", err))
	}
	return signed
}

// GenerateTestClaimToken generates a random 32-byte hex token suitable for
// job run claim tokens.
func GenerateTestClaimToken() string {
	return GenerateTestSecret(32)
}

// GenerateTestKeyHash generates a random 32-byte hex string suitable for
// use as an API key hash in tests.
func GenerateTestKeyHash() string {
	return GenerateTestSecret(32)
}

// GenerateTestDatabaseURL generates a test PostgreSQL connection string with
// a random database name to avoid hardcoding connection strings in tests.
func GenerateTestDatabaseURL() string {
	return fmt.Sprintf("postgres://testuser:testpass@localhost:5432/test_%s?sslmode=disable", GenerateTestSecret(4))
}

// GenerateTestRedisURL generates a test Redis connection string.
func GenerateTestRedisURL() string {
	return "redis://localhost:6379/0"
}
