package billing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestThresholdPercents_IsArrayNotSlice locks the immutability stance via
// a compile-time check: a slice would not be assignable to a fixed-size
// array variable, so this declaration only builds while thresholdPercents
// is an array. A slice could be reslice-mutated by a downstream caller
// (or by a test helper that "just adds 50% for fun") and the dedupe SETNX
// would never notice. An array is a value type and cannot be assigned
// through.
func TestThresholdPercents_IsArrayNotSlice(t *testing.T) {
	t.Parallel()
	var compileTimeArrayCheck [3]int = thresholdPercents //nolint:staticcheck // intentional value copy as type assertion
	_ = compileTimeArrayCheck
}

// TestThresholdPercents_StrictlyAscending guards the highest-crossed scan
// in maybeEmitUsageThreshold: it walks the buckets in declared order and
// keeps the last hit. If a future edit drops them out of order (e.g. 100,
// 90, 80) the scan returns the wrong bucket and the dedupe key collides
// with the lower one. The init() panic catches the same drift at startup;
// this test catches it at compile-and-test time.
func TestThresholdPercents_StrictlyAscending(t *testing.T) {
	t.Parallel()
	for i := 1; i < len(thresholdPercents); i++ {
		assert.False(t, thresholdPercents[i] <= thresholdPercents[i-1])

	}
}

// TestThresholdPercents_AllInValidRange rejects nonsense bucket values.
// A percent of 0 always triggers (zero usage already crosses 0%) and a
// percent above 100 cannot be reached by metered usage that respects its
// own limit, so both would emit zero notifications across the org base.
func TestThresholdPercents_AllInValidRange(t *testing.T) {
	t.Parallel()
	for _, p := range thresholdPercents {
		assert.False(t, p <=

			0 || p > 100)

	}
}

// TestThresholdPercents_CanonicalValues is the deliberate freeze. Customer
// notification copy, dashboards, and test fixtures all reference the
// 80/90/100 set explicitly. Changing the canonical buckets must come with
// a coordinated change to those surfaces, so make this test the canary.
func TestThresholdPercents_CanonicalValues(t *testing.T) {
	t.Parallel()
	want := [...]int{80, 90, 100}
	assert.Equal(t, want,

		thresholdPercents)

}
