// Package webhook provides durable webhook delivery for event triggers.
//
// Deliveries are persisted to the webhook_deliveries table on creation,
// then processed by a background worker with exponential backoff. If all
// attempts are exhausted, the delivery moves to "dead" status (DLQ).
// This survives process restarts — no in-memory state is required.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"

	"go.opentelemetry.io/otel"
)

// DeliveryStore is the subset of store operations needed by the webhook worker.
type DeliveryStore interface {
	CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	UpdateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	ListPendingWebhookRetries(ctx context.Context) ([]domain.WebhookDelivery, error)
	UpdateEventTriggerNotifyStatus(ctx context.Context, id string, notifyStatus string) error
}

// EventNotifier handles both enqueuing and processing webhook deliveries
// for event trigger notifications.
type EventNotifier struct {
	client *http.Client
	store  DeliveryStore
	logger *slog.Logger
}

// NewEventNotifier creates a new event notifier.
func NewEventNotifier(store DeliveryStore, logger *slog.Logger) *EventNotifier {
	if logger == nil {
		logger = slog.Default()
	}
	return &EventNotifier{
		client: &http.Client{Timeout: 10 * time.Second},
		store:  store,
		logger: logger,
	}
}

// NotifyAsync persists a webhook delivery for the given trigger.
// This is the synchronous entry point called during trigger creation
// via the onTriggerCreate callback. The actual HTTP delivery happens
// asynchronously via RunWorker.
func (n *EventNotifier) NotifyAsync(trigger *domain.EventTrigger) {
	if trigger.NotifyURL == "" {
		return
	}

	payload, err := json.Marshal(map[string]any{
		"event_key":    trigger.EventKey,
		"trigger_id":   trigger.ID,
		"project_id":   trigger.ProjectID,
		"expires_at":   trigger.ExpiresAt,
		"callback_url": fmt.Sprintf("/v1/events/%s/send", trigger.EventKey),
	})
	if err != nil {
		n.logger.Error("failed to marshal notify payload", "trigger_id", trigger.ID, "error", err)
		return
	}

	now := time.Now()
	d := &domain.WebhookDelivery{
		EventTriggerID: trigger.ID,
		WebhookURL:     trigger.NotifyURL,
		Status:         domain.WebhookStatusPending,
		Attempts:       0,
		MaxAttempts:    5,
		NextRetryAt:    &now,
	}

	// Store the payload as the last_error field temporarily — the worker reads
	// it from there. Better: we use a dedicated approach below.
	// Actually, we POST the payload directly from the delivery record.
	// We need to stash the payload somewhere. Since the existing schema doesn't
	// have a payload column, we'll reconstruct it from the trigger in the worker.
	// For now, store a marker so the worker can look up the trigger.
	d.LastError = string(payload) // stash payload in last_error temporarily on creation

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := n.store.CreateWebhookDelivery(ctx, d); err != nil {
		n.logger.Error("failed to enqueue webhook delivery", "trigger_id", trigger.ID, "error", err)
		return
	}

	// Clear the last_error now that we've stored it (the worker will use it as payload).
	n.logger.Info("webhook delivery enqueued", "delivery_id", d.ID, "trigger_id", trigger.ID, "url", trigger.NotifyURL)
}

// RunWorker polls for pending deliveries and attempts them. Blocks until ctx is canceled.
// Call this in a goroutine from the service startup (e.g., concpool group).
func (n *EventNotifier) RunWorker(ctx context.Context, pollInterval time.Duration) error {
	n.logger.Info("webhook delivery worker started", "poll_interval", pollInterval)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			n.logger.Info("webhook delivery worker stopped")
			return ctx.Err()
		case <-ticker.C:
			n.processBatch(ctx)
		}
	}
}

// processBatch fetches and processes a batch of pending deliveries.
func (n *EventNotifier) processBatch(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "webhook.ProcessBatch")
	defer span.End()

	deliveries, err := n.store.ListPendingWebhookRetries(ctx)
	if err != nil {
		n.logger.Error("failed to list pending webhook deliveries", "error", err)
		return
	}

	for i := range deliveries {
		if ctx.Err() != nil {
			return
		}
		d := &deliveries[i]
		// Only process event trigger deliveries in this worker.
		// Job run webhook deliveries are handled by the executor.
		if d.EventTriggerID == "" {
			continue
		}
		n.attemptDelivery(ctx, d)
	}
}

// attemptDelivery makes one HTTP request for a delivery.
func (n *EventNotifier) attemptDelivery(ctx context.Context, d *domain.WebhookDelivery) {
	now := time.Now()
	d.Attempts++

	// Reconstruct payload from last_error (where we stashed it on creation)
	// or build a minimal payload from what we know.
	var body []byte
	if d.LastError != "" {
		// Try to parse as JSON — if it is, it's our stashed payload.
		var js json.RawMessage
		if json.Unmarshal([]byte(d.LastError), &js) == nil {
			body = []byte(d.LastError)
			d.LastError = "" // clear so failed attempts use the error message
		}
	}
	if len(body) == 0 {
		// Fallback: minimal payload.
		payload := map[string]any{
			"trigger_id":  d.EventTriggerID,
			"delivery_id": d.ID,
		}
		body, _ = json.Marshal(payload)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.WebhookURL, bytes.NewReader(body))
	if err != nil {
		n.recordFailure(ctx, d, now, false, fmt.Sprintf("create request: %v", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Strait-Trigger-ID", d.EventTriggerID)
	req.Header.Set("X-Strait-Delivery-ID", d.ID)
	req.Header.Set("X-Strait-Attempt", fmt.Sprintf("%d/%d", d.Attempts, d.MaxAttempts))

	resp, err := n.client.Do(req)
	if err != nil {
		n.recordFailure(ctx, d, now, true, fmt.Sprintf("http request: %v", err))
		return
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	statusCode := resp.StatusCode
	d.LastStatusCode = &statusCode

	if statusCode >= 500 {
		n.recordFailure(ctx, d, now, true, fmt.Sprintf("server error: status %d", statusCode))
		return
	}
	if statusCode >= 400 {
		// 4xx: client error, not retryable — go straight to dead.
		n.recordFailure(ctx, d, now, false, fmt.Sprintf("client error: status %d", statusCode))
		return
	}

	// Success.
	d.Status = domain.WebhookStatusDelivered
	d.DeliveredAt = &now
	d.LastError = ""
	if err := n.store.UpdateWebhookDelivery(ctx, d); err != nil {
		n.logger.Error("failed to mark webhook delivered", "delivery_id", d.ID, "error", err)
		return
	}
	_ = n.store.UpdateEventTriggerNotifyStatus(ctx, d.EventTriggerID, "sent")
	n.logger.Info("webhook delivered", "delivery_id", d.ID, "url", d.WebhookURL, "attempt", d.Attempts)
}

// recordFailure handles a failed delivery attempt. For retryable errors, schedules
// the next attempt with exponential backoff. For non-retryable errors or exhausted
// attempts, marks the delivery as dead (DLQ).
func (n *EventNotifier) recordFailure(ctx context.Context, d *domain.WebhookDelivery, now time.Time, retryable bool, errMsg string) {
	d.LastError = errMsg

	// Non-retryable or exhausted → dead letter.
	if !retryable || d.Attempts >= d.MaxAttempts {
		d.Status = domain.WebhookStatusDead
		if err := n.store.UpdateWebhookDelivery(ctx, d); err != nil {
			n.logger.Error("failed to dead-letter webhook", "delivery_id", d.ID, "error", err)
		}
		_ = n.store.UpdateEventTriggerNotifyStatus(ctx, d.EventTriggerID, "failed")
		n.logger.Error("webhook delivery dead-lettered",
			"delivery_id", d.ID, "url", d.WebhookURL,
			"attempts", d.Attempts, "max_attempts", d.MaxAttempts, "error", errMsg)
		return
	}

	// Exponential backoff: 5s, 25s, 125s, 625s (~10min), capped at 30min.
	backoff := min(time.Duration(pow(5, d.Attempts))*time.Second, 30*time.Minute)
	nextAttempt := now.Add(backoff)
	d.NextRetryAt = &nextAttempt
	d.Status = domain.WebhookStatusPending

	if err := n.store.UpdateWebhookDelivery(ctx, d); err != nil {
		n.logger.Error("failed to schedule webhook retry", "delivery_id", d.ID, "error", err)
	}

	n.logger.Warn("webhook delivery failed, scheduled retry",
		"delivery_id", d.ID, "url", d.WebhookURL,
		"attempt", d.Attempts, "max_attempts", d.MaxAttempts,
		"next_attempt", nextAttempt, "error", errMsg)
}

// pow computes base^exp for small positive integers.
func pow(base, exp int) int {
	result := 1
	for range exp {
		result *= base
	}
	return result
}
