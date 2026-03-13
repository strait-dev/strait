package cdc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
)

type EventPublisher interface {
	Publish(ctx context.Context, channel string, data []byte) error
}

type ChangeEvent struct {
	Table     string          `json:"table"`
	Action    Action          `json:"action"`
	Record    json.RawMessage `json:"record"`
	Changes   json.RawMessage `json:"changes,omitempty"`
	Timestamp string          `json:"timestamp"`
	Source    string          `json:"source,omitempty"`
}

type JobRunHandler struct {
	publisher EventPublisher
	logger    *slog.Logger
}

func NewJobRunHandler(pub EventPublisher, logger *slog.Logger) *JobRunHandler {
	if logger == nil {
		logger = slog.Default()
	}

	return &JobRunHandler{publisher: pub, logger: logger}
}

func (h *JobRunHandler) Table() string { return "job_runs" }

func (h *JobRunHandler) Handle(ctx context.Context, msg Message) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "cdc.HandleJobRun")
	defer span.End()

	var record struct {
		ID        string `json:"id"`
		JobID     string `json:"job_id"`
		ProjectID string `json:"project_id"`
		Status    string `json:"status"`
	}

	if err := json.Unmarshal(msg.Record, &record); err != nil {
		return fmt.Errorf("decode job_run record: %w", err)
	}

	h.logger.Info("cdc job_run change",
		"action", msg.Action,
		"run_id", record.ID,
		"job_id", record.JobID,
		"project_id", record.ProjectID,
		"status", record.Status,
	)

	return publishChangeEvent(ctx, h.publisher, h.logger, ChangeEvent{
		Table:     h.Table(),
		Action:    msg.Action,
		Record:    msg.Record,
		Changes:   msg.Changes,
		Timestamp: msg.Metadata.CommitTimestamp,
		Source:    "cdc",
	}, fmt.Sprintf("cdc:project:%s:job_runs", record.ProjectID))
}

type WorkflowRunHandler struct {
	publisher EventPublisher
	logger    *slog.Logger
}

func NewWorkflowRunHandler(pub EventPublisher, logger *slog.Logger) *WorkflowRunHandler {
	if logger == nil {
		logger = slog.Default()
	}

	return &WorkflowRunHandler{publisher: pub, logger: logger}
}

func (h *WorkflowRunHandler) Table() string { return "workflow_runs" }

func (h *WorkflowRunHandler) Handle(ctx context.Context, msg Message) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "cdc.HandleWorkflowRun")
	defer span.End()

	var record struct {
		ID         string `json:"id"`
		WorkflowID string `json:"workflow_id"`
		ProjectID  string `json:"project_id"`
		Status     string `json:"status"`
	}

	if err := json.Unmarshal(msg.Record, &record); err != nil {
		return fmt.Errorf("decode workflow_run record: %w", err)
	}

	h.logger.Info("cdc workflow_run change",
		"action", msg.Action,
		"workflow_run_id", record.ID,
		"workflow_id", record.WorkflowID,
		"project_id", record.ProjectID,
		"status", record.Status,
	)

	return publishChangeEvent(ctx, h.publisher, h.logger, ChangeEvent{
		Table:     h.Table(),
		Action:    msg.Action,
		Record:    msg.Record,
		Changes:   msg.Changes,
		Timestamp: msg.Metadata.CommitTimestamp,
		Source:    "cdc",
	}, fmt.Sprintf("cdc:project:%s:workflow_runs", record.ProjectID))
}

type WorkflowStepRunHandler struct {
	publisher EventPublisher
	logger    *slog.Logger
}

func NewWorkflowStepRunHandler(pub EventPublisher, logger *slog.Logger) *WorkflowStepRunHandler {
	if logger == nil {
		logger = slog.Default()
	}

	return &WorkflowStepRunHandler{publisher: pub, logger: logger}
}

func (h *WorkflowStepRunHandler) Table() string { return "workflow_step_runs" }

func (h *WorkflowStepRunHandler) Handle(ctx context.Context, msg Message) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "cdc.HandleWorkflowStepRun")
	defer span.End()

	var record struct {
		ID            string `json:"id"`
		WorkflowRunID string `json:"workflow_run_id"`
		StepRef       string `json:"step_ref"`
		Status        string `json:"status"`
	}

	if err := json.Unmarshal(msg.Record, &record); err != nil {
		return fmt.Errorf("decode workflow_step_run record: %w", err)
	}

	h.logger.Info("cdc workflow_step_run change",
		"action", msg.Action,
		"step_run_id", record.ID,
		"workflow_run_id", record.WorkflowRunID,
		"step_ref", record.StepRef,
		"status", record.Status,
	)

	return publishChangeEvent(ctx, h.publisher, h.logger, ChangeEvent{
		Table:     h.Table(),
		Action:    msg.Action,
		Record:    msg.Record,
		Changes:   msg.Changes,
		Timestamp: msg.Metadata.CommitTimestamp,
		Source:    "cdc",
	}, fmt.Sprintf("cdc:workflow_run:%s:steps", record.WorkflowRunID))
}

type EventTriggerHandler struct {
	publisher EventPublisher
	logger    *slog.Logger
}

func NewEventTriggerHandler(pub EventPublisher, logger *slog.Logger) *EventTriggerHandler {
	if logger == nil {
		logger = slog.Default()
	}

	return &EventTriggerHandler{publisher: pub, logger: logger}
}

func (h *EventTriggerHandler) Table() string { return "event_triggers" }

func (h *EventTriggerHandler) Handle(ctx context.Context, msg Message) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "cdc.HandleEventTrigger")
	defer span.End()

	var record struct {
		ID        string `json:"id"`
		EventKey  string `json:"event_key"`
		ProjectID string `json:"project_id"`
		Status    string `json:"status"`
	}

	if err := json.Unmarshal(msg.Record, &record); err != nil {
		return fmt.Errorf("decode event_trigger record: %w", err)
	}

	h.logger.Info("cdc event_trigger change",
		"action", msg.Action,
		"trigger_id", record.ID,
		"event_key", record.EventKey,
		"project_id", record.ProjectID,
		"status", record.Status,
	)

	return publishChangeEvent(ctx, h.publisher, h.logger, ChangeEvent{
		Table:     h.Table(),
		Action:    msg.Action,
		Record:    msg.Record,
		Changes:   msg.Changes,
		Timestamp: msg.Metadata.CommitTimestamp,
		Source:    "cdc",
	}, fmt.Sprintf("cdc:project:%s:event_triggers", record.ProjectID))
}

func publishChangeEvent(ctx context.Context, publisher EventPublisher, logger *slog.Logger, event ChangeEvent, channel string) error {
	if publisher == nil {
		return nil
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal change event: %w", err)
	}

	if err := publisher.Publish(ctx, channel, data); err != nil {
		logger.Warn("failed to publish cdc event", "channel", channel, "error", err)
	}

	return nil
}
