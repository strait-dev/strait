package api

import (
	"net/http"
	"time"

	"orchestrator/internal/domain"
)

func (s *Server) handleListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	status := r.URL.Query().Get("status")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	deliveries, err := s.store.ListWebhookDeliveries(r.Context(), projectID, status, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list webhook deliveries")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(deliveries, limit, func(d domain.WebhookDelivery) string {
		return d.CreatedAt.Format(time.RFC3339Nano)
	}))
}
