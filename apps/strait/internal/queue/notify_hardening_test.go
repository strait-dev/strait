package queue

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Phase 10 unit tests for notifier hardening state. Integration tests for
// reconnect behaviour live in notify_hardening_integration_test.go.

func TestQueueNotifier_InitialStateIsClean(t *testing.T) {
	n := NewQueueNotifier("postgres://unused", nil)
	if n.Reconnects() != 0 {
		t.Errorf("reconnects = %d, want 0", n.Reconnects())
	}
	if n.ConnectionAge() != 0 {
		t.Errorf("connection age on fresh notifier = %v, want 0", n.ConnectionAge())
	}
	select {
	case <-n.Degraded():
		t.Error("Degraded channel should not be closed on fresh notifier")
	default:
	}
}

func TestQueueNotifier_MarkDegradedIsIdempotent(t *testing.T) {
	n := NewQueueNotifier("postgres://unused", nil)
	n.markDegraded()
	n.markDegraded() // must not panic (double close would)

	select {
	case <-n.Degraded():
		// expected
	default:
		t.Error("Degraded channel should be closed after markDegraded")
	}
}

func TestQueueNotifier_DegradedResetAllowsReuse(t *testing.T) {
	n := NewQueueNotifier("postgres://unused", nil)
	n.markDegraded()
	select {
	case <-n.Degraded():
	default:
		t.Fatal("expected degraded after markDegraded")
	}

	n.DegradedReset()

	select {
	case <-n.Degraded():
		t.Error("Degraded should be reset; channel should be open")
	default:
	}

	// And markDegraded should still work after the reset.
	n.markDegraded()
	select {
	case <-n.Degraded():
	default:
		t.Error("markDegraded after reset should close the channel")
	}
}

func TestQueueNotifier_DegradedResetNoOpWhenNotDegraded(t *testing.T) {
	n := NewQueueNotifier("postgres://unused", nil)
	n.DegradedReset() // should be a no-op and leave the channel open

	select {
	case <-n.Degraded():
		t.Error("Degraded channel should still be open after no-op reset")
	default:
	}
}

func TestQueueNotifier_ReconnectCountIsAtomic(t *testing.T) {
	n := NewQueueNotifier("postgres://unused", nil)

	var wg sync.WaitGroup
	const numGoroutines = 32
	for range numGoroutines {
		wg.Go(func() {
			for range 100 {
				// Simulate what Run() does on a reconnect.
				incReconnects(n)
			}
		})
	}
	wg.Wait()

	if got := n.Reconnects(); got != numGoroutines*100 {
		t.Errorf("reconnects = %d, want %d", got, numGoroutines*100)
	}
}

// incReconnects mirrors the atomic increment the production Run loop
// performs. Keeping the test package-internal so it can touch the field.
func incReconnects(n *QueueNotifier) {
	atomic.AddUint64(&n.reconnects, 1)
}

func TestQueueNotifier_ConnectionAgeAfterSet(t *testing.T) {
	n := NewQueueNotifier("postgres://unused", nil)
	// Simulate a successful listenLoop.
	atomic.StoreInt64(&n.lastConnectedUnixNano, time.Now().UnixNano())
	time.Sleep(5 * time.Millisecond)
	if age := n.ConnectionAge(); age < 5*time.Millisecond {
		t.Errorf("age = %v, want >= 5ms", age)
	}
}
