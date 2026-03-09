// Package webhook provides notification webhook delivery for event triggers.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"
)

// NotifyStore is the subset of store operations needed by the notifier.
type NotifyStore interface {
	UpdateEventTriggerNotifyStatus(ctx context.Context, id string, notifyStatus string) error
}

// EventNotifier sends webhook notifications when event triggers are created.
type EventNotifier struct {
	client *http.Client
	store  NotifyStore
	logger *slog.Logger
}

// NewEventNotifier creates a new event notifier with a 10-second HTTP timeout.
func NewEventNotifier(store NotifyStore, logger *slog.Logger) *EventNotifier {
	return &EventNotifier{
		client: &http.Client{Timeout: 10 * time.Second},
		store:  store,
		logger: logger,
	}
}

// NotifyAsync sends a notification webhook in a goroutine (fire-and-forget).
// It updates the trigger's notify_status to "sent" or "failed".
func (n *EventNotifier) NotifyAsync(trigger *domain.EventTrigger) {
	if trigger.NotifyURL == "" {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		payload := map[string]any{
			"event_key":    trigger.EventKey,
			"trigger_id":   trigger.ID,
			"project_id":   trigger.ProjectID,
			"expires_at":   trigger.ExpiresAt,
			"callback_url": fmt.Sprintf("/v1/events/%s/send", trigger.EventKey),
		}

		body, err := json.Marshal(payload)
		if err != nil {
			n.logger.Error("failed to marshal notify payload", "trigger_id", trigger.ID, "error", err)
			return
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, trigger.NotifyURL, bytes.NewReader(body))
		if err != nil {
			n.logger.Error("failed to create notify request", "trigger_id", trigger.ID, "error", err)
			_ = n.store.UpdateEventTriggerNotifyStatus(ctx, trigger.ID, "failed")
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Strait-Event-Key", trigger.EventKey)
		req.Header.Set("X-Strait-Trigger-ID", trigger.ID)

		resp, err := n.client.Do(req)
		if err != nil {
			n.logger.Warn("event trigger notify failed", "trigger_id", trigger.ID, "url", trigger.NotifyURL, "error", err)
			_ = n.store.UpdateEventTriggerNotifyStatus(ctx, trigger.ID, "failed")
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			n.logger.Warn("event trigger notify returned error", "trigger_id", trigger.ID, "url", trigger.NotifyURL, "status", resp.StatusCode)
			_ = n.store.UpdateEventTriggerNotifyStatus(ctx, trigger.ID, "failed")
			return
		}

		_ = n.store.UpdateEventTriggerNotifyStatus(ctx, trigger.ID, "sent")
		n.logger.Info("event trigger notify sent", "trigger_id", trigger.ID, "url", trigger.NotifyURL)
	}()
}
