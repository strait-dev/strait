package crypto

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashAPIKey returns the SHA-256 hex digest of a raw API key string. This is the
// single canonical hashing function for API keys: the HTTP API, the gRPC worker
// plane, and the pentest seeding tool all delegate to it so their hashes cannot
// silently diverge (a divergence would make seeded or worker keys unauthable
// against the rest of the system).
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
