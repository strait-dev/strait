package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAPIKeyPrefixLen_MatchesStraitTagPlusFiveHex locks the constant's
// definition: "strait_" (7 chars) + 5 hex chars = 12. A future change that
// touches this number must update the comment and every mint site, so this
// test is the canary that breaks first.
func TestAPIKeyPrefixLen_MatchesStraitTagPlusFiveHex(t *testing.T) {
	t.Parallel()
	assert.Equal(t,
		len("strait_")+5, APIKeyPrefixLen)

}

// TestAPIKeyPrefixLen_LongerThanTagAlone guards against an accidental
// reduction to just the literal "strait_" prefix. Without the random
// component the prefix is identical for every key and useless as an
// identifier in audit logs and the UI.
func TestAPIKeyPrefixLen_LongerThanTagAlone(t *testing.T) {
	t.Parallel()
	assert.False(t,
		APIKeyPrefixLen <=
			len("strait_"))

}

// TestAPIKeyPrefixLen_FitsRawKey asserts the prefix length is safely below
// the minimum raw-key length we ever mint. A "strait_" + 64 hex production
// key is 71 chars; a slice well under that is safe even if the entropy
// length is later trimmed.
func TestAPIKeyPrefixLen_FitsRawKey(t *testing.T) {
	t.Parallel()
	const minRawKeyLen = len("strait_") + 32
	assert.False(t,
		APIKeyPrefixLen >=
			minRawKeyLen)

	// 32 hex chars = 16 random bytes

}
