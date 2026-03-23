package api

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type EventVolumeInput struct {
	From   string `query:"from"`
	To     string `query:"to"`
	Bucket string `query:"bucket"`
}
type EventVolumeOutput struct{ Body any }

func (s *Server) handleEventVolume(ctx context.Context, input *EventVolumeInput) (*EventVolumeOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.EventVolume")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	bucket := input.Bucket
	if bucket == "" {
		bucket = "day"
	}
	if bucket != "hour" && bucket != "day" {
		return nil, huma.Error400BadRequest("bucket must be 'hour' or 'day'")
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)), attribute.String("bucket", bucket))
	result, rErr := s.analytics().GetEventVolume(ctx, projectID, from, to, bucket)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get event volume")
	}
	return &EventVolumeOutput{Body: result}, nil
}

type EventLatencyInput struct {
	From string `query:"from"`
	To   string `query:"to"`
}
type EventLatencyOutput struct{ Body any }

func (s *Server) handleEventLatency(ctx context.Context, input *EventLatencyInput) (*EventLatencyOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.EventLatency")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)))
	result, rErr := s.analytics().GetEventLatency(ctx, projectID, from, to)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get event latency")
	}
	return &EventLatencyOutput{Body: result}, nil
}

type CostForecastInput struct {
	From string `query:"from"`
	To   string `query:"to"`
}
type CostForecastOutput struct{ Body any }

func (s *Server) handleCostForecast(ctx context.Context, input *CostForecastInput) (*CostForecastOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.CostForecast")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)))
	result, rErr := s.analytics().GetCostForecast(ctx, projectID, from, to)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get cost forecast")
	}
	return &CostForecastOutput{Body: result}, nil
}

type CostByTriggerInput struct {
	From string `query:"from"`
	To   string `query:"to"`
}
type CostByTriggerOutput struct{ Body any }

func (s *Server) handleCostByTrigger(ctx context.Context, input *CostByTriggerInput) (*CostByTriggerOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.CostByTrigger")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)))
	result, rErr := s.analytics().GetCostByTrigger(ctx, projectID, from, to)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get cost by trigger")
	}
	return &CostByTriggerOutput{Body: result}, nil
}

type CostByMachineInput struct {
	From string `query:"from"`
	To   string `query:"to"`
}
type CostByMachineOutput struct{ Body any }

func (s *Server) handleCostByMachine(ctx context.Context, input *CostByMachineInput) (*CostByMachineOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.CostByMachine")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)))
	result, rErr := s.analytics().GetCostByMachine(ctx, projectID, from, to)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get cost by machine")
	}
	return &CostByMachineOutput{Body: result}, nil
}
