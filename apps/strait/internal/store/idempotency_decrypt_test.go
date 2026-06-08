package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDecryptIdempotencyResponseBody_PlaintextPassthrough is the regression
// guard for the fragile encrypted-body detection: legacy/plaintext bodies must
// be returned verbatim. The previous substring probe (bytes.Contains for
// `"encrypted"`) misclassified any body containing that token and errored on
// non-JSON bodies, corrupting idempotent replays.
func TestDecryptIdempotencyResponseBody_PlaintextPassthrough(t *testing.T) {
	t.Parallel()
	q := &Queries{} // no encryption key needed for the plaintext paths

	cases := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"non-json plaintext", "connection refused"},
		{"plaintext html", "<html><body>encrypted</body></html>"},
		{"json containing the word encrypted", `{"message":"the data is encrypted at rest"}`},
		{"json with encrypted false", `{"encrypted":false,"body":"plain"}`},
		{"plain json object", `{"status":"ok","value":42}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := q.decryptIdempotencyResponseBody([]byte(tc.raw))
			require.NoError(t, err)
			if tc.raw == "" {
				require.Nil(t, got)
				return
			}
			require.Equal(t, tc.raw, string(got), "plaintext body must pass through unchanged")
		})
	}
}

// TestDecryptIdempotencyResponseBody_EncryptedRoundTrip verifies a genuinely
// encrypted body is detected and decrypted back to the original plaintext.
func TestDecryptIdempotencyResponseBody_EncryptedRoundTrip(t *testing.T) {
	t.Parallel()
	q := &Queries{}
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")

	original := []byte(`{"status":"ok","secret":"sensitive-value"}`)
	wrapped, err := q.encryptIdempotencyResponseBody(original)
	require.NoError(t, err)
	require.NotContains(t, string(wrapped), "sensitive-value", "stored body must be ciphertext")

	got, err := q.decryptIdempotencyResponseBody(wrapped)
	require.NoError(t, err)
	require.Equal(t, string(original), string(got))
}
