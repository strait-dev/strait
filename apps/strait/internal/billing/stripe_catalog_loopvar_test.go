//go:build integration

package billing_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoopVarPerIterationAddress proves the Go 1.22+ semantics that the
// catalog integration test now relies on: each iteration of `for _, tc :=
// range cases` produces a fresh `tc` with its own address. Without this
// the catalog test's `&tc.key` would alias the same backing variable
// across all subtests, and Stripe's PriceListParams (which takes a
// []*string) would race against the loop's reassignments. The previous
// `lookupKeyCopy := tc.key` workaround existed precisely because this
// guarantee did not hold under Go <1.22 loopvar semantics; the copy is
// now dead code.
//
// Build tag matches the file we cleaned up so the regression guard sits
// next to its target.
func TestLoopVarPerIterationAddress(t *testing.T) {
	t.Parallel()

	type item struct{ s string }
	items := []item{{"a"}, {"b"}, {"c"}}

	pointers := make([]*string, 0, len(items))
	for _, it := range items {
		pointers = append(pointers, &it.s)
	}

	got := make([]string, 0, len(pointers))
	for _, p := range pointers {
		got = append(got, *p)
	}

	want := []string{"a", "b", "c"}
	require.Len(t, got, len(want))

	for i := range want {
		assert.Equal(t, want[i],
			got[i])

	}
}
