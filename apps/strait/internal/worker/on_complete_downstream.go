package worker

import (
	"context"
	"encoding/json"

	"strait/internal/domain"
	"strait/internal/queue"
)

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

// JobLookup resolves a job by slug within a project.
type JobLookup interface {
	GetJobBySlug(ctx context.Context, projectID, slug string) (*domain.Job, error)
}

// JobEnqueuer enqueues a new job run.
type JobEnqueuer interface {
	Enqueue(ctx context.Context, run *domain.JobRun) error
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

	if enqueueErr := queue.EnqueueWithRetry(ctx, t.jobEnqueuer, downstreamRun, queue.DefaultInternalEnqueueRetryConfig()); enqueueErr != nil {
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
