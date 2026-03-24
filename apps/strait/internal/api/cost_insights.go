package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

type CostInsightsInput struct {
	From      string  `query:"from"`
	To        string  `query:"to"`
	Threshold float64 `query:"threshold"`
}

type CostInsightsOutput struct {
	Body any
}

func (s *Server) handleGetCostInsights(ctx context.Context, input *CostInsightsInput) (*CostInsightsOutput, error) {
	projectID := projectIDFromContext(ctx)

	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}

	threshold := input.Threshold
	if threshold == 0 {
		threshold = 2.0
	}
	if threshold <= 0 {
		return nil, huma.Error400BadRequest("threshold must be a positive number")
	}

	outliers, oErr := s.analytics().GetCostOutliers(ctx, projectID, from, to, threshold)
	if oErr != nil {
		return nil, huma.Error500InternalServerError("failed to get cost insights")
	}

	return &CostInsightsOutput{Body: map[string]any{
		"outliers":  outliers,
		"threshold": threshold,
	}}, nil
}
