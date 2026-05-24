package billing

import (
	"math"
	"testing"
)

// TestPercentReached_CurrentMaxInt64SaturatesTrue is the headline overflow
// guard: current * 100 wraps into a negative int64 once current crosses
// MaxInt64/100, and the unsaturated comparison would suddenly start
// returning false for impossibly-large counters. Saturate to true — a
// counter that big is, definitionally, past any plan cap.
func TestPercentReached_CurrentMaxInt64SaturatesTrue(t *testing.T) {
	t.Parallel()
	if !percentReached(math.MaxInt64, 1_000, 80) {
		t.Errorf("MaxInt64 current must saturate to true; raw int64 multiply would wrap negative")
	}
	// Same invariant just past the overflow threshold.
	if !percentReached(math.MaxInt64/50, 1_000, 80) {
		t.Errorf("current past MaxInt64/100 must saturate to true")
	}
}

// TestPercentReached_LimitMaxInt64SaturatesFalse closes the symmetric
// branch: limit * pct overflows when limit > MaxInt64/pct, and the
// unsaturated cross-product would wrap into "true" by accident. Saturate
// to false — the threshold is mathematically defined but unreachable in
// int64 space.
func TestPercentReached_LimitMaxInt64SaturatesFalse(t *testing.T) {
	t.Parallel()
	if percentReached(1_000_000, math.MaxInt64, 80) {
		t.Errorf("MaxInt64 limit must saturate to false; raw int64 multiply would wrap positive")
	}
}

// TestPercentReached_BothMaxInt64DoesNotPanic is the "do not crash" test —
// the worst-case caller passes both extremes. Behavior is well-defined
// (the current-saturation branch wins because it is checked first), and
// the function must not panic on signed-integer overflow.
func TestPercentReached_BothMaxInt64DoesNotPanic(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("percentReached panicked on (MaxInt64, MaxInt64, 80): %v", r)
		}
	}()
	_ = percentReached(math.MaxInt64, math.MaxInt64, 80)
}

// TestPercentReached_NonPositivePctReturnsFalse adds a guard the original
// implementation lacked: pct=0 would always evaluate true (any current
// counter is >= 0% of the limit) which is meaningless and would silently
// keep the dedupe key for the wrong bucket. pct must be > 0.
func TestPercentReached_NonPositivePctReturnsFalse(t *testing.T) {
	t.Parallel()
	for _, pct := range []int{0, -1, -100} {
		if percentReached(50, 100, pct) {
			t.Errorf("pct=%d must return false (no meaningful threshold)", pct)
		}
	}
}

// TestPercentReached_NormalRangeUnchanged is a regression guard for the
// hot path: the realistic billing range (counters in the millions, limits
// up to 10^9) must keep returning the same boolean it always did. The
// overflow check must not silently flip behaviour for normal inputs.
func TestPercentReached_NormalRangeUnchanged(t *testing.T) {
	t.Parallel()
	cases := []struct {
		current, limit int64
		pct            int
		want           bool
	}{
		{800_000_000, 1_000_000_000, 80, true},
		{799_999_999, 1_000_000_000, 80, false},
		{1_000_000_000, 1_000_000_000, 100, true},
		{0, 100, 80, false},
		{50, 100, 50, true},
	}
	for _, c := range cases {
		if got := percentReached(c.current, c.limit, c.pct); got != c.want {
			t.Errorf("percentReached(%d, %d, %d) = %v, want %v",
				c.current, c.limit, c.pct, got, c.want)
		}
	}
}
