package api

import (
	"context"
	"encoding/json"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

type SDKSetStateRequest struct {
	Key   string          `json:"key" validate:"required"`
	Value json.RawMessage `json:"value" validate:"required"`
}
type SDKSetStateInput struct {
	RunID string `path:"runID"`
	Body  SDKSetStateRequest
}
type SDKSetStateOutput struct{ Body *domain.RunState }

func (s *Server) handleSDKSetState(ctx context.Context, input *SDKSetStateInput) (*SDKSetStateOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if len(req.Key) > 256 {
		return nil, huma.Error400BadRequest("state key must be 256 characters or fewer")
	}
	if len(req.Value) > 65536 {
		return nil, huma.Error400BadRequest("state value must not exceed 64KB")
	}
	state := &domain.RunState{RunID: input.RunID, StateKey: req.Key, Value: req.Value}
	if err := s.store.UpsertRunState(ctx, state); err != nil {
		return nil, huma.Error500InternalServerError("failed to upsert run state")
	}
	return &SDKSetStateOutput{Body: state}, nil
}

type SDKGetStateInput struct {
	RunID string `path:"runID"`
	Key   string `path:"key"`
}
type SDKGetStateOutput struct{ Body *domain.RunState }

func (s *Server) handleSDKGetState(ctx context.Context, input *SDKGetStateInput) (*SDKGetStateOutput, error) {
	state, err := s.store.GetRunState(ctx, input.RunID, input.Key)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get run state")
	}
	if state == nil {
		return nil, huma.Error404NotFound("state key not found")
	}
	return &SDKGetStateOutput{Body: state}, nil
}

type SDKListStateOutput struct{ Body any }

func (s *Server) handleSDKListState(ctx context.Context, input *SDKRunIDInput) (*SDKListStateOutput, error) {
	items, err := s.store.ListRunState(ctx, input.RunID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list run state")
	}
	return &SDKListStateOutput{Body: items}, nil
}

type SDKDeleteStateInput struct {
	RunID string `path:"runID"`
	Key   string `path:"key"`
}

func (s *Server) handleSDKDeleteState(ctx context.Context, input *SDKDeleteStateInput) (*struct{}, error) {
	if err := s.store.DeleteRunState(ctx, input.RunID, input.Key); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete run state")
	}
	return nil, nil
}

type ListRunStateInput struct {
	RunID string `path:"runID"`
}
type ListRunStateOutput struct{ Body any }

func (s *Server) handleListRunState(ctx context.Context, input *ListRunStateInput) (*ListRunStateOutput, error) {
	items, err := s.store.ListRunState(ctx, input.RunID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list run state")
	}
	return &ListRunStateOutput{Body: items}, nil
}
