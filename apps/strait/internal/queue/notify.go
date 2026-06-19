package queue

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const QueueWakeChannel = "strait_queue_wake"

// reconnectBackoff defaults.
const defaultInitialDelay = time.Second

func defaultMaxReconnectDelay() time.Duration { return 30 * time.Second }

func defaultDegradedAfter() time.Duration { return 30 * time.Second }

// DegradedNotifier provides a channel that closes when the queue notifier
// enters degraded mode. Callers should re-invoke Degraded() after each
// recovery to obtain the fresh channel.
type DegradedNotifier interface {
	Degraded() <-chan struct{}
}

type QueueNotifier struct {
	databaseURL   string
	channel       string
	wake          chan struct{}
	initialDelay  time.Duration
	maxDelay      time.Duration
	degradedAfter time.Duration
	connect       queueNotifyConnector
	listen        func(context.Context) (bool, error)
	logger        *slog.Logger
	metrics       *QueueMetrics
	droppedCount  uint64
	reconnects    uint64
	// Time of the most recent successful LISTEN establishment.
	// Zero while no connection is live. Stored as UnixNano for lock-free
	// reads.
	lastConnectedUnixNano int64
	// degradedMu protects degradedCh against concurrent read/write races
	// between Degraded() (readers) and markDegraded()/DegradedReset() (writers).
	degradedMu sync.RWMutex
	// Aggressive-polling signal channel. When the notifier has been
	// disconnected longer than the configured threshold, this channel is
	// closed; workers that select on it can shorten their poll interval.
	degradedCh chan struct{}
	// Degraded reset guard so we don't re-create the channel after every
	// transient hiccup.
	degradedOnce atomic.Bool
}

type queueNotifyConn interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	WaitForNotification(context.Context) (*pgconn.Notification, error)
	Close(context.Context) error
}

type queueNotifyConnector func(context.Context, string) (queueNotifyConn, error)

func defaultQueueNotifyConnector(ctx context.Context, databaseURL string) (queueNotifyConn, error) {
	return pgx.Connect(ctx, databaseURL)
}

func NewQueueNotifier(databaseURL string, logger *slog.Logger) *QueueNotifier {
	if logger == nil {
		logger = slog.Default()
	}
	m, _ := Metrics() // nil on error; all accesses are nil-guarded

	return &QueueNotifier{
		databaseURL:   databaseURL,
		channel:       QueueWakeChannel,
		wake:          make(chan struct{}, 1),
		initialDelay:  defaultInitialDelay,
		maxDelay:      defaultMaxReconnectDelay(),
		degradedAfter: defaultDegradedAfter(),
		connect:       defaultQueueNotifyConnector,
		logger:        logger,
		metrics:       m,
		degradedCh:    make(chan struct{}),
	}
}

// Reconnects returns the cumulative number of LISTEN reconnects. Exposed
// for tests.
func (n *QueueNotifier) Reconnects() uint64 {
	return atomic.LoadUint64(&n.reconnects)
}

// ConnectionAge returns how long the current LISTEN connection has been
// live, or 0 if the notifier is disconnected.
func (n *QueueNotifier) ConnectionAge() time.Duration {
	t := atomic.LoadInt64(&n.lastConnectedUnixNano)
	if t == 0 {
		return 0
	}
	return time.Since(time.Unix(0, t))
}

// Degraded returns a channel that is closed once the notifier has been
// disconnected longer than the aggressive-polling threshold. Workers can
// select on it to shorten their poll interval while the notifier is out.
// Once the notifier successfully reconnects, callers should call
// DegradedReset to get a fresh channel.
func (n *QueueNotifier) Degraded() <-chan struct{} {
	n.degradedMu.RLock()
	ch := n.degradedCh
	n.degradedMu.RUnlock()
	return ch
}

// MarkDegradedForTest is an exported wrapper around markDegraded for use by
// external test packages. Production callers should not use this.
func (n *QueueNotifier) MarkDegradedForTest() { n.markDegraded() }

// markDegraded closes degradedCh so receivers wake up. Idempotent.
func (n *QueueNotifier) markDegraded() {
	n.degradedMu.Lock()
	defer n.degradedMu.Unlock()
	if n.degradedOnce.CompareAndSwap(false, true) {
		close(n.degradedCh)
	}
}

// DegradedReset replaces the degraded channel with a fresh one after a
// successful reconnect. Safe to call concurrently; reuses the same field.
func (n *QueueNotifier) DegradedReset() {
	n.degradedMu.Lock()
	defer n.degradedMu.Unlock()
	if n.degradedOnce.Load() {
		n.degradedCh = make(chan struct{})
		n.degradedOnce.Store(false)
	}
}

func (n *QueueNotifier) markListenConnected() {
	atomic.StoreInt64(&n.lastConnectedUnixNano, time.Now().UnixNano())
	n.DegradedReset()
}

// DroppedNotifications returns the total wake notifications dropped because
// the wake channel was full. Exposed for tests.
func (n *QueueNotifier) DroppedNotifications() uint64 {
	return atomic.LoadUint64(&n.droppedCount)
}

func (n *QueueNotifier) Wake() <-chan struct{} {
	return n.wake
}

func (n *QueueNotifier) Run(ctx context.Context) {
	var attempt int
	// If a disconnect lasts longer than this, the notifier flips into
	// degraded mode and closes Degraded() so workers can tighten their
	// poll interval.
	degradedThreshold := n.degradedAfter
	if degradedThreshold <= 0 {
		degradedThreshold = defaultDegradedAfter()
	}
	var disconnectStart time.Time
	listen := n.listen
	if listen == nil {
		listen = n.listenLoop
	}
	for {
		if err := ctx.Err(); err != nil {
			return
		}

		connected, err := listen(ctx)
		if err == nil || ctx.Err() != nil {
			return
		}

		// Reset backoff after a successful connection that was later lost,
		// so that transient disconnects do not accumulate delay.
		if connected {
			attempt = 0
			atomic.AddUint64(&n.reconnects, 1)
			if n.metrics != nil {
				n.metrics.NotifyReconnects.Add(ctx, 1)
			}
			n.DegradedReset()
		}

		now := time.Now()
		disconnectStart = disconnectStartForFailedListen(disconnectStart, connected, now)
		if !disconnectStart.Equal(now) && time.Since(disconnectStart) > degradedThreshold {
			n.markDegraded()
			n.logger.Warn("queue notifier degraded: aggressive polling engaged",
				"disconnected_for", time.Since(disconnectStart),
			)
		}

		delay := n.backoffDelay(attempt)
		n.logger.Warn("queue notifier disconnected",
			"channel", n.channel, "error", err,
			"reconnect_delay", delay, "attempt", attempt+1,
		)
		attempt++

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		// If the loop successfully connects on the next iteration it
		// will clear disconnectStart through listenLoop's first success.
	}
}

func disconnectStartForFailedListen(previous time.Time, connected bool, now time.Time) time.Time {
	if connected || previous.IsZero() {
		return now
	}
	return previous
}

// backoffDelay returns an exponential backoff duration with jitter for
// the given attempt, capped at maxDelay. Resets to initialDelay on
// attempt 0.
func (n *QueueNotifier) backoffDelay(attempt int) time.Duration {
	base := float64(n.initialDelay) * math.Pow(2, float64(attempt))
	if base > float64(n.maxDelay) {
		base = float64(n.maxDelay)
	}
	// Add jitter: 75% - 125% of base. Weak randomness is fine for backoff timing.
	jitter := 0.75 + rand.Float64()*0.5 //nolint:gosec // G404: jitter does not need cryptographic randomness
	return time.Duration(base * jitter)
}

// listenLoop connects to Postgres and listens for notifications. It returns
// a boolean indicating whether the connection was successfully established
// (true) and the error that caused the loop to exit.
func (n *QueueNotifier) listenLoop(ctx context.Context) (connected bool, err error) {
	connect := n.connect
	if connect == nil {
		connect = defaultQueueNotifyConnector
	}
	conn, err := connect(ctx, n.databaseURL)
	if err != nil {
		return false, fmt.Errorf("connect notifier: %w", err)
	}
	defer conn.Close(context.Background())

	listenSQL := fmt.Sprintf("LISTEN %s", n.channel)
	if _, err := conn.Exec(ctx, listenSQL); err != nil {
		return false, fmt.Errorf("listen on %s: %w", n.channel, err)
	}

	n.markListenConnected()
	n.logger.Info("queue notifier listening", "channel", n.channel)

	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			return true, fmt.Errorf("wait for notification: %w", err)
		}
		if notification == nil {
			continue
		}
		if notification.Channel != n.channel {
			continue
		}

		select {
		case n.wake <- struct{}{}:
			if n.metrics != nil {
				n.metrics.NotifyWakeDelivered.Add(ctx, 1)
			}
		default:
			atomic.AddUint64(&n.droppedCount, 1)
			if n.metrics != nil {
				n.metrics.NotifyDropped.Add(ctx, 1)
			}
			n.logger.Debug("queue wake notification dropped (channel full)", "channel", n.channel)
		}
	}
}
