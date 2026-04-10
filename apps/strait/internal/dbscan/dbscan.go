// Package dbscan provides shared database row scanning utilities.
package dbscan

import (
	"encoding/json"

	"strait/internal/domain"
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
	var metadata []byte
	var executionTrace []byte
	var tagsJSON []byte
	var runError *string
	var errorClass *string
	var parentRunID *string
	var idempotencyKey *string
	var workflowStepRunID *string
	var continuationOf *string
	var jobVersionID *string
	var createdBy *string
	var batchID *string
	var concurrencyKey *string
	var executionMode *string
	var machineID *string
	var deploymentID *string
	var pinnedImageURI *string
	var pinnedImageDigest *string
	var isRollback bool
	var agentDeploymentID *string

	err := scanner.Scan(
		&run.ID,
		&run.JobID,
		&run.ProjectID,
		&run.Status,
		&run.Attempt,
		&payload,
		&result,
		&metadata,
		&runError,
		&errorClass,
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
		&executionTrace,
		&run.DebugMode,
		&continuationOf,
		&run.LineageDepth,
		&tagsJSON,
		&jobVersionID,
		&createdBy,
		&batchID,
		&concurrencyKey,
		&executionMode,
		&machineID,
		&deploymentID,
		&pinnedImageURI,
		&pinnedImageDigest,
		&isRollback,
		&agentDeploymentID,
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
	if metadata != nil {
		if err := json.Unmarshal(metadata, &run.Metadata); err != nil {
			return nil, err
		}
	}
	if executionTrace != nil {
		var trace domain.ExecutionTrace
		if err := json.Unmarshal(executionTrace, &trace); err != nil {
			return nil, err
		}
		run.ExecutionTrace = &trace
	}
	if runError != nil {
		run.Error = *runError
	}
	if errorClass != nil {
		run.ErrorClass = *errorClass
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
	if continuationOf != nil {
		run.ContinuationOf = *continuationOf
	}
	if len(tagsJSON) > 0 && string(tagsJSON) != "{}" {
		if err := json.Unmarshal(tagsJSON, &run.Tags); err != nil {
			return nil, err
		}
	}
	if jobVersionID != nil {
		run.JobVersionID = *jobVersionID
	}
	if createdBy != nil {
		run.CreatedBy = *createdBy
	}
	if batchID != nil {
		run.BatchID = *batchID
	}
	if concurrencyKey != nil {
		run.ConcurrencyKey = *concurrencyKey
	}
	if executionMode != nil {
		run.ExecutionMode = domain.ExecutionMode(*executionMode)
	}
	if machineID != nil {
		run.MachineID = *machineID
	}
	if deploymentID != nil {
		run.DeploymentID = *deploymentID
	}
	if pinnedImageURI != nil {
		run.PinnedImageURI = *pinnedImageURI
	}
	if pinnedImageDigest != nil {
		run.PinnedImageDigest = *pinnedImageDigest
	}
	run.IsRollback = isRollback
	if agentDeploymentID != nil {
		run.AgentDeploymentID = *agentDeploymentID
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

func NilIfZeroInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

// NilIfZeroInt64 returns nil for zero int64 values, preserving NULL in SQL inserts.
func NilIfZeroInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

// NilIfEmptyIntSlice returns nil for empty slices, preserving NULL in SQL inserts.
func NilIfEmptyIntSlice(value []int) any {
	if len(value) == 0 {
		return nil
	}
	return value
}
