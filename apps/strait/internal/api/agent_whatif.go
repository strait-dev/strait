package api

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/agents"
	"strait/internal/domain"
)

// --- What-If Estimate (GET) ---

type WhatIfEstimateInput struct {
	RunID string `path:"runID"`
	Model string `query:"model" required:"true" doc:"Target model to estimate cost for"`
}

type WhatIfEstimateOutput struct {
	Body *domain.WhatIfEstimate
}

func (s *Server) handleWhatIfEstimate(ctx context.Context, input *WhatIfEstimateInput) (*WhatIfEstimateOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required")
	}

	engine := agents.NewWhatIfEngine(s.store, s.store, svc)
	estimate, err := engine.EstimateCost(ctx, input.RunID, input.Model)
	if err != nil {
		return nil, huma.Error400BadRequest(fmt.Sprintf("estimate failed: %s", err))
	}

	return &WhatIfEstimateOutput{Body: estimate}, nil
}

// --- What-If Replay (POST) ---

type WhatIfReplayRequestBody struct {
	TargetModel string `json:"target_model" required:"true" doc:"Model to replay the run with"`
	AgentID     string `json:"agent_id" required:"true" doc:"Agent ID for the replay"`
}

type WhatIfReplayInput struct {
	RunID string `path:"runID"`
	Body  WhatIfReplayRequestBody
}

type WhatIfReplayOutput struct {
	Body *domain.WhatIfReplayResult
}

func (s *Server) handleWhatIfReplay(ctx context.Context, input *WhatIfReplayInput) (*WhatIfReplayOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required")
	}

	engine := agents.NewWhatIfEngine(s.store, s.store, svc)
	result, err := engine.Replay(ctx, input.RunID, input.Body.TargetModel, projectID, input.Body.AgentID, actorFromContext(ctx))
	if err != nil {
		return nil, huma.Error400BadRequest(fmt.Sprintf("what-if replay failed: %s", err))
	}

	return &WhatIfReplayOutput{Body: result}, nil
}
