package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
)

// TestComponentTracker_WaitWithTimeout exercises the table-driven fast-exit,
// slow-but-in-window, and beyond-deadline paths. Uses a non-zero timeout so
// the default clamp isn't in play.
func TestComponentTracker_WaitWithTimeout(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		components   []func()
		timeout      time.Duration
		wantTimedOut int
	}{
		{
			name: "all_fast_exits",
			components: []func(){
				func() {},
				func() { time.Sleep(5 * time.Millisecond) },
			},
			timeout:      200 * time.Millisecond,
			wantTimedOut: 0,
		},
		{
			name: "one_past_deadline",
			components: []func(){
				func() {},
				func() { time.Sleep(300 * time.Millisecond) },
			},
			timeout:      80 * time.Millisecond,
			wantTimedOut: 1,
		},
		{
			name: "all_past_deadline",
			components: []func(){
				func() { time.Sleep(300 * time.Millisecond) },
				func() { time.Sleep(300 * time.Millisecond) },
			},
			timeout:      50 * time.Millisecond,
			wantTimedOut: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var wg conc.WaitGroup
			var tracker componentTracker

			for i, fn := range tc.components {
				name := tc.name + "-" + string(rune('a'+i))
				tracker.track(&wg, name, fn)
			}

			got := tracker.waitWithTimeout(context.Background(), tc.timeout)
			if got != tc.wantTimedOut {
				t.Fatalf("waitWithTimeout timed_out=%d, want %d", got, tc.wantTimedOut)
			}

			// Always wait for the aggregate WaitGroup so no goroutines leak
			// into the next subtest or the test harness.
			wg.Wait()
		})
	}
}

// TestComponentTracker_TrackCountsInvocations verifies track actually registers
// each component and that the fn body runs.
func TestComponentTracker_TrackCountsInvocations(t *testing.T) {
	t.Parallel()
	var wg conc.WaitGroup
	var tracker componentTracker
	var ran atomic.Int32

	for range 5 {
		tracker.track(&wg, "worker", func() { ran.Add(1) })
	}

	wg.Wait()
	if got := ran.Load(); got != 5 {
		t.Fatalf("ran=%d, want 5", got)
	}
	tracker.mu.Lock()
	items := len(tracker.items)
	tracker.mu.Unlock()
	if items != 5 {
		t.Fatalf("items=%d, want 5", items)
	}
}
