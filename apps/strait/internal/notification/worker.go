package notification

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	concpool "github.com/sourcegraph/conc/pool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Worker polls for pending notification deliveries and dispatches them.
type Worker struct {
	store             store.NotificationStore
	senders           map[string]ChannelSender
	ticker            *time.Ticker
	done              chan struct{}
	stopOnce          sync.Once
	deliveriesCounter metric.Int64Counter
}

const deliveryLeaseDuration time.Duration = 120_000_000_000

const (
	notificationClaimBatchSize   = 16
	notificationWorkerConcurrent = 8
)

// NewWorker creates a notification delivery worker.
func NewWorker(ns store.NotificationStore, client *http.Client, webhookOptions ...WebhookSenderOption) *Worker {
	return &Worker{
		store: ns,
		senders: map[string]ChannelSender{
			domain.ChannelTypeSlack:   NewSlackSender(client),
			domain.ChannelTypeDiscord: NewDiscordSender(client),
			domain.ChannelTypeWebhook: NewWebhookSender(client, webhookOptions...),
		},
		done: make(chan struct{}),
	}
}

// NewWorkerWithEmail creates a notification worker and registers the email
// sender when Resend credentials are configured.
func NewWorkerWithEmail(ns store.NotificationStore, client *http.Client, resendAPIKey, resendFromEmail string, webhookOptions ...WebhookSenderOption) *Worker {
	w := NewWorker(ns, client, webhookOptions...)
	if resendAPIKey == "" {
		return w
	}
	emailSender, err := NewEmailSender(resendAPIKey, resendFromEmail)
	if err != nil {
		slog.Warn("failed to initialize email notification sender", "error", err)
		return w
	}
	w.RegisterSender(domain.ChannelTypeEmail, emailSender)
	return w
}

// RegisterSender adds or replaces a channel sender for the given channel type.
func (w *Worker) RegisterSender(channelType string, sender ChannelSender) {
	w.senders[channelType] = sender
}

// HasSender reports whether a sender is registered for a channel type.
func (w *Worker) HasSender(channelType string) bool {
	_, ok := w.senders[channelType]
	return ok
}

// WithDeliveriesCounter attaches an OTel counter for tracking delivery outcomes.
func (w *Worker) WithDeliveriesCounter(c metric.Int64Counter) *Worker {
	w.deliveriesCounter = c
	return w
}

// Start begins the polling loop owned by this worker. The loop exits when the
// parent context is canceled or Stop closes done.
func (w *Worker) Start(ctx context.Context) {
	w.ticker = time.NewTicker(30 * time.Second)
	go w.run(ctx)
}

// Stop signals the polling loop to exit. It is safe to call multiple times.
func (w *Worker) Stop() {
	w.stopOnce.Do(func() {
		if w.ticker != nil {
			w.ticker.Stop()
		}
		close(w.done)
	})
}

func (w *Worker) run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("notification worker panic recovered", "panic", r)
		}
	}()
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
	for {
		deliveries, err := w.store.ClaimPendingNotificationDeliveries(ctx, notificationClaimBatchSize, deliveryLeaseDuration)
		if err != nil {
			slog.Error("failed to claim pending notification deliveries", "error", err)
			return
		}
		if len(deliveries) == 0 {
			return
		}

		w.dispatchBatch(ctx, deliveries)
	}
}

func (w *Worker) dispatchBatch(ctx context.Context, deliveries []domain.NotificationDelivery) {
	p := concpool.New().WithContext(ctx).WithMaxGoroutines(notificationWorkerConcurrent)
	for i := range deliveries {
		d := deliveries[i]
		p.Go(func(ctx context.Context) error {
			if err := w.dispatch(ctx, &d); err != nil {
				slog.Warn("notification delivery failed", "delivery_id", d.ID, "channel_id", d.ChannelID, "error", err)
			}
			return nil
		})
	}
	_ = p.Wait()
}

func (w *Worker) dispatch(ctx context.Context, d *domain.NotificationDelivery) error {
	ch, err := w.store.GetNotificationChannel(ctx, d.ChannelID, d.ProjectID)
	if err != nil {
		d.Attempts++
		d.LastError = fmt.Sprintf("failed to get channel: %v", err)
		d.Status = "failed"
		d.NextRetryAt = nil
		return w.finishClaim(ctx, d)
	}
	if !ch.Enabled {
		d.LastError = "notification channel disabled"
		d.Status = "failed"
		d.NextRetryAt = nil
		if w.deliveriesCounter != nil {
			w.deliveriesCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "disabled")))
		}
		return w.finishClaim(ctx, d)
	}

	sender, ok := w.senders[ch.ChannelType]
	if !ok {
		d.Attempts++
		d.LastError = fmt.Sprintf("unsupported channel type: %s", ch.ChannelType)
		d.Status = "failed"
		d.NextRetryAt = nil
		return w.finishClaim(ctx, d)
	}

	sendErr := sender.Send(ctx, ch, d)
	d.Attempts++

	if sendErr != nil {
		d.LastError = sanitizeDeliveryError(sendErr)
		if d.Attempts >= d.MaxAttempts {
			d.Status = "failed"
			d.NextRetryAt = nil
		} else {
			d.Status = "pending"
			// Exponential backoff: 30s, 120s, 480s, ...
			backoff := time.Duration(30*math.Pow(4, float64(d.Attempts-1))) * time.Second
			next := time.Now().Add(backoff)
			d.NextRetryAt = &next
		}
		if w.deliveriesCounter != nil {
			w.deliveriesCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		}
	} else {
		d.Status = "delivered"
		d.LastError = ""
		d.NextRetryAt = nil
		now := time.Now()
		d.DeliveredAt = &now
		if w.deliveriesCounter != nil {
			w.deliveriesCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "success")))
		}
	}

	return w.finishClaim(ctx, d)
}

func (w *Worker) finishClaim(ctx context.Context, d *domain.NotificationDelivery) error {
	updated, err := w.store.UpdateClaimedNotificationDelivery(ctx, d)
	if err != nil {
		return err
	}
	if !updated {
		slog.Warn("notification delivery lease lost before update", "delivery_id", d.ID, "channel_id", d.ChannelID)
	}
	return nil
}
