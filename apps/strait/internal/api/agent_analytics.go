package api

import (
	"context"
	"time"

	"strait/internal/clickhouse"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type AgentRunTimelineInput struct {
	AgentID string `query:"agent_id"`
	From    string `query:"from"`
	To      string `query:"to"`
	Bucket  string `query:"bucket"`
}
type AgentRunTimelineOutput struct {
	Body []clickhouse.AgentRunTimelinePoint
}

func (s *Server) handleAgentRunTimeline(ctx context.Context, input *AgentRunTimelineInput) (*AgentRunTimelineOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.AgentRunTimeline")
	defer span.End()

	a, err := s.requireAnalytics()
	if err != nil {
		return nil, err
	}
	chStore, ok := a.(*clickhouse.AnalyticsStore)
	if !ok {
		return nil, huma.Error503ServiceUnavailable("agent analytics requires ClickHouse")
	}

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

	span.SetAttributes(
		attribute.String("agent_id", input.AgentID),
		attribute.String("from", from.Format(time.RFC3339)),
		attribute.String("to", to.Format(time.RFC3339)),
	)

	result, err := chStore.GetAgentRunTimeline(ctx, projectID, input.AgentID, from, to, bucket)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get agent run timeline")
	}
	if result == nil {
		result = []clickhouse.AgentRunTimelinePoint{}
	}
	return &AgentRunTimelineOutput{Body: result}, nil
}

type AgentCostSummaryInput struct {
	AgentID string `query:"agent_id"`
	From    string `query:"from"`
	To      string `query:"to"`
}
type AgentCostSummaryOutput struct{ Body *clickhouse.AgentCostSummary }

func (s *Server) handleAgentCostSummary(ctx context.Context, input *AgentCostSummaryInput) (*AgentCostSummaryOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.AgentCostSummary")
	defer span.End()

	a, err := s.requireAnalytics()
	if err != nil {
		return nil, err
	}
	chStore, ok := a.(*clickhouse.AnalyticsStore)
	if !ok {
		return nil, huma.Error503ServiceUnavailable("agent analytics requires ClickHouse")
	}

	projectID := projectIDFromContext(ctx)
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}

	span.SetAttributes(attribute.String("agent_id", input.AgentID))

	result, err := chStore.GetAgentCostSummary(ctx, projectID, input.AgentID, from, to)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get agent cost summary")
	}
	return &AgentCostSummaryOutput{Body: result}, nil
}

type AgentModelBreakdownInput struct {
	AgentID string `query:"agent_id"`
	From    string `query:"from"`
	To      string `query:"to"`
}
type AgentModelBreakdownOutput struct {
	Body []clickhouse.AgentModelBreakdownRow
}

func (s *Server) handleAgentModelBreakdown(ctx context.Context, input *AgentModelBreakdownInput) (*AgentModelBreakdownOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.AgentModelBreakdown")
	defer span.End()

	a, err := s.requireAnalytics()
	if err != nil {
		return nil, err
	}
	chStore, ok := a.(*clickhouse.AnalyticsStore)
	if !ok {
		return nil, huma.Error503ServiceUnavailable("agent analytics requires ClickHouse")
	}

	projectID := projectIDFromContext(ctx)
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}

	span.SetAttributes(attribute.String("agent_id", input.AgentID))

	result, err := chStore.GetAgentModelBreakdown(ctx, projectID, input.AgentID, from, to)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get agent model breakdown")
	}
	if result == nil {
		result = []clickhouse.AgentModelBreakdownRow{}
	}
	return &AgentModelBreakdownOutput{Body: result}, nil
}

type AgentTopAgentsInput struct {
	From  string `query:"from"`
	To    string `query:"to"`
	Limit int    `query:"limit"`
}
type AgentTopAgentsOutput struct{ Body []clickhouse.AgentRankingRow }

func (s *Server) handleAgentTopAgents(ctx context.Context, input *AgentTopAgentsInput) (*AgentTopAgentsOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.AgentTopAgents")
	defer span.End()

	a, err := s.requireAnalytics()
	if err != nil {
		return nil, err
	}
	chStore, ok := a.(*clickhouse.AnalyticsStore)
	if !ok {
		return nil, huma.Error503ServiceUnavailable("agent analytics requires ClickHouse")
	}

	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	result, err := chStore.GetAgentTopAgents(ctx, projectID, from, to, limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get top agents")
	}
	if result == nil {
		result = []clickhouse.AgentRankingRow{}
	}
	return &AgentTopAgentsOutput{Body: result}, nil
}
