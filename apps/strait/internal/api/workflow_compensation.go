package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/workflow"
)

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

	if err := s.checkFeatureAllowed(ctx, wfRun.ProjectID, billing.FeatureCompensatingTxns, "Compensating transactions"); err != nil {
		return nil, err
	}

	if err := workflow.ValidateCompensationRequest(wfRun); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	steps, err := s.loadWorkflowRunSteps(ctx, wfRun)
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
		return nil, huma.Error400BadRequest("no steps require compensation")
	}

	// Transition to compensating.
	if err := s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, domain.WfStatusCompensating, map[string]any{
		"error": "compensation triggered manually",
	}); err != nil {
		return nil, huma.Error500InternalServerError("failed to start compensation")
	}

	return &CompensateWorkflowRunOutput{Body: plan}, nil
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

	steps, err := s.loadWorkflowRunSteps(ctx, wfRun)
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
