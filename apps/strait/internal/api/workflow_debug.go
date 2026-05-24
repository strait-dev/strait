package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
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

	if err := requireProjectMatch(ctx, wfRun.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
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

	if err := requireProjectMatch(ctx, runA.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run A not found")
	}
	if err := s.requireWorkflowRunReadAccess(ctx, runA.ID); err != nil {
		return nil, err
	}

	runB, err := s.store.GetWorkflowRun(ctx, input.OtherRunID)
	if err != nil {
		return nil, huma.Error404NotFound("workflow run B not found")
	}

	if err := requireProjectMatch(ctx, runB.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run B not found")
	}
	if err := s.requireWorkflowRunReadAccess(ctx, runB.ID); err != nil {
		return nil, err
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

func (s *Server) requireWorkflowRunReadAccess(ctx context.Context, workflowRunID string) error {
	if s.hasProjectPermission(ctx, domain.ScopeRunsRead) {
		return nil
	}
	if actorTypeFromContext(ctx) != "user" {
		return huma.Error403Forbidden("insufficient permissions: requires " + domain.ScopeRunsRead)
	}
	projectID := projectIDFromContext(ctx)
	actorID := actorFromContext(ctx)
	if projectID == "" || actorID == "" {
		return huma.Error403Forbidden("missing project or actor context")
	}
	actions, err := s.store.GetResourcePolicies(ctx, projectID, "workflow_run", workflowRunID, actorID)
	if err != nil || !domain.HasScopeStrict(actions, domain.ScopeRunsRead) {
		return huma.Error403Forbidden("insufficient permissions: requires " + domain.ScopeRunsRead)
	}
	return nil
}
