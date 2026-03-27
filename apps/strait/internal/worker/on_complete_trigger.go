package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"strait/internal/domain"
)

// JobLookup resolves a job by slug within a project.
type JobLookup interface {
	GetJobBySlug(ctx context.Context, projectID, slug string) (*domain.Job, error)
}

// JobEnqueuer enqueues a new job run.
type JobEnqueuer interface {
	Enqueue(ctx context.Context, run *domain.JobRun) error
}

// OnCompleteTrigger handles triggering a workflow or job when a job run completes.
type OnCompleteTrigger struct {
	workflowLookup  WorkflowLookup
	workflowTrigger WorkflowTriggerer
	jobLookup       JobLookup
	jobEnqueuer     JobEnqueuer
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
func NewOnCompleteTrigger(lookup WorkflowLookup, trigger WorkflowTriggerer, jobLookup JobLookup, jobEnqueuer JobEnqueuer, logger *slog.Logger) *OnCompleteTrigger {
	if logger == nil {
		logger = slog.Default()
	}
	return &OnCompleteTrigger{
		workflowLookup:  lookup,
		workflowTrigger: trigger,
		jobLookup:       jobLookup,
		jobEnqueuer:     jobEnqueuer,
		logger:          logger,
	}
}

// MaybeTrigger checks if the job has on_complete trigger configured
// and triggers the downstream workflow or job if the run completed successfully.
func (t *OnCompleteTrigger) MaybeTrigger(ctx context.Context, run *domain.JobRun, job *domain.Job, result json.RawMessage) {
	if run == nil || job == nil {
		return
	}
	if run.Status != domain.StatusCompleted {
		return
	}

	hasWorkflow := job.OnCompleteTriggerWorkflow != ""
	hasJob := job.OnCompleteTriggerJob != ""
	if !hasWorkflow && !hasJob {
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

	if hasWorkflow {
		t.triggerWorkflow(ctx, run, job, payload, job.OnCompleteTriggerWorkflow, domain.TriggerJobCompletion)
	}
	if hasJob {
		t.triggerJob(ctx, run, job, payload, job.OnCompleteTriggerJob, domain.TriggerJobChain)
	}
}

// MaybeTriggerOnFailure checks if the job has on_failure trigger configured
// and triggers the downstream workflow or job when the run reaches a terminal failure state.
func (t *OnCompleteTrigger) MaybeTriggerOnFailure(ctx context.Context, run *domain.JobRun, job *domain.Job, errMsg string) {
	if run == nil || job == nil {
		return
	}

	hasWorkflow := job.OnFailureTriggerWorkflow != ""
	hasJob := job.OnFailureTriggerJob != ""
	if !hasWorkflow && !hasJob {
		return
	}

	if !isTerminalFailureStatus(run.Status) {
		return
	}

	// Build failure context payload.
	failurePayload, _ := json.Marshal(map[string]any{
		"source_job_id":  job.ID,
		"source_run_id":  run.ID,
		"error":          errMsg,
		"error_class":    run.ErrorClass,
		"status":         string(run.Status),
		"attempt":        run.Attempt,
		"original_input": run.Payload,
	})

	payload := json.RawMessage(failurePayload)
	if len(job.OnFailurePayloadMapping) > 0 {
		mapped, mapErr := applyPayloadMapping(failurePayload, job.OnFailurePayloadMapping)
		if mapErr != nil {
			t.logger.Warn("on_failure payload mapping failed, using full failure context",
				"job_id", job.ID,
				"run_id", run.ID,
				"error", mapErr,
			)
		} else {
			payload = mapped
		}
	}

	if hasWorkflow {
		t.triggerWorkflow(ctx, run, job, payload, job.OnFailureTriggerWorkflow, domain.TriggerJobFailure)
	}
	if hasJob {
		t.triggerJob(ctx, run, job, payload, job.OnFailureTriggerJob, domain.TriggerJobFailure)
	}
}

func (t *OnCompleteTrigger) triggerWorkflow(ctx context.Context, run *domain.JobRun, job *domain.Job, payload json.RawMessage, slug, triggeredBy string) {
	if t.workflowLookup == nil || t.workflowTrigger == nil {
		return
	}

	workflow, err := t.workflowLookup.GetWorkflowBySlug(ctx, job.ProjectID, slug)
	if err != nil {
		t.logger.Warn("chained workflow not found",
			"job_id", job.ID,
			"run_id", run.ID,
			"workflow_slug", slug,
			"trigger_type", triggeredBy,
			"error", err,
		)
		return
	}

	wfRun, triggerErr := t.workflowTrigger.TriggerWorkflow(
		ctx,
		workflow.ID,
		job.ProjectID,
		payload,
		triggeredBy,
		nil,
		map[string]string{
			"source_job_id": job.ID,
			"source_run_id": run.ID,
		},
	)
	if triggerErr != nil {
		t.logger.Warn("chained workflow trigger failed",
			"job_id", job.ID,
			"run_id", run.ID,
			"workflow_id", workflow.ID,
			"trigger_type", triggeredBy,
			"error", triggerErr,
		)
		return
	}

	t.logger.Info("chained workflow triggered",
		"job_id", job.ID,
		"run_id", run.ID,
		"workflow_id", workflow.ID,
		"workflow_run_id", wfRun.ID,
		"trigger_type", triggeredBy,
	)
}

func (t *OnCompleteTrigger) triggerJob(ctx context.Context, run *domain.JobRun, job *domain.Job, payload json.RawMessage, slug, triggeredBy string) {
	if t.jobLookup == nil || t.jobEnqueuer == nil {
		return
	}

	// Enforce max chain depth to prevent infinite loops.
	if run.LineageDepth >= domain.MaxJobChainDepth {
		t.logger.Warn("job chain depth limit reached, skipping downstream trigger",
			"job_id", job.ID,
			"run_id", run.ID,
			"target_slug", slug,
			"lineage_depth", run.LineageDepth,
			"max_depth", domain.MaxJobChainDepth,
		)
		return
	}

	targetJob, err := t.jobLookup.GetJobBySlug(ctx, job.ProjectID, slug)
	if err != nil {
		t.logger.Warn("chained job not found",
			"job_id", job.ID,
			"run_id", run.ID,
			"target_slug", slug,
			"trigger_type", triggeredBy,
			"error", err,
		)
		return
	}

	downstreamRun := &domain.JobRun{
		JobID:        targetJob.ID,
		ProjectID:    job.ProjectID,
		Payload:      payload,
		TriggeredBy:  triggeredBy,
		ParentRunID:  run.ID,
		LineageDepth: run.LineageDepth + 1,
		Tags: map[string]string{
			"source_job_id": job.ID,
			"source_run_id": run.ID,
		},
	}

	if enqueueErr := t.jobEnqueuer.Enqueue(ctx, downstreamRun); enqueueErr != nil {
		t.logger.Warn("chained job enqueue failed",
			"job_id", job.ID,
			"run_id", run.ID,
			"target_job_id", targetJob.ID,
			"trigger_type", triggeredBy,
			"error", enqueueErr,
		)
		return
	}

	t.logger.Info("chained job triggered",
		"job_id", job.ID,
		"run_id", run.ID,
		"target_job_id", targetJob.ID,
		"target_run_id", downstreamRun.ID,
		"trigger_type", triggeredBy,
		"lineage_depth", downstreamRun.LineageDepth,
	)
}

func isTerminalFailureStatus(status domain.RunStatus) bool {
	switch status {
	case domain.StatusDeadLetter, domain.StatusTimedOut, domain.StatusCrashed, domain.StatusSystemFailed:
		return true
	default:
		return false
	}
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
