package testutil

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func AssertEqual(t testing.TB, got, want any, opts ...cmp.Option) {
	t.Helper()

	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}

func AssertJSONEqual(t testing.TB, a, b []byte) {
	t.Helper()

	var va any
	if err := json.Unmarshal(a, &va); err != nil {
		t.Fatalf("invalid JSON A: %v\nA: %s", err, string(a))
	}

	var vb any
	if err := json.Unmarshal(b, &vb); err != nil {
		t.Fatalf("invalid JSON B: %v\nB: %s", err, string(b))
	}

	if diff := cmp.Diff(va, vb); diff != "" {
		t.Fatalf("JSON mismatch (-A +B):\n%s", diff)
	}
}

func EquateEmpty() cmp.Option {
	return cmpopts.EquateEmpty()
}

func IgnoreFields(typ any, names ...string) cmp.Option {
	return cmpopts.IgnoreFields(typ, names...)
}
