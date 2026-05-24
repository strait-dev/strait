package store

import (
	"testing"
)

// FuzzIdempotencyAdvisoryKey_DeterministicAndDistinct asserts that the
// advisory-key derivation is deterministic across two calls with the same
// inputs, and that flipping projectID/key never produces an obvious symmetric
// collapse.
func FuzzIdempotencyAdvisoryKey_DeterministicAndDistinct(f *testing.F) {
	f.Add("proj-1", "key-a")
	f.Add("", "")
	f.Add("a", "b:c")
	f.Add("a:b", "c")
	f.Add("very-long-project-name-with-lots-of-chars", "k")
	f.Add(string([]byte{0, 1, 2, 255}), string([]byte{255, 254, 253}))

	f.Fuzz(func(t *testing.T, projectID, key string) {
		a := idempotencyAdvisoryKey(projectID, key)
		b := idempotencyAdvisoryKey(projectID, key)
		if a != b {
			t.Fatalf("non-deterministic: %d vs %d for (%q,%q)", a, b, projectID, key)
		}
		// Swap: must usually differ. When projectID == key the hash is naturally
		// equal under swap, so only assert when they differ.
		if projectID != key {
			swapped := idempotencyAdvisoryKey(key, projectID)
			if swapped == a {
				t.Fatalf("swap collision for (%q,%q): %d", projectID, key, a)
			}
		}
	})
}
