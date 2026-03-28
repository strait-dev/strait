package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/workflow"
)

type GetWorkflowRunDebugInput struct {
	WorkflowRunID string `path:"workflowRunID"`
}

type GetWorkflowRunDebugOutput struct {
	Body *workflow.DebugView
}

func (s *Server) handleGetWorkflowRunDebug(ctx context.Context, input *GetWorkflowRunDebugInput) (*GetWorkflowRunDebugOutput, error) {
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

	view, err := workflow.BuildDebugView(wfRun, steps, stepRuns, nil)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to build debug view")
	}

	return &GetWorkflowRunDebugOutput{Body: view}, nil
}

type CompareWorkflowRunsInput struct {
	WorkflowRunID string `path:"workflowRunID"`
	OtherRunID    string `path:"otherRunID"`
}

type CompareWorkflowRunsOutput struct {
	Body *workflow.RunComparison
}

func (s *Server) handleCompareWorkflowRuns(ctx context.Context, input *CompareWorkflowRunsInput) (*CompareWorkflowRunsOutput, error) {
	runA, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		return nil, huma.Error404NotFound("workflow run A not found")
	}

	runB, err := s.store.GetWorkflowRun(ctx, input.OtherRunID)
	if err != nil {
		return nil, huma.Error404NotFound("workflow run B not found")
	}

	stepsA, err := s.store.ListStepRunsByWorkflowRun(ctx, runA.ID, 1000, nil)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load step runs for run A")
	}

	stepsB, err := s.store.ListStepRunsByWorkflowRun(ctx, runB.ID, 1000, nil)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load step runs for run B")
	}

	comp := workflow.CompareRuns(runA, stepsA, runB, stepsB)
	return &CompareWorkflowRunsOutput{Body: comp}, nil
}
