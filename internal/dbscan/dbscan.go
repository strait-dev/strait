// Package dbscan provides shared database row scanning utilities.
package dbscan

import (
	"encoding/json"

	"orchestrator/internal/domain"
)

// Scanner is satisfied by pgx.Row and pgx.Rows.
type Scanner interface {
	Scan(dest ...any) error
}

// ScanRun scans a single job_runs row into a domain.JobRun.
func ScanRun(scanner Scanner) (*domain.JobRun, error) {
	var run domain.JobRun
	var payload []byte
	var result []byte
	var runError *string
	var parentRunID *string
	var idempotencyKey *string
	var workflowStepRunID *string

	err := scanner.Scan(
		&run.ID,
		&run.JobID,
		&run.ProjectID,
		&run.Status,
		&run.Attempt,
		&payload,
		&result,
		&runError,
		&run.TriggeredBy,
		&run.ScheduledAt,
		&run.StartedAt,
		&run.FinishedAt,
		&run.HeartbeatAt,
		&run.NextRetryAt,
		&run.ExpiresAt,
		&parentRunID,
		&run.Priority,
		&idempotencyKey,
		&run.JobVersion,
		&run.CreatedAt,
		&workflowStepRunID,
	)
	if err != nil {
		return nil, err
	}

	if payload != nil {
		run.Payload = json.RawMessage(payload)
	}
	if result != nil {
		run.Result = json.RawMessage(result)
	}
	if runError != nil {
		run.Error = *runError
	}
	if parentRunID != nil {
		run.ParentRunID = *parentRunID
	}
	if idempotencyKey != nil {
		run.IdempotencyKey = *idempotencyKey
	}
	if workflowStepRunID != nil {
		run.WorkflowStepRunID = *workflowStepRunID
	}

	return &run, nil
}

// NilIfEmptyString returns nil for empty strings, preserving NULL in SQL inserts.
func NilIfEmptyString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// NilIfEmptyRawMessage returns nil for empty JSON, preserving NULL in SQL inserts.
func NilIfEmptyRawMessage(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return value
}
