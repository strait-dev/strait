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

const maxNotifyAttempts = 3

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

// NotifyAsync sends a notification webhook in a goroutine with up to 3 attempts
// using exponential backoff (1s, 2s, 4s). Updates notify_status to "sent" or "failed".
func (n *EventNotifier) NotifyAsync(trigger *domain.EventTrigger) {
	if trigger.NotifyURL == "" {
		return
	}

	go func() {
		// Total budget: 15s per attempt × 3 + backoff ≈ 52s max.
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

		var lastErr error
	retryLoop:
		for attempt := 1; attempt <= maxNotifyAttempts; attempt++ {
			if attempt > 1 {
				backoff := time.Duration(1<<(attempt-2)) * time.Second // 1s, 2s
				select {
				case <-ctx.Done():
					lastErr = ctx.Err()
					break retryLoop
				case <-time.After(backoff):
				}
			}

			retryable, err := n.doNotify(ctx, trigger, body)
			if err == nil {
				_ = n.store.UpdateEventTriggerNotifyStatus(ctx, trigger.ID, "sent")
				n.logger.Info("event trigger notify sent", "trigger_id", trigger.ID, "url", trigger.NotifyURL, "attempt", attempt)
				return
			}
			lastErr = err

			n.logger.Warn("event trigger notify attempt failed",
				"trigger_id", trigger.ID, "url", trigger.NotifyURL,
				"attempt", attempt, "max_attempts", maxNotifyAttempts, "error", err)

			if !retryable {
				break
			}
		}

		_ = n.store.UpdateEventTriggerNotifyStatus(ctx, trigger.ID, "failed")
		n.logger.Error("event trigger notify exhausted retries",
			"trigger_id", trigger.ID, "url", trigger.NotifyURL, "error", lastErr)
	}()
}

// doNotify performs a single notification HTTP request.
// Returns (retryable, error) — retryable is true for network errors and 5xx.
func (n *EventNotifier) doNotify(ctx context.Context, trigger *domain.EventTrigger, body []byte) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, trigger.NotifyURL, bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Strait-Event-Key", trigger.EventKey)
	req.Header.Set("X-Strait-Trigger-ID", trigger.ID)

	resp, err := n.client.Do(req)
	if err != nil {
		return true, fmt.Errorf("http request: %w", err) // network error → retryable
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return true, fmt.Errorf("server error: status %d", resp.StatusCode) // 5xx → retryable
	}
	if resp.StatusCode >= 400 {
		return false, fmt.Errorf("client error: status %d", resp.StatusCode) // 4xx → not retryable
	}

	return false, nil
}
