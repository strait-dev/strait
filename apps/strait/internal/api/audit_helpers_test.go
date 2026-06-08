package api

import (
	"reflect"
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
	require.Equal(t, "f6fdb32bfd0ba473",

		hashIdempotencyKey("idem-123"))
}

func BenchmarkHashIdempotencyKey(b *testing.B) {
	key := "idem-0123456789abcdef0123456789abcdef"

	b.ReportAllocs()
	for b.Loop() {
		hash := hashIdempotencyKey(key)
		if len(hash) != 16 {
			b.Fatalf("hashIdempotencyKey() length = %d, want 16", len(hash))
		}
	}
}
