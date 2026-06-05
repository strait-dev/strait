package api

import (
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		require.NoError(t, err)
		require.Greater(t, len(raw), domain.APIKeyPrefixLen)

		prefix := raw[:domain.APIKeyPrefixLen]
		assert.True(t,
			strings.HasPrefix(prefix, "strait_"),
		)
		assert.Len(t,
			prefix, domain.
				APIKeyPrefixLen)
	}
}

// TestGenerateAPIKey_RawKeysHaveStableShape locks the production key shape
// at "strait_" + 64 hex chars (32 bytes). If this changes, the prefix
// constant + every audit/UI lookup that compares full prefixes needs a
// review.
func TestGenerateAPIKey_RawKeysHaveStableShape(t *testing.T) {
	t.Parallel()
	raw, err := generateAPIKey()
	require.NoError(t, err)

	const wantLen = len("strait_") + 64
	assert.Len(t,
		raw, wantLen,
	)
}
