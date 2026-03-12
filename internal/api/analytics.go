package api

import (
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func (s *Server) handleGetPerformanceAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.GetPerformanceAnalytics")
	defer span.End()

	projectID := projectIDFromContext(ctx)

	periodHours := 24
	if v := r.URL.Query().Get("period_hours"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 || parsed > 720 {
			respondError(w, r, http.StatusBadRequest, "period_hours must be between 1 and 720")
			return
		}
		periodHours = parsed
	}

	span.SetAttributes(attribute.Int("period_hours", periodHours))

	analytics, err := s.store.GetPerformanceAnalytics(ctx, projectID, periodHours)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get analytics")
		return
	}

	respondJSON(w, http.StatusOK, analytics)
}
