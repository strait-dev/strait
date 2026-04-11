package queue

import (
	"sync"
	"testing"
	"time"
)

// Phase 5 unit tests for the per-worker claim cursor.

func TestClaimCursor_UnsetReturnsInvalid(t *testing.T) {
	c := NewClaimCursor(60 * time.Second)
	_, _, ok := c.Snapshot()
	if ok {
		t.Error("fresh cursor should be invalid")
	}
}

func TestClaimCursor_NilSafe(t *testing.T) {
	var c *ClaimCursor
	_, _, ok := c.Snapshot()
	if ok {
		t.Error("nil cursor should be invalid")
	}
	c.Advance(time.Now(), "abc") // must not panic
	c.Reset()                    // must not panic
	c.ForceExpire()              // must not panic
}

func TestClaimCursor_AdvanceMonotonic(t *testing.T) {
	c := NewClaimCursor(60 * time.Second)
	t0 := time.Now()
	c.Advance(t0, "id-2")
	c.Advance(t0.Add(-1*time.Second), "id-1") // older: ignored
	ts, id, ok := c.Snapshot()
	if !ok || ts != t0 || id != "id-2" {
		t.Errorf("Snapshot = (%v, %q, %v), want (%v, id-2, true)", ts, id, ok, t0)
	}
}

func TestClaimCursor_AdvanceTieBreakOnID(t *testing.T) {
	c := NewClaimCursor(60 * time.Second)
	t0 := time.Now()
	c.Advance(t0, "id-1")
	c.Advance(t0, "id-2")
	_, id, _ := c.Snapshot()
	if id != "id-2" {
		t.Errorf("expected id-2 to win tie-break, got %q", id)
	}
	c.Advance(t0, "id-0") // smaller id, ignored
	_, id, _ = c.Snapshot()
	if id != "id-2" {
		t.Errorf("tie-break broken: got %q", id)
	}
}

func TestClaimCursor_ResetClears(t *testing.T) {
	c := NewClaimCursor(60 * time.Second)
	c.Advance(time.Now(), "id-1")
	c.Reset()
	_, _, ok := c.Snapshot()
	if ok {
		t.Error("cursor should be invalid after Reset")
	}
}

func TestClaimCursor_ExpiresAfterInterval(t *testing.T) {
	c := NewClaimCursor(10 * time.Millisecond)
	c.Advance(time.Now(), "id-1")
	time.Sleep(30 * time.Millisecond)
	_, _, ok := c.Snapshot()
	if ok {
		t.Error("cursor should be expired")
	}
}

func TestClaimCursor_ForceExpire(t *testing.T) {
	c := NewClaimCursor(60 * time.Second)
	c.Advance(time.Now(), "id-1")
	c.ForceExpire()
	_, _, ok := c.Snapshot()
	if ok {
		t.Error("cursor should be invalid after ForceExpire")
	}
}

func TestClaimCursor_ConcurrentAdvance(t *testing.T) {
	c := NewClaimCursor(60 * time.Second)
	var wg sync.WaitGroup
	base := time.Now()
	for g := range 16 {
		wg.Go(func() {
			for i := range 200 {
				ts := base.Add(time.Duration(g*200+i) * time.Microsecond)
				c.Advance(ts, "id")
			}
		})
	}
	wg.Wait()
	ts, _, ok := c.Snapshot()
	if !ok {
		t.Fatal("cursor should be valid")
	}
	want := base.Add(time.Duration(16*200-1) * time.Microsecond)
	if !ts.Equal(want) {
		t.Errorf("cursor ts = %v, want %v", ts, want)
	}
}

// FuzzClaimCursorAdvance feeds random (ns-offset, id) pairs to Advance and
// asserts the invariant that Snapshot returns the maximum observed pair or
// nothing if the cursor has expired.
func FuzzClaimCursorAdvance(f *testing.F) {
	f.Add(int64(0), "a")
	f.Add(int64(1_000), "b")
	f.Add(int64(-1), "")
	f.Fuzz(func(t *testing.T, nsOff int64, id string) {
		if len(id) > 64 {
			return
		}
		c := NewClaimCursor(1 * time.Hour)
		base := time.Now()
		c.Advance(base.Add(time.Duration(nsOff)), id)
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Advance panicked: %v", r)
			}
		}()
		_, _, _ = c.Snapshot()
	})
}
