package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

type createDeploymentVersionRequest struct {
	ProjectID      string `json:"project_id" validate:"required"`
	Environment    string `json:"environment" validate:"required"`
	Runtime        string `json:"runtime" validate:"required"`
	ArtifactURI    string `json:"artifact_uri" validate:"required,url"`
	Manifest       any    `json:"manifest"`
	Checksum       string `json:"checksum"`
	Strategy       string `json:"strategy"`
	CanaryPercent  *int   `json:"canary_percent"`
	CanaryDuration string `json:"canary_duration"`
}

type deploymentVersionMutationRequest struct {
	ProjectID   string `json:"project_id" validate:"required"`
	Environment string `json:"environment" validate:"required"`
}

func marshalRaw(value any) json.RawMessage {
	if value == nil {
		return json.RawMessage(`{}`)
	}
	b, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(b)
}

type CreateDeploymentVersionInput struct {
	Body createDeploymentVersionRequest
}

type CreateDeploymentVersionOutput struct {
	Body *domain.DeploymentVersion
}

func (s *Server) handleCreateDeploymentVersion(ctx context.Context, input *CreateDeploymentVersionInput) (*CreateDeploymentVersionOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}
	status := domain.DeploymentVersionStatusDraft
	if req.Runtime == "" {
		req.Runtime = "node"
	}
	strategy := domain.DeploymentStrategyDirect
	if req.Strategy != "" {
		strategy = domain.DeploymentStrategy(req.Strategy)
		if !strategy.IsValid() {
			return nil, huma.Error400BadRequest("invalid strategy: must be \"direct\" or \"canary\"")
		}
	}
	if strategy == domain.DeploymentStrategyCanary {
		if err := s.checkFeatureAllowed(ctx, req.ProjectID, billing.FeatureCanaryDeployments, "Canary deployments"); err != nil {
			return nil, err
		}
		if !validCanaryPercent(req.CanaryPercent) {
			return nil, huma.Error400BadRequest("canary strategy requires canary_percent between 1 and 99")
		}
	}
	var canaryDuration *time.Duration
	if req.CanaryDuration != "" {
		d, parseErr := time.ParseDuration(req.CanaryDuration)
		if parseErr != nil {
			return nil, huma.Error400BadRequest("invalid canary_duration: must be a valid Go duration string")
		}
		canaryDuration = &d
	}
	manifest := marshalRaw(req.Manifest)
	deployment := &domain.DeploymentVersion{
		ProjectID: req.ProjectID, Environment: req.Environment, Runtime: req.Runtime,
		ArtifactURI: req.ArtifactURI, Manifest: manifest, Checksum: req.Checksum,
		Status: status, Strategy: strategy, CanaryPercent: req.CanaryPercent,
		CanaryDuration: canaryDuration, CreatedBy: actorFromContext(ctx), UpdatedBy: actorFromContext(ctx),
	}
	if err := s.store.CreateDeploymentVersion(ctx, deployment); err != nil {
		return nil, huma.Error500InternalServerError("failed to create deployment version")
	}
	s.emitAuditEvent(ctx, domain.AuditActionDeploymentVersionCreated, "deployment_version", deployment.ID, map[string]any{
		"environment": deployment.Environment,
		"runtime":     deployment.Runtime,
		"strategy":    string(deployment.Strategy),
	})
	return &CreateDeploymentVersionOutput{Body: deployment}, nil
}

func validCanaryPercent(percent *int) bool {
	return percent != nil && *percent >= 1 && *percent <= 99
}

type ListDeploymentVersionsInput struct {
	Environment string `query:"environment"`
	Limit       string `query:"limit"`
	Cursor      string `query:"cursor"`
}

type ListDeploymentVersionsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListDeploymentVersions(ctx context.Context, input *ListDeploymentVersionsInput) (*ListDeploymentVersionsOutput, error) {
	projectID := projectIDFromContext(ctx)
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	versions, err := s.store.ListDeploymentVersions(ctx, projectID, input.Environment, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list deployment versions")
	}
	return &ListDeploymentVersionsOutput{Body: paginatedResult(versions, limit, func(v domain.DeploymentVersion) string {
		return v.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

type FinalizeDeploymentVersionInput struct {
	DeploymentID string `path:"deploymentID"`
	Body         deploymentVersionMutationRequest
}

type FinalizeDeploymentVersionOutput struct {
	Body *domain.DeploymentVersion
}

func (s *Server) handleFinalizeDeploymentVersion(ctx context.Context, input *FinalizeDeploymentVersionInput) (*FinalizeDeploymentVersionOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}
	deployment, err := s.store.FinalizeDeploymentVersion(ctx, input.DeploymentID, req.ProjectID, actorFromContext(ctx))
	if err != nil {
		if errors.Is(err, store.ErrDeploymentVersionNotFound) {
			return nil, huma.Error404NotFound("deployment version not found")
		}
		return nil, huma.Error500InternalServerError("failed to finalize deployment version")
	}
	s.emitAuditEvent(ctx, domain.AuditActionDeploymentVersionFinalized, "deployment_version", deployment.ID, map[string]any{
		"environment": deployment.Environment,
	})
	return &FinalizeDeploymentVersionOutput{Body: deployment}, nil
}

type PromoteDeploymentVersionInput struct {
	DeploymentID string `path:"deploymentID"`
	Body         deploymentVersionMutationRequest
}

type PromoteDeploymentVersionOutput struct {
	Body *domain.DeploymentVersion
}

func (s *Server) handlePromoteDeploymentVersion(ctx context.Context, input *PromoteDeploymentVersionInput) (*PromoteDeploymentVersionOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}
	deployment, err := s.store.PromoteDeploymentVersion(ctx, input.DeploymentID, req.ProjectID, req.Environment, actorFromContext(ctx))
	if err != nil {
		if errors.Is(err, store.ErrDeploymentVersionNotFound) {
			return nil, huma.Error404NotFound("deployment version not found")
		}
		return nil, huma.Error500InternalServerError("failed to promote deployment version")
	}
	s.emitAuditEvent(ctx, domain.AuditActionDeploymentVersionPromoted, "deployment_version", deployment.ID, map[string]any{
		"environment": req.Environment,
	})
	return &PromoteDeploymentVersionOutput{Body: deployment}, nil
}

type RollbackDeploymentVersionInput struct {
	DeploymentID string `path:"deploymentID"`
	Body         deploymentVersionMutationRequest
}

type RollbackDeploymentVersionOutput struct {
	Body *domain.DeploymentVersion
}

func (s *Server) handleRollbackDeploymentVersion(ctx context.Context, input *RollbackDeploymentVersionInput) (*RollbackDeploymentVersionOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}
	deployment, err := s.store.RollbackDeploymentVersion(ctx, input.DeploymentID, req.ProjectID, req.Environment, actorFromContext(ctx))
	if err != nil {
		if errors.Is(err, store.ErrDeploymentVersionNotFound) {
			return nil, huma.Error404NotFound("deployment version not found")
		}
		return nil, huma.Error500InternalServerError("failed to rollback deployment version")
	}
	s.emitAuditEvent(ctx, domain.AuditActionDeploymentVersionRolledBack, "deployment_version", deployment.ID, map[string]any{
		"environment": req.Environment,
	})
	return &RollbackDeploymentVersionOutput{Body: deployment}, nil
}
