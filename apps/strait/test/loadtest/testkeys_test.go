//go:build loadtest

package loadtest

import (
	"crypto/rand"
	"encoding/hex"
)

// testJWTSigningKey is a cryptographically random 32-byte key generated once
// per test binary. Avoids hardcoded keys that trigger secret scanners.
var testJWTSigningKey = func() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate test JWT key: " + err.Error())
	}
	return hex.EncodeToString(b)
}()
