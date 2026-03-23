package api

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type TagSummaryInput struct {
	From  string `query:"from"`
	To    string `query:"to"`
	Limit int    `query:"limit"`
}
type TagSummaryOutput struct{ Body any }

func (s *Server) handleTagSummary(ctx context.Context, input *TagSummaryInput) (*TagSummaryOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.TagSummary")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	limit := input.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 500 {
		return nil, huma.Error400BadRequest("limit must be between 1 and 500")
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)), attribute.Int("limit", limit))
	result, rErr := s.analytics().GetTagSummary(ctx, projectID, from, to, limit)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get tag summary")
	}
	return &TagSummaryOutput{Body: result}, nil
}

type TopFailingTagsInput struct {
	From  string `query:"from"`
	To    string `query:"to"`
	Limit int    `query:"limit"`
}
type TopFailingTagsOutput struct{ Body any }

func (s *Server) handleTopFailingTags(ctx context.Context, input *TopFailingTagsInput) (*TopFailingTagsOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.TopFailingTags")
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
	result, rErr := s.analytics().GetTopFailingTags(ctx, projectID, from, to, limit)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get top failing tags")
	}
	return &TopFailingTagsOutput{Body: result}, nil
}

type TagCostInput struct {
	From  string `query:"from"`
	To    string `query:"to"`
	Limit int    `query:"limit"`
}
type TagCostOutput struct{ Body any }

func (s *Server) handleTagCost(ctx context.Context, input *TagCostInput) (*TagCostOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.TagCost")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	limit := input.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 500 {
		return nil, huma.Error400BadRequest("limit must be between 1 and 500")
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)), attribute.Int("limit", limit))
	result, rErr := s.analytics().GetTagCost(ctx, projectID, from, to, limit)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get tag cost")
	}
	return &TagCostOutput{Body: result}, nil
}
