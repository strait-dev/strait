package worker

import (
	"testing"
	"time"
)

func TestAdaptivePoll_BaseOnFreshStart(t *testing.T) {
	a := NewAdaptivePollInterval(5*time.Second, 200*time.Millisecond, 30*time.Second)
	if d := a.Next(); d != 5*time.Second {
		t.Errorf("initial = %v, want base 5s", d)
	}
}

func TestAdaptivePoll_BacksOffOnEmpty(t *testing.T) {
	a := NewAdaptivePollInterval(1*time.Second, 200*time.Millisecond, 30*time.Second)
	prev := a.Next()
	for i := 0; i < 4; i++ {
		a.ObserveEmpty()
		cur := a.Next()
		if cur < prev {
			t.Errorf("empty %d: went backwards %v -> %v", i, prev, cur)
		}
		prev = cur
	}
	if prev < 8*time.Second {
		t.Errorf("after 4 empties = %v, want >= 8s", prev)
	}
}

func TestAdaptivePoll_CappedAtMax(t *testing.T) {
	a := NewAdaptivePollInterval(1*time.Second, 200*time.Millisecond, 5*time.Second)
	for i := 0; i < 20; i++ {
		a.ObserveEmpty()
	}
	if d := a.Next(); d > 5*time.Second {
		t.Errorf("cap broken: %v", d)
	}
}

func TestAdaptivePoll_FloorAtMin(t *testing.T) {
	a := NewAdaptivePollInterval(5*time.Second, 500*time.Millisecond, 30*time.Second)
	a.ObserveDepth(100000) // huge depth → wants tiny interval
	if d := a.Next(); d < 500*time.Millisecond {
		t.Errorf("floor broken: %v", d)
	}
}

func TestAdaptivePoll_DepthShortensInterval(t *testing.T) {
	a := NewAdaptivePollInterval(5*time.Second, 200*time.Millisecond, 30*time.Second)
	a.ObserveDepth(1000)
	if d := a.Next(); d >= 5*time.Second {
		t.Errorf("depth=1000 should shorten, got %v", d)
	}
}

func TestAdaptivePoll_ClaimResetsEmpty(t *testing.T) {
	a := NewAdaptivePollInterval(1*time.Second, 200*time.Millisecond, 30*time.Second)
	for i := 0; i < 4; i++ {
		a.ObserveEmpty()
	}
	before := a.Next()
	a.ObserveClaim(3)
	after := a.Next()
	if after >= before {
		t.Errorf("claim did not reset empty: before=%v after=%v", before, after)
	}
	if after != 1*time.Second {
		t.Errorf("after claim = %v, want base 1s", after)
	}
}

func TestAdaptivePoll_Disable(t *testing.T) {
	a := NewAdaptivePollInterval(2*time.Second, 200*time.Millisecond, 30*time.Second)
	for i := 0; i < 10; i++ {
		a.ObserveEmpty()
	}
	a.Disable()
	if d := a.Next(); d != 2*time.Second {
		t.Errorf("disabled should return base, got %v", d)
	}
}

func TestAdaptivePoll_ObserveClaimZero(t *testing.T) {
	a := NewAdaptivePollInterval(1*time.Second, 200*time.Millisecond, 30*time.Second)
	a.ObserveClaim(0) // zero claim == empty
	if a.emptyCount != 1 {
		t.Errorf("emptyCount = %d, want 1", a.emptyCount)
	}
}

// FuzzAdaptivePoll_MonotonicBounds asserts the result is always within
// [min, max] for any observe sequence.
func FuzzAdaptivePoll_Bounds(f *testing.F) {
	f.Add(uint8(0), uint8(0))
	f.Add(uint8(10), uint8(100))
	f.Fuzz(func(t *testing.T, empties, depth uint8) {
		a := NewAdaptivePollInterval(1*time.Second, 100*time.Millisecond, 10*time.Second)
		for i := uint8(0); i < empties && i < 32; i++ {
			a.ObserveEmpty()
		}
		a.ObserveDepth(int64(depth) * 100)
		d := a.Next()
		if d < 100*time.Millisecond || d > 10*time.Second {
			t.Errorf("out of bounds: %v", d)
		}
	})
}
