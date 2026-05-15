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
//
// Emission is scoped to lifecycle-significant transitions:
//   - INSERT (run.created) and DELETE (run.deleted) always emit.
//   - UPDATE only emits when the run reached a terminal state
//     (completed, failed, timed_out, crashed, system_failed, canceled,
//     expired, dead_letter).
//
// The job_runs table is updated on every heartbeat tick and on every
// intermediate status flip (queued→dequeued→executing→waiting…). Without
// this gate each of those updates produced a signed audit row plus two
// advisory locks for HMAC-chain serialization, which made the audit chain
// a meaningful bottleneck on hot-path workloads. Audit consumers care
// about run creation, terminal outcome, and deletion — not the per-tick
// internals — so the gate is information-preserving.
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

	// Suppress UPDATE-driven audit rows for non-terminal status transitions
	// (heartbeats, queued→dequeued→executing→waiting flips). Terminal
	// states still emit because they are the lifecycle event downstream
	// consumers care about. INSERT/DELETE are unaffected and fall through.
	if msg.Action == ActionUpdate && !domain.RunStatus(record.Status).IsTerminal() {
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
