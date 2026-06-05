package queue

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests for notifier hardening state. Integration tests for
// reconnect behaviour live in notify_hardening_integration_test.go.

func TestQueueNotifier_InitialStateIsClean(t *testing.T) {
	n := NewQueueNotifier("postgres://unused", nil)
	assert.EqualValues(t, 0, n.Reconnects())
	assert.EqualValues(t, 0, n.ConnectionAge())

	select {
	case <-n.Degraded():
		assert.Fail(t, "Degraded channel should not be closed on fresh notifier")
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
		assert.Fail(t, "Degraded channel should be closed after markDegraded")
	}
}

func TestQueueNotifier_DegradedResetAllowsReuse(t *testing.T) {
	n := NewQueueNotifier("postgres://unused", nil)
	n.markDegraded()
	select {
	case <-n.Degraded():
	default:
		require.Fail(t, "expected degraded after markDegraded")
	}

	n.DegradedReset()

	select {
	case <-n.Degraded():
		assert.Fail(t, "Degraded should be reset; channel should be open")
	default:
	}

	// And markDegraded should still work after the reset.
	n.markDegraded()
	select {
	case <-n.Degraded():
	default:
		assert.Fail(t, "markDegraded after reset should close the channel")
	}
}

func TestQueueNotifier_DegradedResetNoOpWhenNotDegraded(t *testing.T) {
	n := NewQueueNotifier("postgres://unused", nil)
	n.DegradedReset() // should be a no-op and leave the channel open

	select {
	case <-n.Degraded():
		assert.Fail(t, "Degraded channel should still be open after no-op reset")
	default:
	}
}

func TestDisconnectStartForFailedListenResetsAfterReconnect(t *testing.T) {
	oldOutage := time.Now().Add(-time.Hour)
	newOutage := time.Now()

	got := disconnectStartForFailedListen(oldOutage, true, newOutage)
	require.True(t, got.Equal(newOutage))

	stillDown := disconnectStartForFailedListen(oldOutage, false, newOutage)
	require.True(t, stillDown.Equal(oldOutage))
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
	assert.EqualValues(t, numGoroutines*
		100, n.
		Reconnects())
}

// incReconnects mirrors the atomic increment the production Run loop
// performs. Keeping the test package-internal so it can touch the field.
func incReconnects(n *QueueNotifier) {
	atomic.AddUint64(&n.reconnects, 1)
}

// TestQueueNotifier_DegradedConcurrency exercises the Degraded / markDegraded /
// DegradedReset paths under heavy concurrent access. Must pass under -race.
func TestQueueNotifier_DegradedConcurrency(t *testing.T) {
	n := NewQueueNotifier("postgres://unused", nil)

	const readers = 128
	const iterations = 10_000

	var wg sync.WaitGroup

	for range readers {
		wg.Go(func() {
			for range iterations {
				ch := n.Degraded()
				select {
				case <-ch:
				default:
				}
			}
		})
	}

	wg.Go(func() {
		for range iterations {
			n.markDegraded()
			n.DegradedReset()
		}
	})

	wg.Wait()
}

func TestDegradedRecoveryReArmsWithFreshChannel(t *testing.T) {
	n := NewQueueNotifier("postgres://unused", nil)

	// Initially the channel is open.
	ch1 := n.Degraded()
	select {
	case <-ch1:
		require.Fail(t, "channel should be open on fresh notifier")
	default:
	}

	// Enter degraded mode.
	n.MarkDegradedForTest()
	select {
	case <-n.Degraded():
	default:
		require.Fail(t, "channel should be closed after markDegraded")
	}

	// Reset simulates reconnect.
	n.DegradedReset()
	ch2 := n.Degraded()
	select {
	case <-ch2:
		require.Fail(t, "fresh channel after reset should be open")
	default:
	}

	// ch1 is still the old closed channel; ch2 is the new open one.
	select {
	case <-ch1:
		// expected: old channel stays closed
	default:
		assert.Fail(t, "old channel should remain closed after reset")
	}
}

func TestQueueNotifier_SuccessfulListenClearsDegradedImmediately(t *testing.T) {
	n := NewQueueNotifier("postgres://unused", nil)
	n.MarkDegradedForTest()
	old := n.Degraded()
	select {
	case <-old:
	default:
		require.Fail(t, "old degraded channel should be closed")
	}

	n.markListenConnected()
	fresh := n.Degraded()
	select {
	case <-fresh:
		require.Fail(t, "successful listen should replace degraded channel immediately")
	default:
	}
	require.NotEqual(t, 0, atomic.
		LoadInt64(&n.lastConnectedUnixNano))
}

// Verify that QueueNotifier satisfies the DegradedNotifier interface.
var _ DegradedNotifier = (*QueueNotifier)(nil)

func TestQueueNotifier_ConnectionAgeAfterSet(t *testing.T) {
	n := NewQueueNotifier("postgres://unused", nil)
	// Simulate a successful listenLoop.
	atomic.StoreInt64(&n.lastConnectedUnixNano, time.Now().UnixNano())
	time.Sleep(5 * time.Millisecond)
	assert.GreaterOrEqual(t, n.
		ConnectionAge(), 5*
		time.
			Millisecond)
}
