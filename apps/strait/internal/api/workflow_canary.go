package api

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"
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
	Body *domain.CanaryDeployment
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

	if err := requireProjectMatch(ctx, wf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	if err := s.checkFeatureAllowed(ctx, wf.ProjectID, billing.FeatureCanaryDeployments, "Canary deployments"); err != nil {
		return nil, err
	}

	canary := &domain.CanaryDeployment{
		WorkflowID:    req.WorkflowID,
		ProjectID:     wf.ProjectID,
		SourceVersion: req.SourceVersion,
		TargetVersion: req.TargetVersion,
		TrafficPct:    req.TrafficPct,
		Status:        "active",
		AutoPromote:   workflow.MarshalAutoPromoteConfig(req.AutoPromote),
	}

	if err := s.store.CreateCanaryDeployment(ctx, canary); err != nil {
		if errors.Is(err, store.ErrCanaryAlreadyActive) {
			return nil, huma.Error409Conflict("an active canary deployment already exists for this workflow")
		}
		return nil, huma.Error500InternalServerError("failed to create canary deployment")
	}

	s.emitAuditEvent(ctx, domain.AuditActionCanaryDeploymentCreated, "canary_deployment", canary.ID, map[string]any{
		"workflow_id":    req.WorkflowID,
		"source_version": req.SourceVersion,
		"target_version": req.TargetVersion,
		"traffic_pct":    req.TrafficPct,
	})

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
	Body *domain.CanaryDeployment
}

func (s *Server) handleUpdateCanaryDeployment(ctx context.Context, input *UpdateCanaryInput) (*UpdateCanaryOutput, error) {
	if err := s.validate.Struct(&input.Body); err != nil {
		return nil, newValidationError(err)
	}

	wf, err := s.store.GetWorkflow(ctx, input.WorkflowID)
	if err != nil || wf == nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	if err := requireProjectMatch(ctx, wf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	if err := s.checkFeatureAllowed(ctx, wf.ProjectID, billing.FeatureCanaryDeployments, "Canary deployments"); err != nil {
		return nil, err
	}

	if err := s.store.UpdateCanaryDeploymentTraffic(ctx, input.WorkflowID, input.Body.TrafficPct); err != nil {
		if errors.Is(err, store.ErrCanaryNotFound) {
			return nil, huma.Error404NotFound("no active canary deployment found for this workflow")
		}
		return nil, huma.Error500InternalServerError("failed to update canary deployment")
	}

	// If traffic reached 100%, complete the canary.
	if input.Body.TrafficPct >= 100 {
		if err := s.store.CompleteCanaryDeployment(ctx, input.WorkflowID, "completed"); err != nil {
			return nil, huma.Error500InternalServerError("failed to complete canary deployment")
		}
	}

	canary, err := s.store.GetActiveCanaryDeployment(ctx, input.WorkflowID)
	if err != nil && !errors.Is(err, store.ErrCanaryNotFound) {
		return nil, huma.Error500InternalServerError("failed to load canary deployment")
	}

	s.emitAuditEvent(ctx, domain.AuditActionCanaryDeploymentUpdated, "canary_deployment", input.WorkflowID, map[string]any{
		"workflow_id": input.WorkflowID,
		"traffic_pct": input.Body.TrafficPct,
	})

	return &UpdateCanaryOutput{Body: canary}, nil
}

type RollbackCanaryInput struct {
	WorkflowID string `path:"workflowID"`
}

type RollbackCanaryOutput struct {
	Body map[string]any
}

func (s *Server) handleRollbackCanaryDeployment(ctx context.Context, input *RollbackCanaryInput) (*RollbackCanaryOutput, error) {
	wf, err := s.store.GetWorkflow(ctx, input.WorkflowID)
	if err != nil || wf == nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	if err := requireProjectMatch(ctx, wf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	if err := s.checkFeatureAllowed(ctx, wf.ProjectID, billing.FeatureCanaryDeployments, "Canary deployments"); err != nil {
		return nil, err
	}

	// First set traffic to 0.
	if err := s.store.UpdateCanaryDeploymentTraffic(ctx, input.WorkflowID, 0); err != nil {
		if errors.Is(err, store.ErrCanaryNotFound) {
			return nil, huma.Error404NotFound("no active canary deployment found for this workflow")
		}
		return nil, huma.Error500InternalServerError("failed to rollback canary deployment")
	}

	// Then mark as rolled back.
	if err := s.store.CompleteCanaryDeployment(ctx, input.WorkflowID, "rolled_back"); err != nil {
		return nil, huma.Error500InternalServerError("failed to finalize canary rollback")
	}

	s.emitAuditEvent(ctx, domain.AuditActionCanaryDeploymentRolledBack, "canary_deployment", input.WorkflowID, map[string]any{
		"workflow_id": input.WorkflowID,
	})

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
	Body *domain.CanaryDeployment
}

func (s *Server) handleGetCanaryStatus(ctx context.Context, input *GetCanaryStatusInput) (*GetCanaryStatusOutput, error) {
	wf, err := s.store.GetWorkflow(ctx, input.WorkflowID)
	if err != nil || wf == nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	if err := requireProjectMatch(ctx, wf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	if err := s.checkFeatureAllowed(ctx, wf.ProjectID, billing.FeatureCanaryDeployments, "Canary deployments"); err != nil {
		return nil, err
	}

	canary, err := s.store.GetActiveCanaryDeployment(ctx, input.WorkflowID)
	if err != nil {
		if errors.Is(err, store.ErrCanaryNotFound) {
			return nil, huma.Error404NotFound("no active canary deployment found for this workflow")
		}
		return nil, huma.Error500InternalServerError("failed to load canary deployment")
	}

	return &GetCanaryStatusOutput{Body: canary}, nil
}
