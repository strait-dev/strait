package cdc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"
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

	record, err := parseTerminalRunRecord(msg.Record)
	if err != nil {
		return fmt.Errorf("webhook trigger: unmarshal record: %w", err)
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
		return fmt.Errorf("webhook trigger: list subscriptions: %w", err)
	}

	var payload []byte
	var nextRetryAt time.Time
	var createErrs []error
	for _, sub := range subs {
		if !sub.Active {
			continue
		}
		if !matchesEventType(sub.EventTypes, eventType) {
			continue
		}

		if payload == nil {
			var marshalErr error
			payload, marshalErr = marshalWebhookTriggerPayload(
				eventType,
				record.ID,
				record.JobID,
				record.ProjectID,
				record.Status,
				record.Attempt,
				record.Error,
				time.Now().UTC(),
			)
			if marshalErr != nil {
				return fmt.Errorf("webhook trigger: marshal payload: %w", marshalErr)
			}
			nextRetryAt = time.Now()
		}

		if createErr := h.store.CreateWebhookDelivery(ctx, &domain.WebhookDelivery{
			RunID:          record.ID,
			JobID:          record.JobID,
			SubscriptionID: sub.ID,
			ProjectID:      record.ProjectID,
			WebhookURL:     sub.WebhookURL,
			WebhookSecret:  sub.Secret,
			Payload:        payload,
			Status:         "pending",
			MaxAttempts:    5,
			NextRetryAt:    &nextRetryAt,
			DedupeKey:      webhookTriggerDedupeKey(record.ID, eventType, sub.ID),
		}); createErr != nil {
			h.logger.Warn("cdc webhook trigger: failed to create delivery",
				"run_id", record.ID, "webhook_url", httputil.RedactURLForLog(sub.WebhookURL), "error", createErr)
			createErrs = append(createErrs, createErr)
		}
	}

	if err := errors.Join(createErrs...); err != nil {
		return fmt.Errorf("webhook trigger: create delivery: %w", err)
	}
	return nil
}

func webhookTriggerDedupeKey(runID, eventType, subscriptionID string) string {
	return cdcJobRunEventDedupeKey(runID, eventType, subscriptionID)
}

func marshalWebhookTriggerPayload(eventType, runID, jobID, projectID, status string, attempt int, errMessage string, timestamp time.Time) ([]byte, error) {
	return marshalTerminalRunPayload(eventType, runID, jobID, projectID, status, attempt, errMessage, timestamp), nil
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
	case domain.StatusCrashed, domain.StatusSystemFailed, domain.StatusExpired, domain.StatusDeadLetter:
		return domain.WebhookEventRunFailed
	default:
		return ""
	}
}

func matchesEventType(subscribed []string, event string) bool {
	return slices.Contains(subscribed, event)
}
