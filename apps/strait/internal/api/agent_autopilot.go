package api

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/agents"
	"strait/internal/domain"
)

// autopilotActionStore is the subset of store methods needed for autopilot CRUD.
type autopilotActionStore interface {
	CreateAutopilotAction(ctx context.Context, action *domain.AutopilotAction) error
	ListAutopilotActions(ctx context.Context, agentID string, limit int) ([]domain.AutopilotAction, error)
	GetLatestAutopilotAction(ctx context.Context, agentID string) (*domain.AutopilotAction, error)
}

// -- Autopilot types --.

type GetAutopilotConfigInput struct {
	AgentID string `path:"agentID"`
}

type GetAutopilotConfigOutput struct {
	Body *domain.AutopilotConfig
}

type UpdateAutopilotConfigInput struct {
	AgentID string `path:"agentID"`
	Body    domain.AutopilotConfig
}

type UpdateAutopilotConfigOutput struct {
	Body *domain.AutopilotConfig
}

type ListAutopilotHistoryInput struct {
	AgentID string `path:"agentID"`
	Limit   int    `query:"limit" minimum:"1" maximum:"100" default:"50"`
}

type ListAutopilotHistoryOutput struct {
	Body []domain.AutopilotAction
}

// -- Handlers --.

func (s *Server) handleGetAutopilotConfig(ctx context.Context, input *GetAutopilotConfigInput) (*GetAutopilotConfigOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	agent, agentErr := svc.GetAgent(ctx, projectID, input.AgentID)
	if agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	var cfg domain.AutopilotConfig
	if len(agent.Config) > 0 {
		var configMap map[string]json.RawMessage
		if err := json.Unmarshal(agent.Config, &configMap); err == nil {
			if raw, ok := configMap["autopilot"]; ok {
				_ = json.Unmarshal(raw, &cfg)
			}
		}
	}

	return &GetAutopilotConfigOutput{Body: &cfg}, nil
}

func (s *Server) handleUpdateAutopilotConfig(ctx context.Context, input *UpdateAutopilotConfigInput) (*UpdateAutopilotConfigOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	agent, agentErr := svc.GetAgent(ctx, projectID, input.AgentID)
	if agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	// Merge autopilot config into existing agent.Config.
	var configMap map[string]json.RawMessage
	if len(agent.Config) > 0 {
		if err := json.Unmarshal(agent.Config, &configMap); err != nil {
			configMap = make(map[string]json.RawMessage)
		}
	} else {
		configMap = make(map[string]json.RawMessage)
	}

	autopilotJSON, err := json.Marshal(input.Body)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to marshal autopilot config")
	}
	configMap["autopilot"] = autopilotJSON

	fullConfig, err := json.Marshal(configMap)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to marshal config")
	}

	updated := agents.UpdateAgentRequest{
		ProjectID:   projectID,
		AgentID:     input.AgentID,
		Name:        agent.Name,
		Slug:        agent.Slug,
		Description: agent.Description,
		Model:       agent.Model,
		Config:      json.RawMessage(fullConfig),
		Actor:       actorFromContext(ctx),
	}
	if _, updateErr := svc.UpdateAgent(ctx, updated); updateErr != nil {
		slog.Error("failed to update agent autopilot config", "agent_id", input.AgentID, "error", updateErr)
		return nil, mapAgentServiceError(updateErr)
	}

	return &UpdateAutopilotConfigOutput{Body: &input.Body}, nil
}

func (s *Server) handleListAutopilotHistory(ctx context.Context, input *ListAutopilotHistoryInput) (*ListAutopilotHistoryOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	// Verify agent exists and belongs to project.
	if _, agentErr := svc.GetAgent(ctx, projectID, input.AgentID); agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	apStore, ok := s.store.(autopilotActionStore)
	if !ok {
		return nil, huma.Error500InternalServerError("autopilot operations not supported")
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	actions, err := apStore.ListAutopilotActions(ctx, input.AgentID, limit)
	if err != nil {
		slog.Error("failed to list autopilot actions", "agent_id", input.AgentID, "error", err)
		return nil, huma.Error500InternalServerError("failed to list autopilot actions")
	}

	if actions == nil {
		actions = []domain.AutopilotAction{}
	}

	return &ListAutopilotHistoryOutput{Body: actions}, nil
}
