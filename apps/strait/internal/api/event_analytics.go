package api

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func (s *Server) handleEventVolume(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.EventVolume")
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

	result, err := s.analytics().GetEventVolume(ctx, projectID, from, to, bucket)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get event volume")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleEventLatency(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.EventLatency")
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

	result, err := s.analytics().GetEventLatency(ctx, projectID, from, to)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get event latency")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleCostForecast(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.CostForecast")
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

	result, err := s.analytics().GetCostForecast(ctx, projectID, from, to)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get cost forecast")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleCostByTrigger(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.CostByTrigger")
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

	result, err := s.analytics().GetCostByTrigger(ctx, projectID, from, to)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get cost by trigger")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleCostByMachine(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.CostByMachine")
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

	result, err := s.analytics().GetCostByMachine(ctx, projectID, from, to)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get cost by machine")
		return
	}

	respondJSON(w, http.StatusOK, result)
}
