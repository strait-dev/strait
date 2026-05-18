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
		JobID     string `json:"job_id"`
		ProjectID string `json:"project_id"`
		Status    string `json:"status"`
		Attempt   int    `json:"attempt"`
	}
	if err := json.Unmarshal(msg.Record, &record); err != nil {
		return fmt.Errorf("audit handler: unmarshal record: %w", err)
	}

	if record.ProjectID == "" {
		return nil
	}

	action, emit := auditAction(msg.Action, record.Status)
	if !emit {
		return nil
	}

	ev := &domain.AuditEvent{
		ProjectID:    record.ProjectID,
		ActorID:      "system:cdc",
		ActorType:    "system",
		Action:       action,
		ResourceType: "run",
		ResourceID:   record.ID,
		Details:      auditDetails(msg, record.ID, record.JobID, record.ProjectID, record.Status, record.Attempt),
	}

	if err := h.store.CreateAuditEvent(ctx, ev); err != nil {
		h.logger.Warn("cdc audit handler: failed to create audit event",
			"run_id", record.ID, "action", action, "error", err)
		return fmt.Errorf("audit handler: create audit event: %w", err)
	}

	return nil
}

func auditDetails(msg Message, runID, jobID, projectID, status string, attempt int) json.RawMessage {
	details := map[string]any{
		"schema_version": "cdc.run.audit.v1",
		"run_id":         runID,
		"job_id":         jobID,
		"project_id":     projectID,
		"status":         status,
		"cdc_action":     msg.Action,
	}
	if attempt > 0 {
		details["attempt"] = attempt
	}
	if msg.Metadata.CommitTimestamp != "" {
		details["commit_timestamp"] = msg.Metadata.CommitTimestamp
	}
	data, err := json.Marshal(details)
	if err != nil {
		return json.RawMessage(`{"schema_version":"cdc.run.audit.v1"}`)
	}
	return data
}

func auditAction(action Action, status string) (string, bool) {
	switch action {
	case ActionInsert:
		return "run.created", true
	case ActionDelete:
		return "run.deleted", true
	case ActionUpdate:
		if !domain.RunStatus(status).IsTerminal() {
			return "", false
		}
		return "run." + status, true
	default:
		return "", false
	}
}
