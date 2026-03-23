package api

import (
	"context"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

type ListRunEventsInput struct {
	RunID  string `path:"runID"`
	Level  string `query:"level"`
	Type   string `query:"type"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

type ListRunEventsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListRunEvents(ctx context.Context, input *ListRunEventsInput) (*ListRunEventsOutput, error) {
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	events, err := s.store.ListEventsByRunFiltered(ctx, input.RunID, input.Level, input.Type, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list events")
	}
	return &ListRunEventsOutput{Body: paginatedResult(events, limit, func(e domain.RunEvent) string {
		return e.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}
