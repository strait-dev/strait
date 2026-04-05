package api

import (
	"context"
	"encoding/json"
	"log/slog"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

// Submit eval results.

type SubmitEvalInput struct {
	AgentID string `path:"agentID"`
	Body    SubmitEvalRequest
}
type SubmitEvalRequest struct {
	DeploymentID string          `json:"deployment_id"`
	SuiteName    string          `json:"suite_name" validate:"required"`
	Results      json.RawMessage `json:"results" validate:"required"`
	TotalCases   int             `json:"total_cases"`
	PassedCases  int             `json:"passed_cases"`
	FailedCases  int             `json:"failed_cases"`
	DurationMs   int             `json:"duration_ms"`
}
type SubmitEvalOutput struct {
	Body *domain.EvalRun
}

func (s *Server) handleSubmitEval(ctx context.Context, input *SubmitEvalInput) (*SubmitEvalOutput, error) {
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	deploymentID := req.DeploymentID
	if deploymentID == "" {
		deploymentID = "latest"
	}

	run := &domain.EvalRun{
		AgentID:      input.AgentID,
		DeploymentID: deploymentID,
		ProjectID:    projectID,
		SuiteName:    req.SuiteName,
		ResultsJSON:  req.Results,
		TotalCases:   req.TotalCases,
		PassedCases:  req.PassedCases,
		FailedCases:  req.FailedCases,
		DurationMs:   req.DurationMs,
		Status:       "completed",
	}

	if err := s.store.CreateEvalRun(ctx, run); err != nil {
		slog.Error("failed to create eval run", "agent_id", input.AgentID, "error", err)
		return nil, huma.Error500InternalServerError("failed to store eval results")
	}

	return &SubmitEvalOutput{Body: run}, nil
}

// List eval runs.

type ListEvalsInput struct {
	AgentID string `path:"agentID"`
	Limit   int    `query:"limit"`
}
type ListEvalsOutput struct {
	Body []domain.EvalRun
}

func (s *Server) handleListEvals(ctx context.Context, input *ListEvalsInput) (*ListEvalsOutput, error) {
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}

	runs, err := s.store.ListEvalRuns(ctx, input.AgentID, limit)
	if err != nil {
		slog.Error("failed to list eval runs", "agent_id", input.AgentID, "error", err)
		return nil, huma.Error500InternalServerError("failed to list eval runs")
	}

	if runs == nil {
		runs = []domain.EvalRun{}
	}

	return &ListEvalsOutput{Body: runs}, nil
}
