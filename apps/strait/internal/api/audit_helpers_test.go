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
