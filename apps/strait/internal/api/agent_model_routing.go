package api

import (
	"context"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
)

// modelRoutingStore is the subset of store methods needed for model routing CRUD.
type modelRoutingStore interface {
	GetModelRouting(ctx context.Context, agentID string) ([]domain.ModelRoute, error)
	UpsertModelRouting(ctx context.Context, route *domain.ModelRoute) error
}

// -- Model Routing types --.

type ModelRouteEntry struct {
	Tier  string `json:"tier" validate:"required"`
	Model string `json:"model" validate:"required"`
}

type UpdateModelRoutingRequest struct {
	Routes []ModelRouteEntry `json:"routes" validate:"required"`
}

type GetModelRoutingInput struct {
	AgentID string `path:"agentID"`
}

type GetModelRoutingOutput struct {
	Body []domain.ModelRoute
}

type UpdateModelRoutingInput struct {
	AgentID string `path:"agentID"`
	Body    UpdateModelRoutingRequest
}

type UpdateModelRoutingOutput struct {
	Body []domain.ModelRoute
}

var validTiers = map[string]bool{
	"simple":   true,
	"standard": true,
	"complex":  true,
}

// -- Handlers --.

func (s *Server) handleGetModelRouting(ctx context.Context, input *GetModelRoutingInput) (*GetModelRoutingOutput, error) {
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

	if _, agentErr := svc.GetAgent(ctx, projectID, input.AgentID); agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	mrStore, ok := s.store.(modelRoutingStore)
	if !ok {
		return nil, huma.Error500InternalServerError("model routing operations not supported")
	}

	routes, err := mrStore.GetModelRouting(ctx, input.AgentID)
	if err != nil {
		slog.Error("failed to get model routing", "agent_id", input.AgentID, "error", err)
		return nil, huma.Error500InternalServerError("failed to get model routing")
	}

	if routes == nil {
		routes = []domain.ModelRoute{}
	}

	return &GetModelRoutingOutput{Body: routes}, nil
}

func (s *Server) handleUpdateModelRouting(ctx context.Context, input *UpdateModelRoutingInput) (*UpdateModelRoutingOutput, error) {
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

	if _, agentErr := svc.GetAgent(ctx, projectID, input.AgentID); agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	// Validate tier values.
	for _, entry := range req.Routes {
		if !validTiers[entry.Tier] {
			return nil, huma.Error400BadRequest("invalid tier: " + entry.Tier + "; must be simple, standard, or complex")
		}
	}

	mrStore, ok := s.store.(modelRoutingStore)
	if !ok {
		return nil, huma.Error500InternalServerError("model routing operations not supported")
	}

	for _, entry := range req.Routes {
		route := &domain.ModelRoute{
			AgentID: input.AgentID,
			Tier:    entry.Tier,
			Model:   entry.Model,
		}
		if err := mrStore.UpsertModelRouting(ctx, route); err != nil {
			slog.Error("failed to upsert model routing", "agent_id", input.AgentID, "tier", entry.Tier, "error", err)
			return nil, huma.Error500InternalServerError("failed to update model routing")
		}
	}

	routes, err := mrStore.GetModelRouting(ctx, input.AgentID)
	if err != nil {
		slog.Error("failed to get model routing after update", "agent_id", input.AgentID, "error", err)
		return nil, huma.Error500InternalServerError("failed to retrieve updated model routing")
	}

	if routes == nil {
		routes = []domain.ModelRoute{}
	}

	return &UpdateModelRoutingOutput{Body: routes}, nil
}
