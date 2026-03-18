package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"strait/internal/domain"
)

// OnCompleteTrigger handles triggering a workflow when a job run completes.
type OnCompleteTrigger struct {
	workflowLookup  WorkflowLookup
	workflowTrigger WorkflowTriggerer
	logger          *slog.Logger
}

// WorkflowLookup resolves a workflow by slug within a project.
type WorkflowLookup interface {
	GetWorkflowBySlug(ctx context.Context, projectID, slug string) (*domain.Workflow, error)
}

// WorkflowTriggerer triggers a workflow run.
type WorkflowTriggerer interface {
	TriggerWorkflow(
		ctx context.Context,
		workflowID, projectID string,
		payload json.RawMessage,
		triggeredBy string,
		stepOverrides []domain.StepOverride,
		extraTags map[string]string,
	) (*domain.WorkflowRun, error)
}

// NewOnCompleteTrigger creates a new OnCompleteTrigger.
func NewOnCompleteTrigger(lookup WorkflowLookup, trigger WorkflowTriggerer, logger *slog.Logger) *OnCompleteTrigger {
	if logger == nil {
		logger = slog.Default()
	}
	return &OnCompleteTrigger{
		workflowLookup:  lookup,
		workflowTrigger: trigger,
		logger:          logger,
	}
}

// MaybeTrigger checks if the job has an on_complete_trigger_workflow configured
// and triggers the workflow if the run completed successfully.
func (t *OnCompleteTrigger) MaybeTrigger(ctx context.Context, run *domain.JobRun, job *domain.Job, result json.RawMessage) {
	if job.OnCompleteTriggerWorkflow == "" {
		return
	}
	if run.Status != domain.StatusCompleted {
		return
	}
	if t.workflowLookup == nil || t.workflowTrigger == nil {
		return
	}

	workflow, err := t.workflowLookup.GetWorkflowBySlug(ctx, job.ProjectID, job.OnCompleteTriggerWorkflow)
	if err != nil {
		t.logger.Warn("on_complete workflow not found",
			"job_id", job.ID,
			"run_id", run.ID,
			"workflow_slug", job.OnCompleteTriggerWorkflow,
			"error", err,
		)
		return
	}

	payload := result
	if len(job.OnCompletePayloadMapping) > 0 {
		mapped, mapErr := applyPayloadMapping(result, job.OnCompletePayloadMapping)
		if mapErr != nil {
			t.logger.Warn("on_complete payload mapping failed, using full result",
				"job_id", job.ID,
				"run_id", run.ID,
				"error", mapErr,
			)
		} else {
			payload = mapped
		}
	}

	wfRun, triggerErr := t.workflowTrigger.TriggerWorkflow(
		ctx,
		workflow.ID,
		job.ProjectID,
		payload,
		domain.TriggerJobCompletion,
		nil,
		map[string]string{
			"source_job_id": job.ID,
			"source_run_id": run.ID,
		},
	)
	if triggerErr != nil {
		t.logger.Warn("on_complete workflow trigger failed",
			"job_id", job.ID,
			"run_id", run.ID,
			"workflow_id", workflow.ID,
			"error", triggerErr,
		)
		return
	}

	t.logger.Info("on_complete workflow triggered",
		"job_id", job.ID,
		"run_id", run.ID,
		"workflow_id", workflow.ID,
		"workflow_run_id", wfRun.ID,
	)
}

// applyPayloadMapping extracts fields from result using the mapping definition.
// The mapping is a JSON object where keys are output field names and values
// are dot-notation paths into the result.
func applyPayloadMapping(result json.RawMessage, mapping json.RawMessage) (json.RawMessage, error) {
	if len(result) == 0 || len(mapping) == 0 {
		return result, nil
	}

	var pathMap map[string]string
	if err := json.Unmarshal(mapping, &pathMap); err != nil {
		return nil, fmt.Errorf("unmarshal payload mapping: %w", err)
	}

	var resultData map[string]any
	if unmarshalErr := json.Unmarshal(result, &resultData); unmarshalErr != nil {
		// If result isn't a JSON object, return as-is.
		return result, nil //nolint:nilerr // intentional: non-object results pass through unchanged
	}

	output := make(map[string]any, len(pathMap))
	for key, path := range pathMap {
		val := extractPath(resultData, path)
		if val != nil {
			output[key] = val
		}
	}

	mapped, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("marshal mapped payload: %w", err)
	}
	return mapped, nil
}

// extractPath extracts a value from a nested map using dot-notation.
func extractPath(data map[string]any, path string) any {
	current := any(data)
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '.' {
			key := path[start:i]
			start = i + 1

			m, ok := current.(map[string]any)
			if !ok {
				return nil
			}
			current = m[key]
		}
	}
	return current
}
