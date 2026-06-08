package api

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// hashIdempotencyKey returns the SHA-256 digest of the idempotency key as hex,
// safe for audit logs. Raw keys are never recorded. The full 256-bit digest is
// used (not a truncated prefix): a 64-bit prefix collides by the birthday bound
// at ~2^32 keys, which is reachable for a high-throughput, multi-tenant
// orchestrator and would produce false correlations in audit queries.
func hashIdempotencyKey(key string) string {
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
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
