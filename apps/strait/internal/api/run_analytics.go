package api

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type RunTimelineInput struct {
	From   string `query:"from"`
	To     string `query:"to"`
	Bucket string `query:"bucket"`
}
type RunTimelineOutput struct{ Body any }

func (s *Server) handleRunTimeline(ctx context.Context, input *RunTimelineInput) (*RunTimelineOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.RunTimeline")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	bucket, err := normalizeAnalyticsBucket(input.Bucket)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)), attribute.String("bucket", bucket))
	result, rErr := s.analytics().GetRunTimeline(ctx, projectID, from, to, bucket)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get run timeline")
	}
	return &RunTimelineOutput{Body: result}, nil
}

type RunDurationDistributionInput struct {
	From string `query:"from"`
	To   string `query:"to"`
}
type RunDurationDistributionOutput struct{ Body any }

func (s *Server) handleRunDurationDistribution(ctx context.Context, input *RunDurationDistributionInput) (*RunDurationDistributionOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.RunDurationDistribution")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)))
	result, rErr := s.analytics().GetRunDurationDistribution(ctx, projectID, from, to)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get duration distribution")
	}
	return &RunDurationDistributionOutput{Body: result}, nil
}

type RunFailureReasonsInput struct {
	From  string `query:"from"`
	To    string `query:"to"`
	Limit int    `query:"limit"`
}
type RunFailureReasonsOutput struct{ Body any }

func (s *Server) handleRunFailureReasons(ctx context.Context, input *RunFailureReasonsInput) (*RunFailureReasonsOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.RunFailureReasons")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	limit := input.Limit
	if limit == 0 {
		r := requestFromContext(ctx)
		if r != nil && r.URL.Query().Has("limit") {
			return nil, huma.Error400BadRequest("limit must be between 1 and 100")
		}
		limit = 10
	}
	if limit < 1 || limit > 100 {
		return nil, huma.Error400BadRequest("limit must be between 1 and 100")
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)), attribute.Int("limit", limit))
	result, rErr := s.analytics().GetRunFailureReasons(ctx, projectID, from, to, limit)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get failure reasons")
	}
	return &RunFailureReasonsOutput{Body: result}, nil
}

type RunSummaryInput struct {
	From string `query:"from"`
	To   string `query:"to"`
}
type RunSummaryOutput struct{ Body any }

func (s *Server) handleRunSummary(ctx context.Context, input *RunSummaryInput) (*RunSummaryOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.RunSummary")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)))
	result, rErr := s.analytics().GetRunSummary(ctx, projectID, from, to)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get run summary")
	}
	return &RunSummaryOutput{Body: result}, nil
}

type RunsByTriggerInput struct {
	From string `query:"from"`
	To   string `query:"to"`
}
type RunsByTriggerOutput struct{ Body any }

func (s *Server) handleRunsByTrigger(ctx context.Context, input *RunsByTriggerInput) (*RunsByTriggerOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.RunsByTrigger")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)))
	result, rErr := s.analytics().GetRunsByTrigger(ctx, projectID, from, to)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get runs by trigger")
	}
	return &RunsByTriggerOutput{Body: result}, nil
}
