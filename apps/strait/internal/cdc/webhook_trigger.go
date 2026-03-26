package cdc

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"time"

	"strait/internal/domain"
)

// WebhookTriggerStore is the minimal store interface for CDC-driven webhook delivery.
type WebhookTriggerStore interface {
	ListWebhookSubscriptions(ctx context.Context, projectID string) ([]domain.WebhookSubscription, error)
	CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
}

// WebhookTriggerHandler creates webhook deliveries from CDC events on job_runs.
// When a run reaches a terminal status, it looks up matching webhook subscriptions
// and enqueues deliveries for the existing DeliveryWorker to process.
type WebhookTriggerHandler struct {
	store  WebhookTriggerStore
	logger *slog.Logger
}

// NewWebhookTriggerHandler creates a CDC handler that triggers webhook deliveries.
func NewWebhookTriggerHandler(store WebhookTriggerStore, logger *slog.Logger) *WebhookTriggerHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &WebhookTriggerHandler{store: store, logger: logger}
}

// Table returns the table this handler watches.
func (h *WebhookTriggerHandler) Table() string { return "job_runs" }

// Handle processes a CDC event for a job run status change.
func (h *WebhookTriggerHandler) Handle(ctx context.Context, msg Message) error {
	// Only trigger on updates (status transitions), not inserts.
	if msg.Action != ActionUpdate {
		return nil
	}

	var record struct {
		ID        string `json:"id"`
		JobID     string `json:"job_id"`
		ProjectID string `json:"project_id"`
		Status    string `json:"status"`
		Attempt   int    `json:"attempt"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal(msg.Record, &record); err != nil {
		return err
	}

	if record.ProjectID == "" {
		return nil
	}

	status := domain.RunStatus(record.Status)
	if !status.IsTerminal() {
		return nil
	}

	eventType := mapStatusToWebhookEvent(status)
	if eventType == "" {
		return nil
	}

	subs, err := h.store.ListWebhookSubscriptions(ctx, record.ProjectID)
	if err != nil {
		h.logger.Warn("cdc webhook trigger: failed to list subscriptions",
			"project_id", record.ProjectID, "error", err)
		return nil // Don't nack on store errors.
	}

	for _, sub := range subs {
		if !sub.Active {
			continue
		}
		if !matchesEventType(sub.EventTypes, eventType) {
			continue
		}

		payload, _ := json.Marshal(map[string]any{
			"event_type": eventType,
			"run_id":     record.ID,
			"job_id":     record.JobID,
			"project_id": record.ProjectID,
			"status":     record.Status,
			"attempt":    record.Attempt,
			"error":      record.Error,
			"timestamp":  time.Now().UTC(),
		})

		now := time.Now()
		if createErr := h.store.CreateWebhookDelivery(ctx, &domain.WebhookDelivery{
			RunID:       record.ID,
			JobID:       record.JobID,
			WebhookURL:  sub.WebhookURL,
			Status:      "pending",
			MaxAttempts: 5,
			NextRetryAt: &now,
			LastError:   string(payload),
		}); createErr != nil {
			h.logger.Warn("cdc webhook trigger: failed to create delivery",
				"run_id", record.ID, "webhook_url", sub.WebhookURL, "error", createErr)
		}
	}

	return nil
}

func mapStatusToWebhookEvent(s domain.RunStatus) string {
	switch s {
	case domain.StatusCompleted:
		return domain.WebhookEventRunCompleted
	case domain.StatusFailed:
		return domain.WebhookEventRunFailed
	case domain.StatusTimedOut:
		return domain.WebhookEventRunTimedOut
	case domain.StatusCanceled:
		return domain.WebhookEventRunCanceled
	default:
		return ""
	}
}

func matchesEventType(subscribed []string, event string) bool {
	return slices.Contains(subscribed, event)
}
