package cdc

import (
	"context"
	"encoding/json"
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

	var record struct {
		ID        string `json:"id"`
		JobID     string `json:"job_id"`
		ProjectID string `json:"project_id"`
		Status    string `json:"status"`
		Attempt   int    `json:"attempt"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal(msg.Record, &record); err != nil {
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
	var createErrs []error
	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}

		if createErr := h.store.CreateNotificationDelivery(ctx, &domain.NotificationDelivery{
			ChannelID:   ch.ID,
			ProjectID:   record.ProjectID,
			EventType:   eventType,
			Payload:     payload,
			Status:      "pending",
			MaxAttempts: 5,
			NextRetryAt: &now,
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
	return fmt.Sprintf("cdc:job_runs:%s:%s:%s", runID, eventType, channelID)
}
