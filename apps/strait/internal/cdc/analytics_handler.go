package cdc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"
)

// AnalyticsHandler enqueues ClickHouse analytics records from CDC events on job_runs.
// When a run reaches a terminal status, it constructs a RunAnalyticsRecord and
// enqueues it via the ClickHouse exporter.
type AnalyticsHandler struct {
	exporter *clickhouse.Exporter
	logger   *slog.Logger
}

// NewAnalyticsHandler creates a CDC handler that enqueues run analytics to ClickHouse.
func NewAnalyticsHandler(exporter *clickhouse.Exporter, logger *slog.Logger) *AnalyticsHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &AnalyticsHandler{exporter: exporter, logger: logger}
}

// Table returns the table this handler watches.
func (h *AnalyticsHandler) Table() string { return "job_runs" }

// Handle processes a CDC event for a job run status change.
func (h *AnalyticsHandler) Handle(_ context.Context, msg Message) error {
	if h.exporter == nil {
		return nil
	}

	if msg.Action != ActionUpdate {
		return nil
	}

	var record struct {
		ID            string `json:"id"`
		JobID         string `json:"job_id"`
		ProjectID     string `json:"project_id"`
		Status        string `json:"status"`
		ExecutionMode string `json:"execution_mode"`
		MachinePreset string `json:"machine_preset"`
		Attempt       int    `json:"attempt"`
		TriggeredBy   string `json:"triggered_by"`
		Tags          string `json:"tags"`
		JobVersionID  string `json:"job_version_id"`
		CreatedAt     string `json:"created_at"`
		StartedAt     string `json:"started_at"`
		FinishedAt    string `json:"finished_at"`
	}
	if err := json.Unmarshal(msg.Record, &record); err != nil {
		return fmt.Errorf("analytics handler: unmarshal record: %w", err)
	}

	status := domain.RunStatus(record.Status)
	if !status.IsTerminal() {
		return nil
	}

	createdAt, _ := time.Parse(time.RFC3339, record.CreatedAt)

	var startedAt *time.Time
	if record.StartedAt != "" {
		if t, err := time.Parse(time.RFC3339, record.StartedAt); err == nil {
			startedAt = &t
		}
	}

	var finishedAt *time.Time
	if record.FinishedAt != "" {
		if t, err := time.Parse(time.RFC3339, record.FinishedAt); err == nil {
			finishedAt = &t
		}
	}

	var durationMs uint64
	if startedAt != nil && finishedAt != nil {
		if d := finishedAt.Sub(*startedAt); d > 0 {
			durationMs = uint64(d.Milliseconds()) //nolint:gosec
		}
	}

	analyticsRecord := clickhouse.RunAnalyticsRecord{
		RunID:         record.ID,
		JobID:         record.JobID,
		ProjectID:     record.ProjectID,
		Status:        record.Status,
		ExecutionMode: record.ExecutionMode,
		Attempt:       record.Attempt,
		DurationMs:    durationMs,
		TriggeredBy:   record.TriggeredBy,
		Tags:          record.Tags,
		JobVersionID:  record.JobVersionID,
		CreatedAt:     createdAt,
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
	}

	if ok := h.exporter.Enqueue(analyticsRecord); !ok {
		h.logger.Warn("cdc analytics handler: failed to enqueue record",
			"run_id", record.ID)
	}

	return nil
}
