package api

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type WebhookDeliveryStatsInput struct {
	From string `query:"from"`
	To   string `query:"to"`
}
type WebhookDeliveryStatsOutput struct{ Body any }

func (s *Server) handleWebhookDeliveryStats(ctx context.Context, input *WebhookDeliveryStatsInput) (*WebhookDeliveryStatsOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.WebhookDeliveryStats")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)))
	result, rErr := s.analytics().GetWebhookDeliveryStats(ctx, projectID, from, to)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get webhook delivery stats")
	}
	return &WebhookDeliveryStatsOutput{Body: result}, nil
}

type WebhookEndpointHealthInput struct {
	From   string `query:"from"`
	To     string `query:"to"`
	Bucket string `query:"bucket"`
}
type WebhookEndpointHealthOutput struct{ Body any }

func (s *Server) handleWebhookEndpointHealth(ctx context.Context, input *WebhookEndpointHealthInput) (*WebhookEndpointHealthOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.WebhookEndpointHealth")
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
	result, rErr := s.analytics().GetWebhookEndpointHealth(ctx, projectID, from, to, bucket)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get webhook endpoint health")
	}
	return &WebhookEndpointHealthOutput{Body: result}, nil
}

type TopFailingWebhooksInput struct {
	From  string `query:"from"`
	To    string `query:"to"`
	Limit int    `query:"limit"`
}
type TopFailingWebhooksOutput struct{ Body any }

func (s *Server) handleTopFailingWebhooks(ctx context.Context, input *TopFailingWebhooksInput) (*TopFailingWebhooksOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.TopFailingWebhooks")
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
	result, rErr := s.analytics().GetTopFailingWebhooks(ctx, projectID, from, to, limit)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get top failing webhooks")
	}
	return &TopFailingWebhooksOutput{Body: result}, nil
}
