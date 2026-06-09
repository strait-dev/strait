package api

import (
	"context"
	"errors"
	"time"

	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type WorkflowStepDurationsInput struct {
	WorkflowID string `path:"workflowID"`
	From       string `query:"from"`
	To         string `query:"to"`
}
type WorkflowStepDurationsOutput struct{ Body any }

func (s *Server) handleWorkflowStepDurations(ctx context.Context, input *WorkflowStepDurationsInput) (*WorkflowStepDurationsOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.WorkflowStepDurations")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	wf, err := s.store.GetWorkflow(ctx, input.WorkflowID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			return nil, huma.Error404NotFound("workflow not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow")
	}
	if err := requireProjectMatch(ctx, wf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}
	span.SetAttributes(attribute.String("workflow_id", input.WorkflowID), attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)))
	result, rErr := s.analytics().GetWorkflowStepDurations(ctx, projectID, input.WorkflowID, from, to)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get workflow step durations")
	}
	return &WorkflowStepDurationsOutput{Body: result}, nil
}

type WorkflowCompletionRatesInput struct {
	From   string `query:"from"`
	To     string `query:"to"`
	Bucket string `query:"bucket"`
}
type WorkflowCompletionRatesOutput struct{ Body any }

func (s *Server) handleWorkflowCompletionRates(ctx context.Context, input *WorkflowCompletionRatesInput) (*WorkflowCompletionRatesOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.WorkflowCompletionRates")
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
	result, rErr := s.analytics().GetWorkflowCompletionRates(ctx, projectID, from, to, bucket)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get workflow completion rates")
	}
	return &WorkflowCompletionRatesOutput{Body: result}, nil
}

type WorkflowAnalyticsSummaryInput struct {
	From string `query:"from"`
	To   string `query:"to"`
}
type WorkflowAnalyticsSummaryOutput struct{ Body any }

func (s *Server) handleWorkflowAnalyticsSummary(ctx context.Context, input *WorkflowAnalyticsSummaryInput) (*WorkflowAnalyticsSummaryOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.WorkflowAnalyticsSummary")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)))
	result, rErr := s.analytics().GetWorkflowSummary(ctx, projectID, from, to)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get workflow summary")
	}
	return &WorkflowAnalyticsSummaryOutput{Body: result}, nil
}
