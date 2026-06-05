package api

import (
	"context"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

type ListRunCheckpointsInput struct {
	RunID  string `path:"runID"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

type ListRunCheckpointsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListRunCheckpoints(ctx context.Context, input *ListRunCheckpointsInput) (*ListRunCheckpointsOutput, error) {
	if err := s.requireRunAccess(ctx, input.RunID); err != nil {
		return nil, err
	}

	limit, cursor, err := parsePaginationParamsTyped(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	checkpoints, err := s.store.ListRunCheckpoints(ctx, input.RunID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list run checkpoints")
	}

	return &ListRunCheckpointsOutput{
		Body: paginatedResult(checkpoints, limit, func(cp domain.RunCheckpoint) string {
			return cp.CreatedAt.Format(time.RFC3339Nano)
		}),
	}, nil
}

type ListRunOutputsInput struct {
	RunID  string `path:"runID"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

type ListRunOutputsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListRunOutputs(ctx context.Context, input *ListRunOutputsInput) (*ListRunOutputsOutput, error) {
	if err := s.requireRunAccess(ctx, input.RunID); err != nil {
		return nil, err
	}

	limit, cursor, err := parsePaginationParamsTyped(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	outputs, err := s.store.ListRunOutputs(ctx, input.RunID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list run outputs")
	}

	return &ListRunOutputsOutput{
		Body: paginatedResult(outputs, limit, func(o domain.RunOutput) string {
			return o.CreatedAt.Format(time.RFC3339Nano)
		}),
	}, nil
}
