package api

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/billing"
)

const maxApprovalStatsRange = 90 * 24 * time.Hour

type ApprovalStatsInput struct {
	From string `query:"from"`
	To   string `query:"to"`
}

type ApprovalStatsOutput struct {
	Body any
}

func (s *Server) handleGetApprovalStats(ctx context.Context, input *ApprovalStatsInput) (*ApprovalStatsOutput, error) {
	projectID := projectIDFromContext(ctx)

	if err := s.checkFeatureAllowed(ctx, projectID, billing.FeatureApprovalGates, "Approval gates"); err != nil {
		return nil, err
	}

	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}

	if to.Before(from) {
		return nil, huma.Error400BadRequest("from must be before to")
	}
	if to.Sub(from) > maxApprovalStatsRange {
		return nil, huma.Error400BadRequest("time range must not exceed 90 days")
	}

	stats, sErr := s.analytics().GetApprovalStats(ctx, projectID, from, to)
	if sErr != nil {
		return nil, huma.Error500InternalServerError("failed to get approval stats")
	}

	return &ApprovalStatsOutput{Body: stats}, nil
}
