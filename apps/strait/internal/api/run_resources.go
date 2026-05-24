package api

import (
	"context"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

type ListRunResourcesInput struct {
	RunID string `path:"runID"`
	From  string `query:"from"`
	To    string `query:"to"`
	Limit string `query:"limit"`
}

type ListRunResourcesOutput struct {
	Body any
}

func (s *Server) handleListRunResources(ctx context.Context, input *ListRunResourcesInput) (*ListRunResourcesOutput, error) {
	if err := s.requireRunAccess(ctx, input.RunID); err != nil {
		return nil, err
	}

	var from, to *time.Time
	if input.From != "" {
		t, parseErr := time.Parse(time.RFC3339, input.From)
		if parseErr != nil {
			return nil, huma.Error400BadRequest("invalid from param: must be RFC3339")
		}
		from = &t
	}
	if input.To != "" {
		t, parseErr := time.Parse(time.RFC3339, input.To)
		if parseErr != nil {
			return nil, huma.Error400BadRequest("invalid to param: must be RFC3339")
		}
		to = &t
	}

	limit := 100
	if input.Limit != "" {
		n, parseErr := strconv.Atoi(input.Limit)
		if parseErr != nil || n < 1 {
			return nil, huma.Error400BadRequest("invalid limit param")
		}
		if n > 1000 {
			n = 1000
		}
		limit = n
	}

	snapshots, err := s.store.ListRunResourceSnapshots(ctx, input.RunID, from, to, limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list resource snapshots")
	}

	return &ListRunResourcesOutput{Body: snapshots}, nil
}
