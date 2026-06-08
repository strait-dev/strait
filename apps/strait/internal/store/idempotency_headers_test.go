package store

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSanitizeIdempotencyHeaders_StripsUnsafe is the regression guard for
// replaying credentials, hop-by-hop, and CORS headers from a cached response.
func TestSanitizeIdempotencyHeaders_StripsUnsafe(t *testing.T) {
	t.Parallel()
	h := http.Header{
		"Content-Type":                     {"application/json"},
		"X-Request-Id":                     {"abc"},
		"Set-Cookie":                       {"s=1"},
		"Authorization":                    {"Bearer x"},
		"Transfer-Encoding":                {"chunked"},
		"Connection":                       {"keep-alive"},
		"Access-Control-Allow-Origin":      {"https://evil.example"},
		"Access-Control-Allow-Credentials": {"true"},
		"Vary":                             {"Origin"},
	}
	out := sanitizeIdempotencyHeaders(h)

	// Safe headers preserved.
	require.Equal(t, "application/json", out.Get("Content-Type"))
	require.Equal(t, "abc", out.Get("X-Request-Id"))

	// Unsafe headers stripped.
	for _, k := range []string{
		"Set-Cookie", "Authorization", "Transfer-Encoding", "Connection",
		"Access-Control-Allow-Origin", "Access-Control-Allow-Credentials", "Vary",
	} {
		require.Emptyf(t, out.Get(k), "header %q must be stripped from idempotency replay", k)
	}
}
