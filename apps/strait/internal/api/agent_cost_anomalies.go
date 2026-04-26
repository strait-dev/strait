package api

import (
	"context"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// CostAnomalyStore defines store methods used by cost anomaly API handlers.
type CostAnomalyStore interface {
	ListCostAnomalies(ctx context.Context, agentID string, limit int) ([]domain.CostAnomaly, error)
	SnoozeAnomaly(ctx context.Context, id string, until time.Time) error
}

// -- List anomalies --

type ListCostAnomaliesInput struct {
	AgentID string `path:"agentID" doc:"Agent ID"`
	Limit   int    `query:"limit" default:"50" doc:"Max anomalies to return"`
}

type ListCostAnomaliesOutput struct {
	Body []domain.CostAnomaly
}

func (s *Server) handleListCostAnomalies(ctx context.Context, input *ListCostAnomaliesInput) (*ListCostAnomaliesOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.ListCostAnomalies")
	defer span.End()

	svc, svcErr := s.requireAgentService()
	if svcErr != nil {
		return nil, svcErr
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required")
	}
	if _, agentErr := svc.GetAgent(ctx, projectID, input.AgentID); agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	span.SetAttributes(attribute.String("agent_id", input.AgentID))

	anomalies, err := s.store.ListCostAnomalies(ctx, input.AgentID, input.Limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list cost anomalies")
	}
	if anomalies == nil {
		anomalies = []domain.CostAnomaly{}
	}
	return &ListCostAnomaliesOutput{Body: anomalies}, nil
}

// -- Snooze anomaly --

type SnoozeCostAnomalyInput struct {
	AgentID   string `path:"agentID" doc:"Agent ID"`
	AnomalyID string `path:"anomalyID" doc:"Anomaly ID"`
	Body      struct {
		Hours int `json:"hours" default:"24" doc:"Hours to snooze"`
	}
}

type SnoozeCostAnomalyOutput struct {
	Body struct {
		Status       string    `json:"status"`
		SnoozedUntil time.Time `json:"snoozed_until"`
	}
}

func (s *Server) handleSnoozeCostAnomaly(ctx context.Context, input *SnoozeCostAnomalyInput) (*SnoozeCostAnomalyOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.SnoozeCostAnomaly")
	defer span.End()

	svc, svcErr := s.requireAgentService()
	if svcErr != nil {
		return nil, svcErr
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required")
	}
	if _, agentErr := svc.GetAgent(ctx, projectID, input.AgentID); agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	span.SetAttributes(
		attribute.String("agent_id", input.AgentID),
		attribute.String("anomaly_id", input.AnomalyID),
	)

	hours := input.Body.Hours
	if hours <= 0 {
		hours = 24
	}
	until := time.Now().UTC().Add(time.Duration(hours) * time.Hour)

	if err := s.store.SnoozeAnomaly(ctx, input.AnomalyID, until); err != nil {
		return nil, huma.Error500InternalServerError("failed to snooze anomaly")
	}

	out := &SnoozeCostAnomalyOutput{}
	out.Body.Status = "snoozed"
	out.Body.SnoozedUntil = until
	return out, nil
}
