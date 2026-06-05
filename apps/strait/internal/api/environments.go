package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
	"strait/internal/store"
)

type CreateEnvironmentRequest struct {
	ProjectID string            `json:"project_id" validate:"required"`
	Name      string            `json:"name" validate:"required,max=255"`
	Slug      string            `json:"slug" validate:"required,max=255"`
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
	ID                   string    `json:"id"`
	ProjectID            string    `json:"project_id"`
	Name                 string    `json:"name"`
	Slug                 string    `json:"slug"`
	ParentID             string    `json:"parent_id,omitempty"`
	IsStandard           bool      `json:"is_standard"`
	VariableKeys         []string  `json:"variable_keys,omitempty"`
	ResolvedVariableKeys []string  `json:"resolved_variable_keys,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// CreateEnvironmentInput is the typed input for creating an environment.
type CreateEnvironmentInput struct {
	Body CreateEnvironmentRequest
}

// CreateEnvironmentOutput is the typed output for creating an environment.
type CreateEnvironmentOutput struct {
	Body EnvironmentResponse
}

func (s *Server) handleCreateEnvironment(ctx context.Context, input *CreateEnvironmentInput) (*CreateEnvironmentOutput, error) {
	req := input.Body

	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}
	if environmentIDFromContext(ctx) != "" {
		return nil, huma.Error403Forbidden("environment-scoped credentials cannot create environments")
	}

	orgID, maxEnvironments, displayName, err := s.resolveEnvironmentCreateLimit(ctx, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if err := s.validateEnvironmentParent(ctx, req.ProjectID, "", req.ParentID); err != nil {
		return nil, err
	}
	if len(req.Variables) > 0 {
		if err := s.requireEnvironmentVariableWrite(ctx); err != nil {
			return nil, err
		}
	}

	env := &domain.Environment{
		ProjectID: req.ProjectID,
		Name:      req.Name,
		Slug:      req.Slug,
		ParentID:  req.ParentID,
		Variables: req.Variables,
	}

	var createErr error
	if creator, ok := s.store.(environmentOrgLimitCreator); ok {
		createErr = creator.CreateEnvironmentWithOrgLimit(ctx, env, orgID, maxEnvironments)
	} else {
		if err := s.checkEnvironmentLimit(ctx, req.ProjectID); err != nil {
			return nil, err
		}
		createErr = s.store.CreateEnvironment(ctx, env)
	}
	if createErr != nil {
		if errors.Is(createErr, store.ErrEnvironmentLimitExceeded) {
			return nil, huma.Error400BadRequest(
				fmt.Sprintf("Your %s plan allows %d environments. Upgrade at /settings/billing", displayName, maxEnvironments),
			)
		}
		return nil, huma.Error500InternalServerError("failed to create environment")
	}

	s.emitAuditEvent(ctx, domain.AuditActionEnvironmentCreated, "environment", env.ID, map[string]any{
		"name":          env.Name,
		"slug":          env.Slug,
		"parent_id":     env.ParentID,
		"variable_keys": tagKeys(env.Variables),
	})

	return &CreateEnvironmentOutput{Body: environmentResponse(env, nil)}, nil
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
	projectID := projectIDFromContext(ctx)
	env, err := s.store.GetEnvironment(ctx, input.EnvID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return nil, huma.Error404NotFound("environment not found")
		}
		return nil, huma.Error500InternalServerError("failed to get environment")
	}

	if err := requireProjectMatch(ctx, env.ProjectID); err != nil {
		return nil, huma.Error404NotFound("environment not found")
	}
	if err := requireEnvironmentMatch(ctx, env.ID); err != nil {
		return nil, huma.Error404NotFound("environment not found")
	}

	resolvedVariables, err := s.store.GetResolvedEnvironmentVariables(ctx, input.EnvID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return nil, huma.Error404NotFound("environment not found")
		}
		return nil, huma.Error500InternalServerError("failed to resolve environment variables")
	}

	return &GetEnvironmentOutput{Body: environmentResponse(env, resolvedVariables)}, nil
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
	if environmentID := environmentIDFromContext(ctx); environmentID != "" {
		filtered := envs[:0]
		for _, env := range envs {
			if env.ID == environmentID {
				filtered = append(filtered, env)
			}
		}
		envs = filtered
	}

	responses := make([]EnvironmentResponse, 0, len(envs))
	for i := range envs {
		responses = append(responses, environmentResponse(&envs[i], nil))
	}

	return &ListEnvironmentsOutput{Body: paginatedResult(responses, limit, func(e EnvironmentResponse) string {
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
	Body EnvironmentResponse
}

func (s *Server) handleUpdateEnvironment(ctx context.Context, input *UpdateEnvironmentInput) (*UpdateEnvironmentOutput, error) {
	projectID := projectIDFromContext(ctx)
	env, err := s.store.GetEnvironment(ctx, input.EnvID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return nil, huma.Error404NotFound("environment not found")
		}
		return nil, huma.Error500InternalServerError("failed to get environment")
	}

	if err := requireProjectMatch(ctx, env.ProjectID); err != nil {
		return nil, huma.Error404NotFound("environment not found")
	}
	if err := requireEnvironmentMatch(ctx, env.ID); err != nil {
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
		if err := s.validateEnvironmentParent(ctx, env.ProjectID, env.ID, *req.ParentID); err != nil {
			return nil, err
		}
		env.ParentID = *req.ParentID
	}
	if req.Variables != nil {
		if err := s.requireEnvironmentVariableWrite(ctx); err != nil {
			return nil, err
		}
		env.Variables = *req.Variables
	}

	if err := s.store.UpdateEnvironment(ctx, env); err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return nil, huma.Error404NotFound("environment not found")
		}
		return nil, huma.Error500InternalServerError("failed to update environment")
	}

	changedFields := []string{}
	if req.Name != nil {
		changedFields = append(changedFields, "name")
	}
	if req.Slug != nil {
		changedFields = append(changedFields, "slug")
	}
	if req.ParentID != nil {
		changedFields = append(changedFields, "parent_id")
	}
	if req.Variables != nil {
		changedFields = append(changedFields, "variables")
	}
	s.emitAuditEvent(ctx, domain.AuditActionEnvironmentUpdated, "environment", env.ID, map[string]any{
		"name":           env.Name,
		"changed_fields": changedFields,
		"variable_keys":  tagKeys(env.Variables),
	})

	return &UpdateEnvironmentOutput{Body: environmentResponse(env, nil)}, nil
}

func (s *Server) requireEnvironmentVariableWrite(ctx context.Context) error {
	if isInternalCaller(ctx) {
		return nil
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return huma.Error403Forbidden("environment variables require project context")
	}

	scopes := scopesFromContext(ctx)
	switch actorTypeFromContext(ctx) {
	case "api_key":
		if !domain.HasScopeStrict(scopes, domain.ScopeSecretsWrite) {
			return huma.Error403Forbidden("environment variables require secrets:write")
		}
		return nil
	case "user":
		actorID := actorFromContext(ctx)
		if actorID == "" {
			return huma.Error403Forbidden("environment variables require actor context")
		}
		if ctx.Value(ctxOIDCScopeClaimPresentKey) == true && len(scopes) == 0 {
			return huma.Error403Forbidden("environment variables require secrets:write")
		}
		if len(scopes) > 0 && !domain.HasScopeStrict(scopes, domain.ScopeSecretsWrite) {
			return huma.Error403Forbidden("environment variables require secrets:write")
		}
		perms, err := s.store.GetUserPermissions(ctx, projectID, actorID)
		if err != nil {
			return huma.Error500InternalServerError("failed to load permissions")
		}
		if !domain.HasScopeStrict(perms, domain.ScopeSecretsWrite) {
			return huma.Error403Forbidden("environment variables require secrets:write")
		}
		return nil
	default:
		return huma.Error403Forbidden("environment variables require authenticated actor")
	}
}

// DeleteEnvironmentInput is the typed input for deleting an environment.
type DeleteEnvironmentInput struct {
	EnvID string `path:"envID"`
}

func (s *Server) handleDeleteEnvironment(ctx context.Context, input *DeleteEnvironmentInput) (*struct{}, error) {
	projectID := projectIDFromContext(ctx)
	env, err := s.store.GetEnvironment(ctx, input.EnvID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return nil, huma.Error404NotFound("environment not found")
		}
		return nil, huma.Error500InternalServerError("failed to get environment")
	}

	if err := requireProjectMatch(ctx, env.ProjectID); err != nil {
		return nil, huma.Error404NotFound("environment not found")
	}
	if err := requireEnvironmentMatch(ctx, env.ID); err != nil {
		return nil, huma.Error404NotFound("environment not found")
	}

	if err := s.store.DeleteEnvironment(ctx, input.EnvID, projectID); err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return nil, huma.Error404NotFound("environment not found")
		}
		if errors.Is(err, store.ErrStandardEnvironment) {
			return nil, huma.Error403Forbidden("cannot delete standard environment")
		}
		return nil, huma.Error500InternalServerError("failed to delete environment")
	}

	s.emitAuditEvent(ctx, domain.AuditActionEnvironmentDeleted, "environment", input.EnvID, map[string]any{
		"name": env.Name,
		"slug": env.Slug,
	})

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
	projectID := projectIDFromContext(ctx)
	env, err := s.store.GetEnvironment(ctx, input.EnvID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return nil, huma.Error404NotFound("environment not found")
		}
		return nil, huma.Error500InternalServerError("failed to get environment")
	}

	if err := requireProjectMatch(ctx, env.ProjectID); err != nil {
		return nil, huma.Error404NotFound("environment not found")
	}
	if err := requireEnvironmentMatch(ctx, env.ID); err != nil {
		return nil, huma.Error404NotFound("environment not found")
	}

	resolvedVariables, err := s.store.GetResolvedEnvironmentVariables(ctx, input.EnvID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return nil, huma.Error404NotFound("environment not found")
		}
		return nil, huma.Error500InternalServerError("failed to resolve environment variables")
	}

	return &GetResolvedVariablesOutput{Body: map[string]map[string]string{"variables": resolvedVariables}}, nil
}

func (s *Server) validateEnvironmentParent(ctx context.Context, projectID, envID, parentID string) error {
	if parentID == "" {
		return nil
	}
	if envID != "" && parentID == envID {
		return huma.Error400BadRequest("environment cannot inherit from itself")
	}
	if callerEnvID := environmentIDFromContext(ctx); callerEnvID != "" && parentID != callerEnvID {
		return huma.Error404NotFound("parent environment not found")
	}
	parent, err := s.store.GetEnvironment(ctx, parentID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return huma.Error404NotFound("parent environment not found")
		}
		return huma.Error500InternalServerError("failed to get parent environment")
	}
	if parent.ProjectID != projectID {
		return huma.Error404NotFound("parent environment not found")
	}
	if err := requireEnvironmentMatch(ctx, parent.ID); err != nil {
		return huma.Error404NotFound("parent environment not found")
	}
	return nil
}

func environmentResponse(env *domain.Environment, resolvedVariables map[string]string) EnvironmentResponse {
	if env == nil {
		return EnvironmentResponse{}
	}
	return EnvironmentResponse{
		ID:                   env.ID,
		ProjectID:            env.ProjectID,
		Name:                 env.Name,
		Slug:                 env.Slug,
		ParentID:             env.ParentID,
		IsStandard:           env.IsStandard,
		VariableKeys:         tagKeys(env.Variables),
		ResolvedVariableKeys: tagKeys(resolvedVariables),
		CreatedAt:            env.CreatedAt,
		UpdatedAt:            env.UpdatedAt,
	}
}
