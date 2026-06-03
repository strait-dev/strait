package api

import (
	"context"
	"strconv"
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

// parsePaginationParamsTyped is a typed-handler variant of parsePaginationParams
// that accepts raw string values from query params instead of *http.Request.
func parsePaginationParamsTyped(limitStr, cursorStr string) (int, *time.Time, error) {
	limit := defaultPageLimit
	if limitStr != "" {
		parsed, parseErr := strconv.Atoi(limitStr)
		if parseErr != nil || parsed <= 0 {
			return 0, nil, &paginationError{msg: "limit must be a positive integer"}
		}
		if parsed > maxPageLimit {
			parsed = maxPageLimit
		}
		limit = parsed
	}

	var cursor *time.Time
	if cursorStr != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, cursorStr)
		if parseErr != nil {
			return 0, nil, &paginationError{msg: "cursor must be a valid RFC3339 timestamp"}
		}
		cursor = &parsed
	}

	return limit, cursor, nil
}
