package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorHash_ASCIIBeyondLimitTruncatesAtByte200(t *testing.T) {
	t.Parallel()

	// ASCII: 1 byte per rune, so 250 chars -> 250 runes; expect truncation
	// to first 200 runes.
	msg := strings.Repeat("a", 250)
	want := sha256.Sum256([]byte(strings.Repeat("a", 200)))
	wantHex := hex.EncodeToString(want[:8])
	require.Equal(t, wantHex,
		errorHash(msg))

}

func TestErrorHash_MultibyteUnderRuneLimitFullyHashed(t *testing.T) {
	t.Parallel()

	// 100 multi-byte runes (~300 bytes). With the byte-based truncation that
	// the original implementation used this would have been split mid-rune
	// and produced an invalid UTF-8 prefix, mismatching the full-string hash.
	msg := strings.Repeat("漢", 100)
	want := sha256.Sum256([]byte(msg))
	wantHex := hex.EncodeToString(want[:8])
	require.Equal(t, wantHex,
		errorHash(msg))

}

func TestErrorHash_MultibyteOverRuneLimitTruncatedByRune(t *testing.T) {
	t.Parallel()

	// 250 multi-byte runes; truncate to first 200 runes (not 200 bytes).
	msg := strings.Repeat("漢", 250)
	prefix := strings.Repeat("漢", 200)
	want := sha256.Sum256([]byte(prefix))
	wantHex := hex.EncodeToString(want[:8])
	require.Equal(t, wantHex,
		errorHash(msg))

	// Appending more runes past the 200-rune boundary must not change the hash.
	more := msg + strings.Repeat("漢", 5)
	require.Equal(t, wantHex,
		errorHash(more))

}

func TestErrorHash_EmojiTruncationStable(t *testing.T) {
	t.Parallel()

	// Emoji: 4 bytes each, 60 runes -> 240 bytes. Under 200-rune limit so
	// hash equals full-string hash.
	msg := strings.Repeat("🚀", 60)
	want := sha256.Sum256([]byte(msg))
	wantHex := hex.EncodeToString(want[:8])
	require.Equal(t, wantHex,
		errorHash(msg))

}

func TestErrorHash_Stable(t *testing.T) {
	t.Parallel()

	msg := "connection refused"
	first := errorHash(msg)
	second := errorHash(msg)
	require.Equal(t, second,
		first)

}
