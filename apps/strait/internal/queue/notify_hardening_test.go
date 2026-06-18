package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests for notifier hardening state. Integration tests for
// reconnect behaviour live in notify_hardening_integration_test.go.

func TestQueueNotifier_InitialStateIsClean(t *testing.T) {
	n := NewQueueNotifier("postgres://unused", nil)
	assert.EqualValues(t, 0, n.Reconnects())
	assert.EqualValues(t, 0, n.ConnectionAge())
	assert.Equal(t, defaultInitialDelay, n.initialDelay)
	assert.Equal(t, defaultMaxReconnectDelay(), n.maxDelay)
	assert.Equal(t, defaultDegradedAfter(), n.degradedAfter)
	require.NotNil(t, n.connect)

	select {
	case <-n.Degraded():
		assert.Fail(t, "Degraded channel should not be closed on fresh notifier")
	default:
	}
}

func TestQueueNotifier_BackoffDelayUsesJitterAndMaxDelay(t *testing.T) {
	n := &QueueNotifier{
		initialDelay: time.Second,
		maxDelay:     defaultMaxReconnectDelay(),
	}

	first := n.backoffDelay(0)
	require.GreaterOrEqual(t, first, 750*time.Millisecond)
	require.LessOrEqual(t, first, 1250*time.Millisecond)

	capped := n.backoffDelay(10)
	require.GreaterOrEqual(t, capped, 22*time.Second)
	require.LessOrEqual(t, capped, 38*time.Second)
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

func TestQueueNotifierRunHandlesCancellationNilErrorReconnectAndDegraded(t *testing.T) {
	t.Run("context canceled before listen", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var connects int
		n := NewQueueNotifier("postgres://unused", nil)
		n.connect = func(context.Context, string) (queueNotifyConn, error) {
			connects++
			return nil, errors.New("unexpected connect")
		}

		n.Run(ctx)

		require.Zero(t, connects)
	})

	t.Run("nil listen error exits", func(t *testing.T) {
		n := NewQueueNotifier("postgres://unused", nil)
		n.listen = func(context.Context) (bool, error) {
			return false, nil
		}

		n.Run(context.Background())

		require.Zero(t, n.Reconnects())
	})

	t.Run("successful connection failure records reconnect", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var calls int
		n := NewQueueNotifier("postgres://unused", nil)
		n.initialDelay = time.Nanosecond
		n.maxDelay = time.Nanosecond
		n.listen = func(context.Context) (bool, error) {
			calls++
			if calls == 1 {
				return true, errors.New("wait failed")
			}
			cancel()
			return false, context.Canceled
		}

		n.Run(ctx)

		require.EqualValues(t, 1, n.Reconnects())
	})

	t.Run("extended failed listen marks degraded", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		n := NewQueueNotifier("postgres://unused", nil)
		n.initialDelay = time.Nanosecond
		n.maxDelay = time.Nanosecond
		n.degradedAfter = time.Nanosecond
		n.listen = func(context.Context) (bool, error) {
			return false, errors.New("connect failed")
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			n.Run(ctx)
		}()

		select {
		case <-n.Degraded():
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for degraded notifier")
		}
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for notifier shutdown")
		}
	})
}

func TestQueueNotifierListenLoopHandlesConnectListenWaitAndNotifications(t *testing.T) {
	t.Run("connect error", func(t *testing.T) {
		wantErr := errors.New("connect failed")
		n := NewQueueNotifier("postgres://unused", nil)
		n.connect = func(context.Context, string) (queueNotifyConn, error) {
			return nil, wantErr
		}

		connected, err := n.listenLoop(context.Background())

		require.False(t, connected)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("listen error", func(t *testing.T) {
		wantErr := errors.New("listen failed")
		conn := &queueNotifyFakeConn{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, wantErr
			},
		}
		n := NewQueueNotifier("postgres://unused", nil)
		n.connect = func(context.Context, string) (queueNotifyConn, error) {
			return conn, nil
		}

		connected, err := n.listenLoop(context.Background())

		require.False(t, connected)
		require.ErrorIs(t, err, wantErr)
		require.True(t, conn.closed)
	})

	t.Run("wait nil wrong channel delivered dropped and error", func(t *testing.T) {
		wantErr := errors.New("wait failed")
		conn := &queueNotifyFakeConn{
			notifications: []*pgconn.Notification{
				nil,
				{Channel: "other"},
				{Channel: QueueWakeChannel},
				{Channel: QueueWakeChannel},
			},
			waitErr: wantErr,
		}
		n := NewQueueNotifier("postgres://unused", nil)
		n.metrics = nil
		n.connect = func(context.Context, string) (queueNotifyConn, error) {
			return conn, nil
		}

		connected, err := n.listenLoop(context.Background())

		require.True(t, connected)
		require.ErrorIs(t, err, wantErr)
		require.Equal(t, "LISTEN "+QueueWakeChannel, conn.listenSQL)
		require.True(t, conn.closed)
		require.EqualValues(t, 1, n.DroppedNotifications())
		select {
		case <-n.Wake():
		default:
			t.Fatal("expected one delivered wake")
		}
	})
}

type queueNotifyFakeConn struct {
	execFn        func(context.Context, string, ...any) (pgconn.CommandTag, error)
	waitFn        func(context.Context) (*pgconn.Notification, error)
	notifications []*pgconn.Notification
	waitErr       error
	listenSQL     string
	closed        bool
}

func (c *queueNotifyFakeConn) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	c.listenSQL = sql
	if c.execFn != nil {
		return c.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (c *queueNotifyFakeConn) WaitForNotification(ctx context.Context) (*pgconn.Notification, error) {
	if c.waitFn != nil {
		return c.waitFn(ctx)
	}
	if len(c.notifications) == 0 {
		return nil, c.waitErr
	}
	notification := c.notifications[0]
	c.notifications = c.notifications[1:]
	return notification, nil
}

func (c *queueNotifyFakeConn) Close(context.Context) error {
	c.closed = true
	return nil
}
