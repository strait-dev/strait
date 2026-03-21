package api

import (
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func (s *Server) handleGetCostAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.GetCostAnalytics")
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

	analytics, err := s.analytics().GetCostAnalytics(ctx, projectID, from, to)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get cost analytics")
		return
	}

	respondJSON(w, http.StatusOK, analytics)
}

func (s *Server) handleGetCostTrends(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.GetCostTrends")
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

	trends, err := s.analytics().GetCostTrends(ctx, projectID, from, to)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get cost trends")
		return
	}

	respondJSON(w, http.StatusOK, trends)
}

func (s *Server) handleGetTopCosts(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.GetTopCosts")
	defer span.End()

	projectID := projectIDFromContext(ctx)

	from, to, ok := parseCostTimeRange(w, r)
	if !ok {
		return
	}

	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 || parsed > 100 {
			respondError(w, r, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}

	span.SetAttributes(
		attribute.String("from", from.Format(time.RFC3339)),
		attribute.String("to", to.Format(time.RFC3339)),
		attribute.Int("limit", limit),
	)

	items, err := s.analytics().GetTopCosts(ctx, projectID, from, to, limit)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get top costs")
		return
	}

	respondJSON(w, http.StatusOK, items)
}

func (s *Server) handleGetComputeCostAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.GetComputeCostAnalytics")
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

	analytics, err := s.analytics().GetComputeCostAnalytics(ctx, projectID, from, to)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get compute cost analytics")
		return
	}

	respondJSON(w, http.StatusOK, analytics)
}

const maxCostWindow = 90 * 24 * time.Hour

// parseCostTimeRange extracts and validates from/to query parameters.
// Returns false if validation failed (error already written to response).
func parseCostTimeRange(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, bool) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	if fromStr == "" || toStr == "" {
		respondError(w, r, http.StatusBadRequest, "from and to query parameters are required (RFC3339 format)")
		return time.Time{}, time.Time{}, false
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, "from must be in RFC3339 format")
		return time.Time{}, time.Time{}, false
	}

	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, "to must be in RFC3339 format")
		return time.Time{}, time.Time{}, false
	}

	if !to.After(from) {
		respondError(w, r, http.StatusBadRequest, "to must be after from")
		return time.Time{}, time.Time{}, false
	}

	if to.Sub(from) > maxCostWindow {
		respondError(w, r, http.StatusBadRequest, "time range must not exceed 90 days")
		return time.Time{}, time.Time{}, false
	}

	return from, to, true
}
