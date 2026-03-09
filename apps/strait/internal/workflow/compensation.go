package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"strait/internal/domain"
)

// maxStepRunsPerQuery is the upper bound on step runs fetched for compensation.
// Workflows with more step runs than this will have incomplete compensation.
const maxStepRunsPerQuery = 10000

// CompensationEngine handles the Saga compensation pattern for workflow runs.
// When a workflow is canceled or fails, it walks completed steps in reverse
// chronological order and triggers their compensation steps.
type CompensationEngine struct {
	store  CompensationStore
	queue  EngineQueue
	logger *slog.Logger
}

// CompensationStore defines the store operations needed for compensation.
type CompensationStore interface {
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error)
	ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error)
	UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	CreateWorkflowStepRun(ctx context.Context, sr *domain.WorkflowStepRun) error
}

// CompensationResult describes the outcome of a compensation run.
type CompensationResult struct {
	// Compensated is the list of step refs that were successfully enqueued.
	Compensated []string
	// Failed is the list of step refs that failed to enqueue.
	Failed []string
	// Status is the final compensation status.
	Status domain.CompensationStatus
}

// NewCompensationEngine creates a new compensation engine.
func NewCompensationEngine(store CompensationStore, queue EngineQueue, logger *slog.Logger) *CompensationEngine {
	if logger == nil {
		logger = slog.Default()
	}
	return &CompensationEngine{
		store:  store,
		queue:  queue,
		logger: logger,
	}
}

// CancelWorkflowRun cancels a running workflow and triggers compensation
// for completed steps that have compensation steps defined.
func (c *CompensationEngine) CancelWorkflowRun(ctx context.Context, workflowRunID string) (*CompensationResult, error) {
	wfRun, err := c.store.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return nil, fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun == nil {
		return nil, fmt.Errorf("workflow run not found: %s", workflowRunID)
	}
	if wfRun.Status.IsTerminal() {
		return nil, fmt.Errorf("workflow run %s is already in terminal state: %s", workflowRunID, wfRun.Status)
	}

	// Cancel running/waiting steps first
	if err := c.cancelActiveSteps(ctx, workflowRunID); err != nil {
		return nil, fmt.Errorf("cancel active steps: %w", err)
	}

	// Run compensation for completed steps
	result, err := c.runCompensation(ctx, wfRun)
	if err != nil {
		c.logger.Error("compensation failed", "workflow_run_id", workflowRunID, "error", err)
		// Don't fail the cancel — mark as canceled anyway
	}

	// Mark workflow as canceled
	now := time.Now()
	cancelFields := map[string]any{
		"finished_at": now,
		"error":       "canceled with compensation",
	}
	if result != nil {
		cancelFields["compensation_status"] = string(result.Status)
		cancelFields["compensation_steps_total"] = len(result.Compensated) + len(result.Failed)
		cancelFields["compensation_steps_completed"] = len(result.Compensated)
	}
	if err := c.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, domain.WfStatusCanceled, cancelFields); err != nil {
		return result, fmt.Errorf("mark workflow canceled: %w", err)
	}

	return result, nil
}

// CompensateFailedWorkflow triggers compensation for a workflow that has
// already been marked as failed. This is called when a step fails with
// FailWorkflow policy.
func (c *CompensationEngine) CompensateFailedWorkflow(ctx context.Context, workflowRunID string) (*CompensationResult, error) {
	wfRun, err := c.store.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return nil, fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun == nil {
		return nil, fmt.Errorf("workflow run not found: %s", workflowRunID)
	}

	return c.runCompensation(ctx, wfRun)
}

// RetryFailedCompensation retries compensation for steps that previously
// failed to enqueue. It looks for compensation step runs in a failed state
// and re-enqueues them. This is idempotent — already-completed compensation
// steps are skipped.
func (c *CompensationEngine) RetryFailedCompensation(ctx context.Context, workflowRunID string) (*CompensationResult, error) {
	wfRun, err := c.store.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return nil, fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun == nil {
		return nil, fmt.Errorf("workflow run not found: %s", workflowRunID)
	}

	if wfRun.CompensationStatus != domain.CompensationPartial &&
		wfRun.CompensationStatus != domain.CompensationFailed {
		return nil, fmt.Errorf("workflow run %s compensation status is %s, not retryable", workflowRunID, wfRun.CompensationStatus)
	}

	steps, err := c.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return nil, fmt.Errorf("list workflow steps: %w", err)
	}

	stepRuns, err := c.store.ListStepRunsByWorkflowRun(ctx, wfRun.ID, maxStepRunsPerQuery, nil)
	if err != nil {
		return nil, fmt.Errorf("list step runs: %w", err)
	}

	// Build step ref -> step map
	stepByRef := make(map[string]domain.WorkflowStep, len(steps))
	for _, s := range steps {
		stepByRef[s.StepRef] = s
	}

	// Find compensation step runs that are in a failed/canceled state
	// and need to be retried.
	var compensated, failed []string
	for _, sr := range stepRuns {
		step, ok := stepByRef[sr.StepRef]
		if !ok {
			continue
		}
		// Only look at compensation steps (steps that are referenced as
		// compensate_step_ref by another step)
		isCompensationStep := false
		for _, s := range steps {
			if s.CompensateStepRef == sr.StepRef {
				isCompensationStep = true
				break
			}
		}
		if !isCompensationStep {
			continue
		}

		// Skip already-completed compensation
		if sr.Status == domain.StepCompleted {
			compensated = append(compensated, sr.StepRef)
			continue
		}

		// Only retry failed/canceled compensation step runs
		if sr.Status != domain.StepFailed && sr.Status != domain.StepCanceled {
			continue
		}

		c.logger.Info("retrying compensation step",
			"workflow_run_id", wfRun.ID,
			"step_ref", sr.StepRef,
		)

		if step.JobID != "" {
			jobRun := &domain.JobRun{
				JobID:             step.JobID,
				ProjectID:         wfRun.ProjectID,
				Status:            domain.StatusQueued,
				Attempt:           sr.Attempt + 1,
				Payload:           sr.Output, // original output from the step being compensated
				TriggeredBy:       "compensation_retry",
				WorkflowStepRunID: sr.ID,
			}

			if err := c.queue.Enqueue(ctx, jobRun); err != nil {
				c.logger.Error("failed to re-enqueue compensation job",
					"step_ref", sr.StepRef,
					"error", err,
				)
				failed = append(failed, sr.StepRef)
				continue
			}

			_ = c.store.UpdateStepRunStatus(ctx, sr.ID, domain.StepRunning, map[string]any{
				"job_run_id": jobRun.ID,
				"started_at": time.Now(),
				"attempt":    sr.Attempt + 1,
			})
		}

		compensated = append(compensated, sr.StepRef)
	}

	status := domain.CompensationCompleted
	if len(failed) > 0 && len(compensated) > 0 {
		status = domain.CompensationPartial
	} else if len(failed) > 0 {
		status = domain.CompensationFailed
	}

	// Update the workflow run's compensation tracking
	_ = c.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, wfRun.Status, map[string]any{
		"compensation_status":          string(status),
		"compensation_steps_completed": len(compensated),
	})

	return &CompensationResult{
		Compensated: compensated,
		Failed:      failed,
		Status:      status,
	}, nil
}

// cancelActiveSteps cancels all running and waiting step runs in a workflow.
func (c *CompensationEngine) cancelActiveSteps(ctx context.Context, workflowRunID string) error {
	stepRuns, err := c.store.ListStepRunsByWorkflowRun(ctx, workflowRunID, maxStepRunsPerQuery, nil)
	if err != nil {
		return fmt.Errorf("list step runs: %w", err)
	}

	now := time.Now()
	for _, sr := range stepRuns {
		if sr.Status.IsTerminal() {
			continue
		}

		if err := c.store.UpdateStepRunStatus(ctx, sr.ID, domain.StepCanceled, map[string]any{
			"finished_at": now,
			"error":       "workflow canceled",
		}); err != nil {
			c.logger.Error("failed to cancel step run",
				"step_run_id", sr.ID,
				"step_ref", sr.StepRef,
				"error", err,
			)
		}

		// If this step has a job run, cancel it too.
		// Try each possible non-terminal source status since we don't
		// know the run's current state without an extra DB lookup.
		if sr.JobRunID != "" {
			cancelFields := map[string]any{
				"finished_at": now,
				"error":       "workflow canceled",
			}
			cancelingFields := map[string]any{
				"error": "workflow canceled",
			}

			// Try executing/waiting → canceling (graceful)
			canceled := false
			for _, from := range []domain.RunStatus{domain.StatusExecuting, domain.StatusWaiting} {
				if err := c.store.UpdateRunStatus(ctx, sr.JobRunID, from, domain.StatusCanceling, cancelingFields); err == nil {
					canceled = true
					break
				}
			}
			// Fallback: try direct cancel from queued/dequeued/delayed
			if !canceled {
				for _, from := range []domain.RunStatus{domain.StatusQueued, domain.StatusDequeued, domain.StatusDelayed} {
					if err := c.store.UpdateRunStatus(ctx, sr.JobRunID, from, domain.StatusCanceled, cancelFields); err == nil {
						break
					}
				}
			}
		}
	}

	return nil
}

// runCompensation finds completed steps with compensation refs and triggers
// them in reverse chronological order.
func (c *CompensationEngine) runCompensation(ctx context.Context, wfRun *domain.WorkflowRun) (*CompensationResult, error) {
	steps, err := c.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return nil, fmt.Errorf("list workflow steps: %w", err)
	}

	stepRuns, err := c.store.ListStepRunsByWorkflowRun(ctx, wfRun.ID, maxStepRunsPerQuery, nil)
	if err != nil {
		return nil, fmt.Errorf("list step runs: %w", err)
	}

	// Build step ref -> step map
	stepByRef := make(map[string]domain.WorkflowStep, len(steps))
	for _, s := range steps {
		stepByRef[s.StepRef] = s
	}

	// Find completed steps that have compensation
	type completedWithCompensation struct {
		stepRun domain.WorkflowStepRun
		step    domain.WorkflowStep
	}

	var toCompensate []completedWithCompensation
	for _, sr := range stepRuns {
		if sr.Status != domain.StepCompleted {
			continue
		}
		step, ok := stepByRef[sr.StepRef]
		if !ok || step.CompensateStepRef == "" {
			continue
		}
		toCompensate = append(toCompensate, completedWithCompensation{
			stepRun: sr,
			step:    step,
		})
	}

	if len(toCompensate) == 0 {
		return &CompensationResult{Status: domain.CompensationNone}, nil
	}

	// Mark compensation as running
	_ = c.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, wfRun.Status, map[string]any{
		"compensation_status":      string(domain.CompensationRunning),
		"compensation_steps_total": len(toCompensate),
	})

	// Sort in reverse order by finish time (most recently completed first).
	// This gives us reverse execution order for the Saga pattern.
	// Steps with nil FinishedAt sort last (created_at fallback).
	sort.Slice(toCompensate, func(i, j int) bool {
		ti := toCompensate[i].stepRun.FinishedAt
		tj := toCompensate[j].stepRun.FinishedAt
		// Fallback to CreatedAt when FinishedAt is nil
		if ti == nil {
			ti = &toCompensate[i].stepRun.CreatedAt
		}
		if tj == nil {
			tj = &toCompensate[j].stepRun.CreatedAt
		}
		return ti.After(*tj)
	})

	var compensated, failed []string
	for _, tc := range toCompensate {
		compStepRef := tc.step.CompensateStepRef
		compStep, ok := stepByRef[compStepRef]
		if !ok {
			c.logger.Error("compensation step not found",
				"step_ref", tc.step.StepRef,
				"compensate_step_ref", compStepRef,
			)
			failed = append(failed, compStepRef)
			continue
		}

		c.logger.Info("triggering compensation",
			"workflow_run_id", wfRun.ID,
			"original_step", tc.step.StepRef,
			"compensation_step", compStepRef,
		)

		// Create a step run for the compensation step
		compStepRun := &domain.WorkflowStepRun{
			WorkflowRunID:  wfRun.ID,
			WorkflowStepID: compStep.ID,
			StepRef:        compStepRef,
			Attempt:        1,
			Status:         domain.StepPending,
			DepsCompleted:  0,
			DepsRequired:   0,
		}

		if err := c.store.CreateWorkflowStepRun(ctx, compStepRun); err != nil {
			c.logger.Error("failed to create compensation step run",
				"step_ref", compStepRef,
				"error", err,
			)
			failed = append(failed, compStepRef)
			continue
		}

		// Enqueue the compensation job with the original step's output as payload
		if compStep.JobID != "" {
			jobRun := &domain.JobRun{
				JobID:             compStep.JobID,
				ProjectID:         wfRun.ProjectID,
				Status:            domain.StatusQueued,
				Attempt:           1,
				Payload:           tc.stepRun.Output,
				TriggeredBy:       "compensation",
				WorkflowStepRunID: compStepRun.ID,
			}

			if err := c.queue.Enqueue(ctx, jobRun); err != nil {
				c.logger.Error("failed to enqueue compensation job",
					"step_ref", compStepRef,
					"error", err,
				)
				failed = append(failed, compStepRef)
				continue
			}

			if err := c.store.UpdateStepRunStatus(ctx, compStepRun.ID, domain.StepRunning, map[string]any{
				"job_run_id": jobRun.ID,
				"started_at": time.Now(),
			}); err != nil {
				c.logger.Error("failed to update compensation step run",
					"step_ref", compStepRef,
					"error", err,
				)
			}
		}

		compensated = append(compensated, compStepRef)
	}

	// Determine final compensation status
	status := domain.CompensationCompleted
	if len(failed) > 0 && len(compensated) > 0 {
		status = domain.CompensationPartial
	} else if len(failed) > 0 && len(compensated) == 0 {
		status = domain.CompensationFailed
	}

	// Update compensation tracking
	_ = c.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, wfRun.Status, map[string]any{
		"compensation_status":          string(status),
		"compensation_steps_completed": len(compensated),
	})

	return &CompensationResult{
		Compensated: compensated,
		Failed:      failed,
		Status:      status,
	}, nil
}
