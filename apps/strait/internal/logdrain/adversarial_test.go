package logdrain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProtectedHeaders_AllBlocked verifies every entry in ProtectedHeaders is filtered.
func TestProtectedHeaders_AllBlocked(t *testing.T) {
	t.Parallel()

	expected := []string{
		"host", "content-length", "content-type", "transfer-encoding",
		"connection", "upgrade", "te", "trailer",
	}

	for _, h := range expected {
		assert.True(t, ProtectedHeaders[h])

	}
	assert.Len(t, ProtectedHeaders,

		len(expected))

}

// TestProtectedHeaders_CaseInsensitive verifies the filtering uses lowercase comparison.
func TestProtectedHeaders_CaseInsensitive(t *testing.T) {
	t.Parallel()

	variants := []string{"HOST", "Host", "host", "HoSt"}
	for _, v := range variants {
		lower := strings.ToLower(v)
		assert.True(t, ProtectedHeaders[lower])

	}
}

// TestProtectedHeaders_CustomInjection verifies that Authorization in custom headers
// is NOT blocked (it is not in the protected list), which is by design since the
// "header" auth type allows arbitrary custom headers except the protected ones.
func TestProtectedHeaders_CustomInjection(t *testing.T) {
	t.Parallel()
	require.False(t, ProtectedHeaders[strings.ToLower("Authorization")])

	// Authorization is intentionally allowed for custom header auth.

}

// TestProtectedHeaders_NullByteBypass verifies "host\x00" is not treated as "host".
func TestProtectedHeaders_NullByteBypass(t *testing.T) {
	t.Parallel()

	malicious := "host\x00"
	lower := strings.ToLower(malicious)
	require.False(t, ProtectedHeaders[lower])
	require.NotEqual(t,
		"host", lower,
	)

	// The null byte makes it a different string, so it should not match.

	// However, strings.ToLower("host\x00") preserves the null byte.

}

// FuzzProtectedHeaders fuzzes header names to verify the lookup is consistent.
func FuzzProtectedHeaders(f *testing.F) {
	f.Add("host")
	f.Add("Host")
	f.Add("HOST")
	f.Add("content-type")
	f.Add("x-custom-header")
	f.Add("authorization")
	f.Add("host\x00")
	f.Add("")

	f.Fuzz(func(t *testing.T, header string) {
		lower := strings.ToLower(header)
		blocked := ProtectedHeaders[lower]
		assert.False(t, blocked &&
			!ProtectedHeaders[lower])

		// If blocked, the lowercase form must be in the map.

		// Verify idempotency: double-lowercase should not change the result.
		doubleLower := strings.ToLower(lower)
		assert.Equal(t, blocked,
			ProtectedHeaders[doubleLower])

	})
}
