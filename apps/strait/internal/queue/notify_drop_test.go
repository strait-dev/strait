package queue

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

// Phase 2 test for QueueNotifier drop counter. The wake channel is buffered
// to 1; the second notification must be dropped and counted.
func TestQueueNotifier_DroppedNotifications_BufferFull(t *testing.T) {
	t.Parallel()

	n := &QueueNotifier{
		channel: "test_chan",
		wake:    make(chan struct{}, 1),
	}
	// We access the private drop path by mimicking the select inside
	// listenLoop. This avoids a real pgx listen round-trip.
	ctx := context.Background()
	send := func() {
		select {
		case n.wake <- struct{}{}:
		default:
			atomic.AddUint64(&n.droppedCount, 1)
			_ = ctx
		}
	}

	// First send succeeds (buffer empty).
	send()
	// Next 256 sends all drop.
	for range 256 {
		send()
	}

	if got := n.DroppedNotifications(); got != 256 {
		t.Errorf("DroppedNotifications = %d, want 256", got)
	}
}

func TestQueueNotifier_DroppedNotifications_ConcurrentSends(t *testing.T) {
	t.Parallel()

	n := &QueueNotifier{
		channel: "test_chan",
		wake:    make(chan struct{}, 1),
	}

	var wg sync.WaitGroup
	const senders = 32
	const perSender = 64
	for range senders {
		wg.Go(func() {
			for range perSender {
				select {
				case n.wake <- struct{}{}:
				default:
					// Match production code path: atomic increment.
					atomic.AddUint64(&n.droppedCount, 1)
				}
			}
		})
	}
	wg.Wait()

	// Total sends = senders * perSender = 2048. The wake channel is never
	// drained in this test, so it holds at most 1 element. Every send
	// beyond the first must be counted as dropped. Allow slack for the
	// buffer element plus scheduler-induced dropping.
	total := uint64(senders * perSender)
	dropped := n.DroppedNotifications()
	accepted := total - dropped
	if accepted > 1 {
		t.Errorf("more than 1 accepted into a buffer=1 channel: accepted=%d dropped=%d", accepted, dropped)
	}
	if dropped == 0 {
		t.Errorf("expected some notifications to be dropped, got 0")
	}
}
