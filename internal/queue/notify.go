package queue

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

const QueueWakeChannel = "strait_queue_wake"

type QueueNotifier struct {
	databaseURL    string
	channel        string
	wake           chan struct{}
	reconnectDelay time.Duration
	logger         *slog.Logger
}

func NewQueueNotifier(databaseURL string, logger *slog.Logger) *QueueNotifier {
	if logger == nil {
		logger = slog.Default()
	}

	return &QueueNotifier{
		databaseURL:    databaseURL,
		channel:        QueueWakeChannel,
		wake:           make(chan struct{}, 1),
		reconnectDelay: time.Second,
		logger:         logger,
	}
}

func (n *QueueNotifier) Wake() <-chan struct{} {
	return n.wake
}

func (n *QueueNotifier) Run(ctx context.Context) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}

		err := n.listenLoop(ctx)
		if err == nil || ctx.Err() != nil {
			return
		}

		n.logger.Warn("queue notifier disconnected", "channel", n.channel, "error", err)

		select {
		case <-ctx.Done():
			return
		case <-time.After(n.reconnectDelay):
		}
	}
}

func (n *QueueNotifier) listenLoop(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, n.databaseURL)
	if err != nil {
		return fmt.Errorf("connect notifier: %w", err)
	}
	defer conn.Close(context.Background())

	listenSQL := fmt.Sprintf("LISTEN %s", n.channel)
	if _, err := conn.Exec(ctx, listenSQL); err != nil {
		return fmt.Errorf("listen on %s: %w", n.channel, err)
	}

	n.logger.Info("queue notifier listening", "channel", n.channel)

	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			return fmt.Errorf("wait for notification: %w", err)
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
		}
	}
}
