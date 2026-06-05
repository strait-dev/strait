package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// FuzzSecretScanner_NeverPanics asserts the scanner is total — for any
// input it either matches a shape (returns hits) or doesn't, but never
// panics. Regex-based code rarely panics, but fuzzing catches edge cases
// in map/slice traversal.
func FuzzSecretScanner_NeverPanics(f *testing.F) {
	seeds := []string{
		"",
		"hello",
		"sk_live_1234567890abcdefghij",
		"whsec_aaaaaaaaaaaaaaaaaaaaaaaaaa",
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abc.def",
		"AKIAIOSFODNN7EXAMPLE",
		"Bearer abcdef1234567890ABCDEF",
		"-----BEGIN RSA PRIVATE KEY-----\nfoo\n-----END-----",
		"01HXABCXYZ1234567890ABCDEF",
		"proj_abc123",
		"\x00\xff\x7f",
		strings.Repeat("X", 4096),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		// Must not panic.
		_ = scanForSecrets(s)
		_ = scanForSecrets(map[string]any{"key": s})
		_ = scanForSecrets([]any{s, s, s})
		_ = scanForSecrets(map[string]any{
			"nested": map[string]any{
				"deeper": []any{s, map[string]any{"leaf": s}},
			},
		})
	})
}

// FuzzSecretScanner_KnownPrefixesAreDetected asserts that any string
// starting with a known secret prefix and satisfying the minimum length
// requirement is detected. Fuzzer mutates the suffix.
func FuzzSecretScanner_KnownPrefixesAreDetected(f *testing.F) {
	f.Add("sk_live_", "abcdef0123456789XYZabcdef")
	f.Add("sk_test_", "aaaaaaaaaaaaaaaaaaaa")
	f.Add("whsec_", "abcdef0123456789abcdef")
	f.Add("ghp_", "abcdefghij0123456789XYZab")

	f.Fuzz(func(t *testing.T, prefix, suffix string) {
		// Restrict suffix to alphanumerics so we hit the pattern
		// constraints; otherwise the scanner correctly ignores
		// non-conforming strings.
		var clean strings.Builder
		for _, r := range suffix {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				clean.WriteRune(r)
			}
		}
		cs := clean.String()
		if len(cs) < 20 {
			t.Skip("suffix too short after cleaning")
		}

		// Only test prefixes we know produce hits.
		validPrefixes := map[string]bool{
			"sk_live_": true,
			"sk_test_": true,
			"whsec_":   true,
			"ghp_":     true,
		}
		if !validPrefixes[prefix] {
			t.Skip()
		}

		payload := prefix + cs
		hits := scanForSecrets(payload)
		assert.NotEmpty(t, hits)

	})
}
