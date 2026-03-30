package testutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
