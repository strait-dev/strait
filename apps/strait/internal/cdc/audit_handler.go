package cdc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"strait/internal/domain"
)

// AuditStore is the minimal store interface for CDC-driven audit event creation.
type AuditStore interface {
	CreateAuditEvent(ctx context.Context, ev *domain.AuditEvent) error
}

// AuditHandler creates audit events from CDC events on job_runs.
// It fires on ALL actions (insert, update, delete), not just terminal statuses.
type AuditHandler struct {
	store  AuditStore
	logger *slog.Logger
}

// NewAuditHandler creates a CDC handler that creates audit events for run changes.
func NewAuditHandler(store AuditStore, logger *slog.Logger) *AuditHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuditHandler{store: store, logger: logger}
}

// Table returns the table this handler watches.
func (h *AuditHandler) Table() string { return "job_runs" }

// Handle processes a CDC event for a job run change.
func (h *AuditHandler) Handle(ctx context.Context, msg Message) error {
	var record struct {
		ID        string `json:"id"`
		ProjectID string `json:"project_id"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(msg.Record, &record); err != nil {
		return fmt.Errorf("audit handler: unmarshal record: %w", err)
	}

	if record.ProjectID == "" {
		return nil
	}

	action := auditAction(msg.Action, record.Status)

	ev := &domain.AuditEvent{
		ProjectID:    record.ProjectID,
		ActorID:      "system:cdc",
		ActorType:    "system",
		Action:       action,
		ResourceType: "run",
		ResourceID:   record.ID,
		Details:      msg.Record,
	}

	if err := h.store.CreateAuditEvent(ctx, ev); err != nil {
		h.logger.Warn("cdc audit handler: failed to create audit event",
			"run_id", record.ID, "action", action, "error", err)
		return nil
	}

	return nil
}

func auditAction(action Action, status string) string {
	switch action {
	case ActionInsert:
		return "run.created"
	case ActionDelete:
		return "run.deleted"
	default:
		return "run." + status
	}
}
