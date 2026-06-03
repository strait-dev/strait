package api

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/workflow"
)

type workflowCompensationRunStore interface {
	CreateCompensationRun(ctx context.Context, run *domain.CompensationRun) error
	MarkCompensationRunStarted(ctx context.Context, id, jobRunID string, startedAt time.Time) error
}

type CompensateWorkflowRunInput struct {
	WorkflowRunID string `path:"workflowRunID"`
}

type CompensateWorkflowRunOutput struct {
	Body *workflow.CompensationPlan
}

func (s *Server) handleCompensateWorkflowRun(ctx context.Context, input *CompensateWorkflowRunInput) (*CompensateWorkflowRunOutput, error) {
	wfRun, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	if err := requireProjectMatch(ctx, wfRun.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	if err := s.checkFeatureAllowed(ctx, wfRun.ProjectID, billing.FeatureCompensatingTxns, "Compensating transactions"); err != nil {
		return nil, err
	}

	if err := workflow.ValidateCompensationRequest(wfRun); err != nil {
		workflow.RecordWorkflowCompensation(ctx, "skipped")
		return nil, huma.Error400BadRequest(err.Error())
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		workflow.RecordWorkflowCompensation(ctx, "failure")
		return nil, huma.Error500InternalServerError("failed to load workflow steps")
	}

	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, wfRun.ID, 1000, nil)
	if err != nil {
		workflow.RecordWorkflowCompensation(ctx, "failure")
		return nil, huma.Error500InternalServerError("failed to load step runs")
	}

	plan, err := workflow.BuildCompensationPlan(wfRun.ID, steps, stepRuns)
	if err != nil {
		workflow.RecordWorkflowCompensation(ctx, "failure")
		return nil, huma.Error500InternalServerError("failed to build compensation plan")
	}
	if plan == nil {
		workflow.RecordWorkflowCompensation(ctx, "skipped")
		return nil, huma.Error400BadRequest("no steps require compensation")
	}

	compensationRuns, jobRuns, err := buildManualCompensationRuns(wfRun, plan)
	if err != nil {
		workflow.RecordWorkflowCompensation(ctx, "failure")
		return nil, huma.Error500InternalServerError("failed to build compensation jobs")
	}

	if err := s.runInTx(ctx, func(txStore APIStore) error {
		if err := txStore.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, domain.WfStatusCompensating, map[string]any{
			"error": "compensation triggered manually",
		}); err != nil {
			return fmt.Errorf("start compensation: %w", err)
		}
		compStore, ok := txStore.(workflowCompensationRunStore)
		if !ok {
			return nil
		}
		for i := range compensationRuns {
			if err := compStore.CreateCompensationRun(ctx, &compensationRuns[i]); err != nil {
				return fmt.Errorf("create compensation run %s: %w", compensationRuns[i].StepRef, err)
			}
		}
		return nil
	}); err != nil {
		workflow.RecordWorkflowCompensation(ctx, "failure")
		return nil, huma.Error500InternalServerError("failed to start compensation")
	}

	for i := range jobRuns {
		if err := queue.EnqueueWithRetry(ctx, s.queue, jobRuns[i], queue.DefaultInternalEnqueueRetryConfig()); err != nil {
			workflow.RecordWorkflowCompensation(ctx, "failure")
			_ = s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusCompensating, domain.WfStatusCompensationFailed, map[string]any{
				"error": fmt.Sprintf("failed to enqueue compensation job for step %s", compensationRuns[i].StepRef),
			})
			return nil, huma.Error500InternalServerError("failed to enqueue compensation job")
		}
		if compStore, ok := s.store.(workflowCompensationRunStore); ok {
			if err := compStore.MarkCompensationRunStarted(ctx, compensationRuns[i].ID, jobRuns[i].ID, time.Now()); err != nil {
				workflow.RecordWorkflowCompensation(ctx, "failure")
				_ = s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusCompensating, domain.WfStatusCompensationFailed, map[string]any{
					"error": fmt.Sprintf("failed to persist compensation job for step %s", compensationRuns[i].StepRef),
				})
				return nil, huma.Error500InternalServerError("failed to persist compensation job")
			}
		}
	}
	workflow.RecordWorkflowCompensation(ctx, "success")

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowRunCompensated, "workflow_run", wfRun.ID, map[string]any{
		"workflow_id":     wfRun.WorkflowID,
		"previous_status": string(wfRun.Status),
		"plan_steps":      len(plan.Steps),
	})

	return &CompensateWorkflowRunOutput{Body: plan}, nil
}

func buildManualCompensationRuns(wfRun *domain.WorkflowRun, plan *workflow.CompensationPlan) ([]domain.CompensationRun, []*domain.JobRun, error) {
	if wfRun == nil || plan == nil || len(plan.Steps) == 0 {
		return nil, nil, fmt.Errorf("empty compensation plan")
	}
	compensationRuns := make([]domain.CompensationRun, 0, len(plan.Steps))
	jobRuns := make([]*domain.JobRun, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		compensationRunID := uuid.Must(uuid.NewV7()).String()
		jobRunID := uuid.Must(uuid.NewV7()).String()
		input, err := compensationJobPayload(wfRun.ID, step)
		if err != nil {
			return nil, nil, err
		}
		compensationRuns = append(compensationRuns, domain.CompensationRun{
			ID:                compensationRunID,
			WorkflowRunID:     wfRun.ID,
			StepRunID:         step.StepRunID,
			StepRef:           step.StepRef,
			CompensationJobID: step.CompensationJobID,
			JobRunID:          jobRunID,
			Status:            domain.CompensationPending,
			Input:             input,
		})
		jobRuns = append(jobRuns, &domain.JobRun{
			ID:                  jobRunID,
			JobID:               step.CompensationJobID,
			ProjectID:           wfRun.ProjectID,
			Tags:                maps.Clone(wfRun.Tags),
			Payload:             input,
			TriggeredBy:         domain.TriggerWorkflow,
			TimeoutSecsOverride: step.TimeoutSecs,
			CreatedBy:           "system:workflow-compensation",
			Metadata: map[string]string{
				domain.RunMetadataCompensationRunID:         compensationRunID,
				domain.RunMetadataCompensationWorkflowRunID: wfRun.ID,
				domain.RunMetadataCompensationStepRef:       step.StepRef,
			},
		})
	}
	return compensationRuns, jobRuns, nil
}

func compensationJobPayload(workflowRunID string, step workflow.CompensationStep) (json.RawMessage, error) {
	payload := map[string]any{
		"workflow_run_id":      workflowRunID,
		"step_run_id":          step.StepRunID,
		"step_ref":             step.StepRef,
		"compensation_job_id":  step.CompensationJobID,
		"original_step_output": step.OriginalOutput,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal compensation payload: %w", err)
	}
	return raw, nil
}

type GetCompensationPlanInput struct {
	WorkflowRunID string `path:"workflowRunID"`
}

type GetCompensationPlanOutput struct {
	Body *workflow.CompensationPlan
}

func (s *Server) handleGetCompensationPlan(ctx context.Context, input *GetCompensationPlanInput) (*GetCompensationPlanOutput, error) {
	wfRun, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	if err := requireProjectMatch(ctx, wfRun.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	if err := s.checkFeatureAllowed(ctx, wfRun.ProjectID, billing.FeatureCompensatingTxns, "Compensating transactions"); err != nil {
		return nil, err
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load workflow steps")
	}

	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, wfRun.ID, 1000, nil)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load step runs")
	}

	plan, err := workflow.BuildCompensationPlan(wfRun.ID, steps, stepRuns)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to build compensation plan")
	}
	if plan == nil {
		return nil, huma.Error404NotFound("no compensation plan available")
	}

	return &GetCompensationPlanOutput{Body: plan}, nil
}
