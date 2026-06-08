package cdc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"
)

// NotificationTriggerStore is the minimal store interface for CDC-driven notification delivery.
type NotificationTriggerStore interface {
	ListNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error
}

// NotificationTriggerHandler creates notification deliveries from CDC events on job_runs.
// When a run reaches a terminal status, it looks up enabled notification channels
// and enqueues deliveries for each matching channel.
type NotificationTriggerHandler struct {
	store  NotificationTriggerStore
	logger *slog.Logger
}

// NewNotificationTriggerHandler creates a CDC handler that triggers notification deliveries.
func NewNotificationTriggerHandler(store NotificationTriggerStore, logger *slog.Logger) *NotificationTriggerHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &NotificationTriggerHandler{store: store, logger: logger}
}

// Table returns the table this handler watches.
func (h *NotificationTriggerHandler) Table() string { return "job_runs" }

// Handle processes a CDC event for a job run status change.
func (h *NotificationTriggerHandler) Handle(ctx context.Context, msg Message) error {
	if msg.Action != ActionUpdate {
		return nil
	}

	record, err := parseTerminalRunRecord(msg.Record)
	if err != nil {
		return fmt.Errorf("notification trigger: unmarshal record: %w", err)
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

	channels, err := h.store.ListNotificationChannels(ctx, record.ProjectID)
	if err != nil {
		h.logger.Warn("cdc notification trigger: failed to list channels",
			"project_id", record.ProjectID, "error", err)
		return fmt.Errorf("notification trigger: list channels: %w", err)
	}

	var payload []byte
	var nextRetryAt time.Time
	var createErrs []error
	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		if payload == nil {
			var marshalErr error
			payload, marshalErr = marshalNotificationTriggerPayload(
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
				return fmt.Errorf("notification trigger: marshal payload: %w", marshalErr)
			}
			nextRetryAt = time.Now()
		}

		if createErr := h.store.CreateNotificationDelivery(ctx, &domain.NotificationDelivery{
			ChannelID:   ch.ID,
			ProjectID:   record.ProjectID,
			EventType:   eventType,
			Payload:     payload,
			Status:      "pending",
			MaxAttempts: 5,
			NextRetryAt: &nextRetryAt,
			DedupeKey:   notificationTriggerDedupeKey(record.ID, eventType, ch.ID),
		}); createErr != nil {
			h.logger.Warn("cdc notification trigger: failed to create delivery",
				"run_id", record.ID, "channel_id", ch.ID, "error", createErr)
			createErrs = append(createErrs, createErr)
		}
	}

	if err := errors.Join(createErrs...); err != nil {
		return fmt.Errorf("notification trigger: create delivery: %w", err)
	}
	return nil
}

func notificationTriggerDedupeKey(runID, eventType, channelID string) string {
	return cdcJobRunEventDedupeKey(runID, eventType, channelID)
}

func marshalNotificationTriggerPayload(eventType, runID, jobID, projectID, status string, attempt int, errMessage string, timestamp time.Time) ([]byte, error) {
	return marshalTerminalRunPayload(eventType, runID, jobID, projectID, status, attempt, errMessage, timestamp), nil
}
