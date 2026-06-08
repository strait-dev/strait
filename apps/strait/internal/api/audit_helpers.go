package api

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"unsafe"
)

// hashIdempotencyKey returns a short SHA-256 prefix of the idempotency key,
// safe for audit logs. Raw keys are never recorded.
func hashIdempotencyKey(key string) string {
	if key == "" {
		return ""
	}
	// sha256.Sum256 only reads the input synchronously, so this avoids copying
	// the idempotency key before hashing it for the trigger audit hot path.
	sum := sha256.Sum256(unsafe.Slice(unsafe.StringData(key), len(key)))
	var out [16]byte
	hex.Encode(out[:], sum[:8])
	return string(out[:])
}

// tagKeys returns the sorted tag keys of a tag map. Values are never included
// in audit events because they may contain user data.
func tagKeys(tags map[string]string) []string {
	if len(tags) == 0 {
		return nil
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
