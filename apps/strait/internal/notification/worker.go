package notification

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// Worker polls for pending notification deliveries and dispatches them.
type Worker struct {
	store   store.NotificationStore
	senders map[string]ChannelSender
	ticker  *time.Ticker
	done    chan struct{}
}

// NewWorker creates a notification delivery worker.
func NewWorker(ns store.NotificationStore, client *http.Client) *Worker {
	return &Worker{
		store: ns,
		senders: map[string]ChannelSender{
			domain.ChannelTypeSlack:   NewSlackSender(client),
			domain.ChannelTypeDiscord: NewDiscordSender(client),
			domain.ChannelTypeWebhook: NewWebhookSender(client),
		},
		done: make(chan struct{}),
	}
}

// Start begins the background polling loop.
func (w *Worker) Start(ctx context.Context) {
	w.ticker = time.NewTicker(30 * time.Second)
	go w.run(ctx)
}

// Stop halts the background polling loop.
func (w *Worker) Stop() {
	if w.ticker != nil {
		w.ticker.Stop()
	}
	close(w.done)
}

func (w *Worker) run(ctx context.Context) {
	// Process once immediately on start.
	w.process(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.done:
			return
		case <-w.ticker.C:
			w.process(ctx)
		}
	}
}

func (w *Worker) process(ctx context.Context) {
	deliveries, err := w.store.ListPendingNotificationDeliveries(ctx, 50)
	if err != nil {
		slog.Error("failed to list pending notification deliveries", "error", err)
		return
	}

	for i := range deliveries {
		d := &deliveries[i]
		if err := w.dispatch(ctx, d); err != nil {
			slog.Warn("notification delivery failed", "delivery_id", d.ID, "channel_id", d.ChannelID, "error", err)
		}
	}
}

func (w *Worker) dispatch(ctx context.Context, d *domain.NotificationDelivery) error {
	ch, err := w.store.GetNotificationChannel(ctx, d.ChannelID)
	if err != nil {
		d.Attempts++
		d.LastError = fmt.Sprintf("failed to get channel: %v", err)
		d.Status = "failed"
		return w.store.UpdateNotificationDelivery(ctx, d)
	}

	sender, ok := w.senders[ch.ChannelType]
	if !ok {
		d.Attempts++
		d.LastError = fmt.Sprintf("unsupported channel type: %s", ch.ChannelType)
		d.Status = "failed"
		return w.store.UpdateNotificationDelivery(ctx, d)
	}

	sendErr := sender.Send(ctx, ch, d)
	d.Attempts++

	if sendErr != nil {
		d.LastError = sendErr.Error()
		if d.Attempts >= d.MaxAttempts {
			d.Status = "failed"
		} else {
			// Exponential backoff: 30s, 120s, 480s, ...
			backoff := time.Duration(30*math.Pow(4, float64(d.Attempts-1))) * time.Second
			next := time.Now().Add(backoff)
			d.NextRetryAt = &next
		}
	} else {
		d.Status = "delivered"
		now := time.Now()
		d.DeliveredAt = &now
	}

	return w.store.UpdateNotificationDelivery(ctx, d)
}
