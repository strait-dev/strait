package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func (s *Server) handleWorkflowStepDurations(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.WorkflowStepDurations")
	defer span.End()

	projectID := projectIDFromContext(ctx)
	workflowID := chi.URLParam(r, "workflowID")

	from, to, ok := parseCostTimeRange(w, r)
	if !ok {
		return
	}

	span.SetAttributes(
		attribute.String("workflow_id", workflowID),
		attribute.String("from", from.Format(time.RFC3339)),
		attribute.String("to", to.Format(time.RFC3339)),
	)

	result, err := s.analytics().GetWorkflowStepDurations(ctx, projectID, workflowID, from, to)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow step durations")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleWorkflowCompletionRates(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.WorkflowCompletionRates")
	defer span.End()

	projectID := projectIDFromContext(ctx)

	from, to, ok := parseCostTimeRange(w, r)
	if !ok {
		return
	}

	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		bucket = "day"
	}
	if bucket != "hour" && bucket != "day" {
		respondError(w, r, http.StatusBadRequest, "bucket must be 'hour' or 'day'")
		return
	}

	span.SetAttributes(
		attribute.String("from", from.Format(time.RFC3339)),
		attribute.String("to", to.Format(time.RFC3339)),
		attribute.String("bucket", bucket),
	)

	result, err := s.analytics().GetWorkflowCompletionRates(ctx, projectID, from, to, bucket)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow completion rates")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleWorkflowAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.WorkflowAnalyticsSummary")
	defer span.End()

	projectID := projectIDFromContext(ctx)

	from, to, ok := parseCostTimeRange(w, r)
	if !ok {
		return
	}

	span.SetAttributes(
		attribute.String("from", from.Format(time.RFC3339)),
		attribute.String("to", to.Format(time.RFC3339)),
	)

	result, err := s.analytics().GetWorkflowSummary(ctx, projectID, from, to)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow summary")
		return
	}

	respondJSON(w, http.StatusOK, result)
}
