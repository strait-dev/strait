package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"strait/internal/domain"
)

// CompensationEngine handles the Saga compensation pattern for workflow runs.
// When a workflow is canceled or fails, it walks completed steps in reverse
// topological order and triggers their compensation steps.
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
// Returns the list of compensation steps that were triggered.
func (c *CompensationEngine) CancelWorkflowRun(ctx context.Context, workflowRunID string) ([]string, error) {
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
	compensated, err := c.runCompensation(ctx, wfRun)
	if err != nil {
		c.logger.Error("compensation failed", "workflow_run_id", workflowRunID, "error", err)
		// Don't fail the cancel — mark as canceled anyway
	}

	// Mark workflow as canceled
	now := time.Now()
	if err := c.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, domain.WfStatusCanceled, map[string]any{
		"finished_at": now,
		"error":       "canceled with compensation",
	}); err != nil {
		return compensated, fmt.Errorf("mark workflow canceled: %w", err)
	}

	return compensated, nil
}

// CompensateFailedWorkflow triggers compensation for a workflow that has
// already been marked as failed. This is called when a step fails with
// FailWorkflow policy.
func (c *CompensationEngine) CompensateFailedWorkflow(ctx context.Context, workflowRunID string) ([]string, error) {
	wfRun, err := c.store.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return nil, fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun == nil {
		return nil, fmt.Errorf("workflow run not found: %s", workflowRunID)
	}

	return c.runCompensation(ctx, wfRun)
}

// cancelActiveSteps cancels all running and waiting step runs in a workflow.
func (c *CompensationEngine) cancelActiveSteps(ctx context.Context, workflowRunID string) error {
	stepRuns, err := c.store.ListStepRunsByWorkflowRun(ctx, workflowRunID, 10000, nil)
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

		// If this step has a job run, cancel it too
		if sr.JobRunID != "" {
			if err := c.store.UpdateRunStatus(ctx, sr.JobRunID, domain.StatusExecuting, domain.StatusCanceling, map[string]any{
				"error": "workflow canceled",
			}); err != nil {
				// Try other source statuses
				_ = c.store.UpdateRunStatus(ctx, sr.JobRunID, domain.StatusQueued, domain.StatusCanceled, map[string]any{
					"finished_at": now,
					"error":       "workflow canceled",
				})
			}
		}
	}

	return nil
}

// runCompensation finds completed steps with compensation refs and triggers
// them in reverse topological order.
func (c *CompensationEngine) runCompensation(ctx context.Context, wfRun *domain.WorkflowRun) ([]string, error) {
	steps, err := c.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return nil, fmt.Errorf("list workflow steps: %w", err)
	}

	stepRuns, err := c.store.ListStepRunsByWorkflowRun(ctx, wfRun.ID, 10000, nil)
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
		return nil, nil
	}

	// Sort in reverse order by finish time (most recently completed first)
	// This gives us reverse execution order for the Saga
	sort.Slice(toCompensate, func(i, j int) bool {
		ti := toCompensate[i].stepRun.FinishedAt
		tj := toCompensate[j].stepRun.FinishedAt
		if ti == nil || tj == nil {
			return false
		}
		return ti.After(*tj)
	})

	var compensated []string
	for _, tc := range toCompensate {
		compStepRef := tc.step.CompensateStepRef
		compStep, ok := stepByRef[compStepRef]
		if !ok {
			c.logger.Error("compensation step not found",
				"step_ref", tc.step.StepRef,
				"compensate_step_ref", compStepRef,
			)
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

	return compensated, nil
}
