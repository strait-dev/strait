package queue

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

const QueueWakeChannel = "strait_queue_wake"

// reconnectBackoff defaults.
const (
	defaultInitialDelay = time.Second
	defaultMaxDelay     = 30 * time.Second
)

type QueueNotifier struct {
	databaseURL  string
	channel      string
	wake         chan struct{}
	initialDelay time.Duration
	maxDelay     time.Duration
	logger       *slog.Logger
	metrics      *QueueMetrics
	droppedCount uint64
}

func NewQueueNotifier(databaseURL string, logger *slog.Logger) *QueueNotifier {
	if logger == nil {
		logger = slog.Default()
	}
	m, _ := Metrics() // nil on error; all accesses are nil-guarded

	return &QueueNotifier{
		databaseURL:  databaseURL,
		channel:      QueueWakeChannel,
		wake:         make(chan struct{}, 1),
		initialDelay: defaultInitialDelay,
		maxDelay:     defaultMaxDelay,
		logger:       logger,
		metrics:      m,
	}
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
	for {
		if err := ctx.Err(); err != nil {
			return
		}

		connected, err := n.listenLoop(ctx)
		if err == nil || ctx.Err() != nil {
			return
		}

		// Reset backoff after a successful connection that was later lost,
		// so that transient disconnects do not accumulate delay.
		if connected {
			attempt = 0
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
	}
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
	conn, err := pgx.Connect(ctx, n.databaseURL)
	if err != nil {
		return false, fmt.Errorf("connect notifier: %w", err)
	}
	defer conn.Close(context.Background())

	listenSQL := fmt.Sprintf("LISTEN %s", n.channel)
	if _, err := conn.Exec(ctx, listenSQL); err != nil {
		return false, fmt.Errorf("listen on %s: %w", n.channel, err)
	}

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
		default:
			atomic.AddUint64(&n.droppedCount, 1)
			if n.metrics != nil {
				n.metrics.NotifyDropped.Add(ctx, 1)
			}
			n.logger.Debug("queue wake notification dropped (channel full)", "channel", n.channel)
		}
	}
}
