package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"strait/internal/agents"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

type CreateAgentRequest struct {
	ProjectID   string          `json:"project_id" validate:"required"`
	Name        string          `json:"name" validate:"required"`
	Slug        string          `json:"slug" validate:"required"`
	Description string          `json:"description,omitempty"`
	Model       string          `json:"model" validate:"required"`
	Config      json.RawMessage `json:"config,omitempty"`
}

type UpdateAgentRequest struct {
	Name        *string          `json:"name,omitempty"`
	Slug        *string          `json:"slug,omitempty"`
	Description *string          `json:"description,omitempty"`
	Model       *string          `json:"model,omitempty"`
	Config      *json.RawMessage `json:"config,omitempty"`
}

type RunAgentRequest struct {
	Payload json.RawMessage `json:"payload,omitempty"`
}

type CreateAgentInput struct {
	Body CreateAgentRequest
}

type CreateAgentOutput struct {
	Body *domain.Agent
}

type ListAgentsInput struct {
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

type ListAgentsOutput struct {
	Body []domain.Agent
}

type GetAgentInput struct {
	AgentID string `path:"agentID"`
}

type GetAgentOutput struct {
	Body *domain.Agent
}

type UpdateAgentInput struct {
	AgentID string `path:"agentID"`
	Body    UpdateAgentRequest
}

type UpdateAgentOutput struct {
	Body *domain.Agent
}

type DeleteAgentInput struct {
	AgentID string `path:"agentID"`
}

type DeployAgentInput struct {
	AgentID string `path:"agentID"`
}

type DeployAgentOutput struct {
	Body *domain.AgentDeployment
}

type RunAgentInput struct {
	AgentID string `path:"agentID"`
	Body    RunAgentRequest
}

type RunAgentOutput struct {
	Body *domain.JobRun
}

type ListAgentRunsInput struct {
	AgentID string `path:"agentID"`
	Limit   string `query:"limit"`
	Offset  string `query:"offset"`
}

type ListAgentRunsOutput struct {
	Body []domain.JobRun
}

func (s *Server) requireAgentService() (agents.Service, error) {
	if s.agentService == nil {
		return nil, huma.Error503ServiceUnavailable("agent service unavailable")
	}
	return s.agentService, nil
}

func (s *Server) handleCreateAgent(ctx context.Context, input *CreateAgentInput) (*CreateAgentOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		projectID = req.ProjectID
	}
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if req.ProjectID != "" && projectID != req.ProjectID {
		return nil, huma.Error403Forbidden("access denied")
	}
	if err := validateAgentConfigJSON(req.Config); err != nil {
		return nil, err
	}

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID:   projectID,
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		Model:       req.Model,
		Config:      req.Config,
		Actor:       actorFromContext(ctx),
	})
	if err != nil {
		return nil, mapAgentServiceError(err)
	}

	return &CreateAgentOutput{Body: agent}, nil
}

func (s *Server) handleListAgents(ctx context.Context, input *ListAgentsInput) (*ListAgentsOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	limit := defaultPageLimit
	if input.Limit != "" {
		parsed, err := strconv.Atoi(input.Limit)
		if err != nil || parsed <= 0 || parsed > maxPageLimit {
			return nil, huma.Error400BadRequest("invalid limit")
		}
		limit = parsed
	}

	var cursor *time.Time
	if input.Cursor != "" {
		parsed, err := time.Parse(time.RFC3339Nano, input.Cursor)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid cursor")
		}
		cursor = &parsed
	}

	items, err := svc.ListAgents(ctx, projectID, limit, cursor)
	if err != nil {
		return nil, mapAgentServiceError(err)
	}
	return &ListAgentsOutput{Body: items}, nil
}

func (s *Server) handleGetAgent(ctx context.Context, input *GetAgentInput) (*GetAgentOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	agent, err := svc.GetAgent(ctx, projectID, input.AgentID)
	if err != nil {
		return nil, mapAgentServiceError(err)
	}
	return &GetAgentOutput{Body: agent}, nil
}

func (s *Server) handleUpdateAgent(ctx context.Context, input *UpdateAgentInput) (*UpdateAgentOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	existing, err := svc.GetAgent(ctx, projectID, input.AgentID)
	if err != nil {
		return nil, mapAgentServiceError(err)
	}

	req := input.Body
	updated := agents.UpdateAgentRequest{
		ProjectID:   projectID,
		AgentID:     input.AgentID,
		Name:        existing.Name,
		Slug:        existing.Slug,
		Description: existing.Description,
		Model:       existing.Model,
		Config:      existing.Config,
		Actor:       actorFromContext(ctx),
	}
	if req.Name != nil {
		updated.Name = *req.Name
	}
	if req.Slug != nil {
		updated.Slug = *req.Slug
	}
	if req.Description != nil {
		updated.Description = *req.Description
	}
	if req.Model != nil {
		updated.Model = *req.Model
	}
	if req.Config != nil {
		if err := validateAgentConfigJSON(*req.Config); err != nil {
			return nil, err
		}
		updated.Config = *req.Config
	}

	agent, err := svc.UpdateAgent(ctx, updated)
	if err != nil {
		return nil, mapAgentServiceError(err)
	}
	return &UpdateAgentOutput{Body: agent}, nil
}

func (s *Server) handleDeleteAgent(ctx context.Context, input *DeleteAgentInput) (*struct{}, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	if err := svc.DeleteAgent(ctx, projectID, input.AgentID); err != nil {
		return nil, mapAgentServiceError(err)
	}
	return nil, nil
}

func (s *Server) handleDeployAgent(ctx context.Context, input *DeployAgentInput) (*DeployAgentOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	deployment, err := svc.DeployAgent(ctx, projectID, input.AgentID, actorFromContext(ctx))
	if err != nil {
		return nil, mapAgentServiceError(err)
	}
	return &DeployAgentOutput{Body: deployment}, nil
}

func (s *Server) handleRunAgent(ctx context.Context, input *RunAgentInput) (*RunAgentOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	run, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   input.AgentID,
		Payload:   input.Body.Payload,
		Actor:     actorFromContext(ctx),
	})
	if err != nil {
		return nil, mapAgentServiceError(err)
	}
	return &RunAgentOutput{Body: run}, nil
}

func (s *Server) handleListAgentRuns(ctx context.Context, input *ListAgentRunsInput) (*ListAgentRunsOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	limit := defaultPageLimit
	if input.Limit != "" {
		parsed, err := strconv.Atoi(input.Limit)
		if err != nil || parsed <= 0 || parsed > maxPageLimit {
			return nil, huma.Error400BadRequest("invalid limit")
		}
		limit = parsed
	}

	offset := 0
	if input.Offset != "" {
		parsed, err := strconv.Atoi(input.Offset)
		if err != nil || parsed < 0 {
			return nil, huma.Error400BadRequest("invalid offset")
		}
		offset = parsed
	}

	runs, err := svc.ListAgentRuns(ctx, projectID, input.AgentID, limit, offset)
	if err != nil {
		return nil, mapAgentServiceError(err)
	}
	return &ListAgentRunsOutput{Body: runs}, nil
}

func mapAgentServiceError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrAgentNotFound):
		return huma.Error404NotFound("agent not found")
	case errors.Is(err, store.ErrAgentSlugConflict):
		return huma.Error409Conflict("agent slug already exists")
	case errors.Is(err, agents.ErrNotDeployed):
		return huma.Error409Conflict("agent is not deployed")
	case errors.Is(err, store.ErrAgentDeploymentNotFound):
		return huma.Error404NotFound("agent deployment not found")
	}

	var fieldErr *domain.FieldError
	if errors.As(err, &fieldErr) {
		return huma.Error400BadRequest(fieldErr.Error())
	}
	var validationErr *agents.ValidationError
	if errors.As(err, &validationErr) {
		return huma.Error400BadRequest(validationErr.Error())
	}

	slog.Error("agent handler failed", "error", err)
	return huma.Error500InternalServerError("agent request failed")
}

func validateAgentConfigJSON(config json.RawMessage) error {
	if len(config) == 0 {
		return nil
	}
	if !json.Valid(config) {
		return huma.Error400BadRequest("config must be valid JSON")
	}

	var decoded any
	if err := json.Unmarshal(config, &decoded); err != nil {
		return huma.Error400BadRequest("config must be valid JSON")
	}
	if _, ok := decoded.(map[string]any); !ok {
		return huma.Error400BadRequest("config must be a JSON object")
	}
	return nil
}
