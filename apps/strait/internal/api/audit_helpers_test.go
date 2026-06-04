package api

import (
	"reflect"
	"testing"
)

func TestTagKeysSortedAndValueFree(t *testing.T) {
	t.Parallel()

	got := tagKeys(map[string]string{
		"team":   "platform",
		"env":    "prod",
		"region": "eu",
	})
	want := []string{"env", "region", "team"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tagKeys() = %#v, want %#v", got, want)
	}
}

func TestTagKeysEmpty(t *testing.T) {
	t.Parallel()

	if got := tagKeys(nil); got != nil {
		t.Fatalf("tagKeys(nil) = %#v, want nil", got)
	}
}

func TestHashIdempotencyKey(t *testing.T) {
	t.Parallel()

	if got := hashIdempotencyKey(""); got != "" {
		t.Fatalf("hashIdempotencyKey(\"\") = %q, want empty", got)
	}
	if got := hashIdempotencyKey("idem-123"); got != "f6fdb32bfd0ba473" {
		t.Fatalf("hashIdempotencyKey() = %q, want stable SHA-256 prefix", got)
	}
}
