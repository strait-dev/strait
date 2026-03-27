package api

import (
	"context"
	"encoding/json"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/workflow"
)

type CreateCanaryDeploymentRequest struct {
	WorkflowID    string                      `json:"workflow_id" validate:"required"`
	SourceVersion int                         `json:"source_version" validate:"required,min=1"`
	TargetVersion int                         `json:"target_version" validate:"required,min=1"`
	TrafficPct    int                         `json:"traffic_pct" validate:"min=0,max=100"`
	AutoPromote   *workflow.AutoPromoteConfig `json:"auto_promote,omitempty"`
}

type CreateCanaryInput struct {
	Body CreateCanaryDeploymentRequest
}

type CreateCanaryOutput struct {
	Body *workflow.CanaryDeployment
}

func (s *Server) handleCreateCanaryDeployment(ctx context.Context, input *CreateCanaryInput) (*CreateCanaryOutput, error) {
	req := input.Body

	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	if err := workflow.ValidateCanaryRequest(req.WorkflowID, req.SourceVersion, req.TargetVersion, req.TrafficPct); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	wf, err := s.store.GetWorkflow(ctx, req.WorkflowID)
	if err != nil || wf == nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	canary := &workflow.CanaryDeployment{
		WorkflowID:        req.WorkflowID,
		ProjectID:         wf.ProjectID,
		SourceVersion:     req.SourceVersion,
		TargetVersion:     req.TargetVersion,
		TrafficPct:        req.TrafficPct,
		Status:            workflow.CanaryActive,
		AutoPromoteConfig: req.AutoPromote,
	}

	return &CreateCanaryOutput{Body: canary}, nil
}

type UpdateCanaryRequest struct {
	TrafficPct int `json:"traffic_pct" validate:"min=0,max=100"`
}

type UpdateCanaryInput struct {
	WorkflowID string `path:"workflowID"`
	Body       UpdateCanaryRequest
}

type UpdateCanaryOutput struct {
	Body map[string]any
}

func (s *Server) handleUpdateCanaryDeployment(_ context.Context, input *UpdateCanaryInput) (*UpdateCanaryOutput, error) {
	if err := s.validate.Struct(&input.Body); err != nil {
		return nil, newValidationError(err)
	}

	return &UpdateCanaryOutput{Body: map[string]any{
		"workflow_id": input.WorkflowID,
		"traffic_pct": input.Body.TrafficPct,
		"status":      "active",
	}}, nil
}

type RollbackCanaryInput struct {
	WorkflowID string `path:"workflowID"`
}

type RollbackCanaryOutput struct {
	Body map[string]any
}

func (s *Server) handleRollbackCanaryDeployment(_ context.Context, input *RollbackCanaryInput) (*RollbackCanaryOutput, error) {
	return &RollbackCanaryOutput{Body: map[string]any{
		"workflow_id": input.WorkflowID,
		"traffic_pct": 0,
		"status":      "rolled_back",
	}}, nil
}

type GetCanaryStatusInput struct {
	WorkflowID string `path:"workflowID"`
}

type GetCanaryStatusOutput struct {
	Body map[string]any
}

func (s *Server) handleGetCanaryStatus(_ context.Context, input *GetCanaryStatusInput) (*GetCanaryStatusOutput, error) {
	// Health evaluation placeholder -- real implementation reads from canary_deployments table.
	health := workflow.CanaryHealthCheck{}
	decision := workflow.EvaluateHealth(health, nil)

	return &GetCanaryStatusOutput{Body: map[string]any{
		"workflow_id": input.WorkflowID,
		"status":      "none",
		"decision":    string(decision),
		"health":      json.RawMessage(`{}`),
	}}, nil
}
