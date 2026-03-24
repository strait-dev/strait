package api

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type CostTimeRangeInput struct {
	From string `query:"from"`
	To   string `query:"to"`
}

type CostAnalyticsOutput struct {
	Body any
}

func (s *Server) handleGetCostAnalytics(ctx context.Context, input *CostTimeRangeInput) (*CostAnalyticsOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.GetCostAnalytics")
	defer span.End()

	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}

	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)))

	analytics, aErr := s.analytics().GetCostAnalytics(ctx, projectID, from, to)
	if aErr != nil {
		return nil, huma.Error500InternalServerError("failed to get cost analytics")
	}
	return &CostAnalyticsOutput{Body: analytics}, nil
}

type CostTrendsInput struct {
	From string `query:"from"`
	To   string `query:"to"`
}
type CostTrendsOutput struct{ Body any }

func (s *Server) handleGetCostTrends(ctx context.Context, input *CostTrendsInput) (*CostTrendsOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.GetCostTrends")
	defer span.End()

	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)))

	trends, tErr := s.analytics().GetCostTrends(ctx, projectID, from, to)
	if tErr != nil {
		return nil, huma.Error500InternalServerError("failed to get cost trends")
	}
	return &CostTrendsOutput{Body: trends}, nil
}

type TopCostsInput struct {
	From  string `query:"from"`
	To    string `query:"to"`
	Limit int    `query:"limit"`
}
type TopCostsOutput struct{ Body any }

func (s *Server) handleGetTopCosts(ctx context.Context, input *TopCostsInput) (*TopCostsOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.GetTopCosts")
	defer span.End()

	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	limit := input.Limit
	if limit == 0 {
		limit = 10
	}
	if limit < 1 || limit > 100 {
		return nil, huma.Error400BadRequest("limit must be between 1 and 100")
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)), attribute.Int("limit", limit))

	items, iErr := s.analytics().GetTopCosts(ctx, projectID, from, to, limit)
	if iErr != nil {
		return nil, huma.Error500InternalServerError("failed to get top costs")
	}
	return &TopCostsOutput{Body: items}, nil
}

type ComputeCostInput struct {
	From string `query:"from"`
	To   string `query:"to"`
}
type ComputeCostOutput struct{ Body any }

func (s *Server) handleGetComputeCostAnalytics(ctx context.Context, input *ComputeCostInput) (*ComputeCostOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.GetComputeCostAnalytics")
	defer span.End()

	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)))

	analytics, aErr := s.analytics().GetComputeCostAnalytics(ctx, projectID, from, to)
	if aErr != nil {
		return nil, huma.Error500InternalServerError("failed to get compute cost analytics")
	}
	return &ComputeCostOutput{Body: analytics}, nil
}

const maxCostWindow = 90 * 24 * time.Hour
