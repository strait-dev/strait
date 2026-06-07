package api

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTagKeysSortedAndValueFree(t *testing.T) {
	t.Parallel()

	got := tagKeys(map[string]string{
		"team":   "platform",
		"env":    "prod",
		"region": "eu",
	})
	want := []string{"env", "region", "team"}
	require.True(
		t, reflect.
			DeepEqual(got, want))
}

func TestTagKeysEmpty(t *testing.T) {
	t.Parallel()
	require.Nil(t, tagKeys(nil))
}

func TestHashIdempotencyKey(t *testing.T) {
	t.Parallel()
	require.Empty(t, hashIdempotencyKey(""))

	got := hashIdempotencyKey("idem-123")
	// Full 256-bit digest (64 hex chars), not a 64-bit truncated prefix.
	require.Len(t, got, 64)
	require.True(t, strings.HasPrefix(got, "f6fdb32bfd0ba473"),
		"must remain the SHA-256 of the key, just untruncated")

	// Distinct keys must not collide.
	require.NotEqual(t, got, hashIdempotencyKey("idem-124"))
}
