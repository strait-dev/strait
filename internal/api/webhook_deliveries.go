package api

import (
	"net/http"
	"strconv"
)

func (s *Server) handleListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	status := r.URL.Query().Get("status")
	limit := defaultPageLimit
	if limitRaw := r.URL.Query().Get("limit"); limitRaw != "" {
		parsedLimit, err := strconv.Atoi(limitRaw)
		if err != nil || parsedLimit <= 0 {
			respondError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsedLimit > maxPageLimit {
			parsedLimit = maxPageLimit
		}
		limit = parsedLimit
	}

	deliveries, err := s.store.ListWebhookDeliveries(r.Context(), projectID, status, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list webhook deliveries")
		return
	}

	respondJSON(w, http.StatusOK, deliveries)
}
