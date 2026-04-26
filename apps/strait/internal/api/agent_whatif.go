package api

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/agents"
	"strait/internal/domain"
	"strait/internal/store"
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

	run, runErr := s.store.GetRun(ctx, input.RunID)
	if runErr != nil {
		if errors.Is(runErr, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("run not found")
	}

	engine := agents.NewWhatIfEngine(s.store, s.store, svc)
	estimate, estErr := engine.EstimateCost(ctx, input.RunID, input.Model)
	if estErr != nil {
		return nil, huma.Error400BadRequest("estimate failed")
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

	run, runErr := s.store.GetRun(ctx, input.RunID)
	if runErr != nil {
		if errors.Is(runErr, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("run not found")
	}

	engine := agents.NewWhatIfEngine(s.store, s.store, svc)
	result, replayErr := engine.Replay(ctx, input.RunID, input.Body.TargetModel, projectID, input.Body.AgentID, actorFromContext(ctx))
	if replayErr != nil {
		return nil, huma.Error400BadRequest("what-if replay failed")
	}

	return &WhatIfReplayOutput{Body: result}, nil
}
