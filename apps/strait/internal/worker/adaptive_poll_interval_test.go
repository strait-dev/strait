package worker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAdaptivePoll_BaseOnFreshStart(t *testing.T) {
	a := NewAdaptivePollInterval(5*time.Second, 200*time.Millisecond, 30*time.Second)
	assert.Equal(t,
		5*time.
			Second,

		a.Next())

}

func TestAdaptivePoll_BacksOffOnEmpty(t *testing.T) {
	a := NewAdaptivePollInterval(1*time.Second, 200*time.Millisecond, 30*time.Second)
	prev := a.Next()
	for range 4 {
		a.ObserveEmpty()
		cur := a.Next()
		assert.GreaterOrEqual(t,
			cur,

			prev)

		prev = cur
	}
	assert.GreaterOrEqual(t,
		prev,

		8*time.Second)

}

func TestAdaptivePoll_CappedAtMax(t *testing.T) {
	a := NewAdaptivePollInterval(1*time.Second, 200*time.Millisecond, 5*time.Second)
	for range 20 {
		a.ObserveEmpty()
	}
	assert.LessOrEqual(t, a.
		Next(), 5*time.Second)

}

func TestAdaptivePoll_FloorAtMin(t *testing.T) {
	a := NewAdaptivePollInterval(5*time.Second, 500*time.Millisecond, 30*time.Second)
	a.ObserveDepth(100000)
	assert.GreaterOrEqual(t,
		a.
			Next(), 500*time.Millisecond)

	// huge depth → wants tiny interval

}

func TestAdaptivePoll_DepthShortensInterval(t *testing.T) {
	a := NewAdaptivePollInterval(5*time.Second, 200*time.Millisecond, 30*time.Second)
	a.ObserveDepth(1000)
	if d := a.Next(); d >= 5*time.Second {
		assert.Failf(t, "test failure",

			"depth=1000 should shorten, got %v", d)
	}
}

func TestAdaptivePoll_ClaimResetsEmpty(t *testing.T) {
	a := NewAdaptivePollInterval(1*time.Second, 200*time.Millisecond, 30*time.Second)
	for range 4 {
		a.ObserveEmpty()
	}
	before := a.Next()
	a.ObserveClaim(3)
	after := a.Next()
	assert.False(t,
		after >=
			before,
	)
	assert.Equal(t,
		1*time.
			Second,

		after)

}

func TestAdaptivePoll_Disable(t *testing.T) {
	a := NewAdaptivePollInterval(2*time.Second, 200*time.Millisecond, 30*time.Second)
	for range 10 {
		a.ObserveEmpty()
	}
	a.Disable()
	assert.Equal(t,
		2*time.
			Second,

		a.Next())

}

func TestAdaptivePoll_ObserveClaimZero(t *testing.T) {
	a := NewAdaptivePollInterval(1*time.Second, 200*time.Millisecond, 30*time.Second)
	a.ObserveClaim(0)
	assert.EqualValues(t, 1, a.emptyCount)

	// zero claim == empty

}

// FuzzAdaptivePoll_MonotonicBounds asserts the result is always within
// [min, max] for any observe sequence.
func FuzzAdaptivePoll_Bounds(f *testing.F) {
	f.Add(uint8(0), uint8(0))
	f.Add(uint8(10), uint8(100))
	f.Fuzz(func(t *testing.T, empties, depth uint8) {
		a := NewAdaptivePollInterval(1*time.Second, 100*time.Millisecond, 10*time.Second)
		for range min(int(empties), 32) {
			a.ObserveEmpty()
		}
		a.ObserveDepth(int64(depth) * 100)
		d := a.Next()
		assert.False(t,
			d < 100*
				time.
					Millisecond || d > 10*time.Second)

	})
}
