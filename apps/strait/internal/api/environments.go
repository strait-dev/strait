package api

import (
	"context"
	"errors"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
	"strait/internal/store"
)

type CreateEnvironmentRequest struct {
	ProjectID string            `json:"project_id" validate:"required"`
	Name      string            `json:"name" validate:"required"`
	Slug      string            `json:"slug" validate:"required"`
	ParentID  string            `json:"parent_id,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
}

type UpdateEnvironmentRequest struct {
	Name      *string            `json:"name,omitempty"`
	Slug      *string            `json:"slug,omitempty"`
	ParentID  *string            `json:"parent_id,omitempty"`
	Variables *map[string]string `json:"variables,omitempty"`
}

type EnvironmentResponse struct {
	domain.Environment
	ResolvedVariables map[string]string `json:"resolved_variables,omitempty"`
}

// CreateEnvironmentInput is the typed input for creating an environment.
type CreateEnvironmentInput struct {
	Body CreateEnvironmentRequest
}

// CreateEnvironmentOutput is the typed output for creating an environment.
type CreateEnvironmentOutput struct {
	Body *domain.Environment
}

func (s *Server) handleCreateEnvironment(ctx context.Context, input *CreateEnvironmentInput) (*CreateEnvironmentOutput, error) {
	req := input.Body

	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	env := &domain.Environment{
		ProjectID: req.ProjectID,
		Name:      req.Name,
		Slug:      req.Slug,
		ParentID:  req.ParentID,
		Variables: req.Variables,
	}

	if err := s.store.CreateEnvironment(ctx, env); err != nil {
		return nil, huma.Error500InternalServerError("failed to create environment")
	}

	return &CreateEnvironmentOutput{Body: env}, nil
}

// GetEnvironmentInput is the typed input for getting an environment.
type GetEnvironmentInput struct {
	EnvID string `path:"envID"`
}

// GetEnvironmentOutput is the typed output for getting an environment.
type GetEnvironmentOutput struct {
	Body EnvironmentResponse
}

func (s *Server) handleGetEnvironment(ctx context.Context, input *GetEnvironmentInput) (*GetEnvironmentOutput, error) {
	env, err := s.store.GetEnvironment(ctx, input.EnvID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return nil, huma.Error404NotFound("environment not found")
		}
		return nil, huma.Error500InternalServerError("failed to get environment")
	}

	if err := requireProjectMatch(ctx, env.ProjectID); err != nil {
		return nil, huma.Error404NotFound("environment not found")
	}

	resolvedVariables, err := s.store.GetResolvedEnvironmentVariables(ctx, input.EnvID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return nil, huma.Error404NotFound("environment not found")
		}
		return nil, huma.Error500InternalServerError("failed to resolve environment variables")
	}

	return &GetEnvironmentOutput{Body: EnvironmentResponse{
		Environment:       *env,
		ResolvedVariables: resolvedVariables,
	}}, nil
}

// ListEnvironmentsInput is the typed input for listing environments.
type ListEnvironmentsInput struct {
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

// ListEnvironmentsOutput is the typed output for listing environments.
type ListEnvironmentsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListEnvironments(ctx context.Context, input *ListEnvironmentsInput) (*ListEnvironmentsOutput, error) {
	projectID := projectIDFromContext(ctx)

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	envs, err := s.store.ListEnvironments(ctx, projectID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list environments")
	}

	return &ListEnvironmentsOutput{Body: paginatedResult(envs, limit, func(e domain.Environment) string {
		return e.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

// UpdateEnvironmentInput is the typed input for updating an environment.
type UpdateEnvironmentInput struct {
	EnvID string `path:"envID"`
	Body  UpdateEnvironmentRequest
}

// UpdateEnvironmentOutput is the typed output for updating an environment.
type UpdateEnvironmentOutput struct {
	Body *domain.Environment
}

func (s *Server) handleUpdateEnvironment(ctx context.Context, input *UpdateEnvironmentInput) (*UpdateEnvironmentOutput, error) {
	env, err := s.store.GetEnvironment(ctx, input.EnvID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return nil, huma.Error404NotFound("environment not found")
		}
		return nil, huma.Error500InternalServerError("failed to get environment")
	}

	if err := requireProjectMatch(ctx, env.ProjectID); err != nil {
		return nil, huma.Error404NotFound("environment not found")
	}

	req := input.Body
	if req.Name != nil {
		env.Name = *req.Name
	}
	if req.Slug != nil {
		env.Slug = *req.Slug
	}
	if req.ParentID != nil {
		env.ParentID = *req.ParentID
	}
	if req.Variables != nil {
		env.Variables = *req.Variables
	}

	if err := s.store.UpdateEnvironment(ctx, env); err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return nil, huma.Error404NotFound("environment not found")
		}
		return nil, huma.Error500InternalServerError("failed to update environment")
	}

	return &UpdateEnvironmentOutput{Body: env}, nil
}

// DeleteEnvironmentInput is the typed input for deleting an environment.
type DeleteEnvironmentInput struct {
	EnvID string `path:"envID"`
}

func (s *Server) handleDeleteEnvironment(ctx context.Context, input *DeleteEnvironmentInput) (*struct{}, error) {
	if err := s.store.DeleteEnvironment(ctx, input.EnvID); err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return nil, huma.Error404NotFound("environment not found")
		}
		if errors.Is(err, store.ErrStandardEnvironment) {
			return nil, huma.Error403Forbidden("cannot delete standard environment")
		}
		return nil, huma.Error500InternalServerError("failed to delete environment")
	}

	return nil, nil
}

// GetResolvedVariablesInput is the typed input for getting resolved variables.
type GetResolvedVariablesInput struct {
	EnvID string `path:"envID"`
}

// GetResolvedVariablesOutput is the typed output for getting resolved variables.
type GetResolvedVariablesOutput struct {
	Body map[string]map[string]string
}

func (s *Server) handleGetResolvedVariables(ctx context.Context, input *GetResolvedVariablesInput) (*GetResolvedVariablesOutput, error) {
	resolvedVariables, err := s.store.GetResolvedEnvironmentVariables(ctx, input.EnvID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return nil, huma.Error404NotFound("environment not found")
		}
		return nil, huma.Error500InternalServerError("failed to resolve environment variables")
	}

	return &GetResolvedVariablesOutput{Body: map[string]map[string]string{"variables": resolvedVariables}}, nil
}
