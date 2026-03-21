package api

import (
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func (s *Server) handleTagSummary(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.TagSummary")
	defer span.End()

	projectID := projectIDFromContext(ctx)

	from, to, ok := parseCostTimeRange(w, r)
	if !ok {
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 || parsed > 500 {
			respondError(w, r, http.StatusBadRequest, "limit must be between 1 and 500")
			return
		}
		limit = parsed
	}

	span.SetAttributes(
		attribute.String("from", from.Format(time.RFC3339)),
		attribute.String("to", to.Format(time.RFC3339)),
		attribute.Int("limit", limit),
	)

	result, err := s.analytics().GetTagSummary(ctx, projectID, from, to, limit)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get tag summary")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleTopFailingTags(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.TopFailingTags")
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

	result, err := s.analytics().GetTopFailingTags(ctx, projectID, from, to, limit)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get top failing tags")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleTagCost(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.TagCost")
	defer span.End()

	projectID := projectIDFromContext(ctx)

	from, to, ok := parseCostTimeRange(w, r)
	if !ok {
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 || parsed > 500 {
			respondError(w, r, http.StatusBadRequest, "limit must be between 1 and 500")
			return
		}
		limit = parsed
	}

	span.SetAttributes(
		attribute.String("from", from.Format(time.RFC3339)),
		attribute.String("to", to.Format(time.RFC3339)),
		attribute.Int("limit", limit),
	)

	result, err := s.analytics().GetTagCost(ctx, projectID, from, to, limit)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get tag cost")
		return
	}

	respondJSON(w, http.StatusOK, result)
}
