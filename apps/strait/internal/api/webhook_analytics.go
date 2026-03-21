package api

import (
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func (s *Server) handleWebhookDeliveryStats(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.WebhookDeliveryStats")
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

	result, err := s.analytics().GetWebhookDeliveryStats(ctx, projectID, from, to)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get webhook delivery stats")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleWebhookEndpointHealth(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.WebhookEndpointHealth")
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

	result, err := s.analytics().GetWebhookEndpointHealth(ctx, projectID, from, to, bucket)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get webhook endpoint health")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleTopFailingWebhooks(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.TopFailingWebhooks")
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

	result, err := s.analytics().GetTopFailingWebhooks(ctx, projectID, from, to, limit)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get top failing webhooks")
		return
	}

	respondJSON(w, http.StatusOK, result)
}
