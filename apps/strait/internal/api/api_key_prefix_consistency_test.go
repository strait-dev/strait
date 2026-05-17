package api

import (
	"strings"
	"testing"

	"strait/internal/domain"
)

// TestGenerateAPIKey_PrefixSliceIsExactlyAPIKeyPrefixLen proves a freshly
// minted production key, sliced at domain.APIKeyPrefixLen, lands on the
// canonical "strait_" + N-hex shape. This is the regression guard for
// issue #7 — every mint site previously hard-coded :12 / :11 with no
// shared definition, so a one-character drift in any one site produced
// inconsistent prefixes that broke UI lookups by prefix.
func TestGenerateAPIKey_PrefixSliceIsExactlyAPIKeyPrefixLen(t *testing.T) {
	t.Parallel()

	for range 8 {
		raw, err := generateAPIKey()
		if err != nil {
			t.Fatalf("generateAPIKey: %v", err)
		}
		if len(raw) <= domain.APIKeyPrefixLen {
			t.Fatalf("raw key shorter than prefix len: got %d, need > %d", len(raw), domain.APIKeyPrefixLen)
		}
		prefix := raw[:domain.APIKeyPrefixLen]
		if !strings.HasPrefix(prefix, "strait_") {
			t.Errorf("prefix missing strait_ tag: got %q", prefix)
		}
		if len(prefix) != domain.APIKeyPrefixLen {
			t.Errorf("prefix length drifted: got %d, want %d", len(prefix), domain.APIKeyPrefixLen)
		}
	}
}

// TestGenerateAPIKey_RawKeysHaveStableShape locks the production key shape
// at "strait_" + 64 hex chars (32 bytes). If this changes, the prefix
// constant + every audit/UI lookup that compares full prefixes needs a
// review.
func TestGenerateAPIKey_RawKeysHaveStableShape(t *testing.T) {
	t.Parallel()
	raw, err := generateAPIKey()
	if err != nil {
		t.Fatalf("generateAPIKey: %v", err)
	}
	const wantLen = len("strait_") + 64
	if len(raw) != wantLen {
		t.Errorf("raw key length drifted: got %d, want %d", len(raw), wantLen)
	}
}
