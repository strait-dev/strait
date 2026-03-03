package api

import (
	"net/http"
	"strconv"
)

func (s *Server) handleListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	limit := 50
	if limitRaw := r.URL.Query().Get("limit"); limitRaw != "" {
		parsedLimit, err := strconv.Atoi(limitRaw)
		if err != nil || parsedLimit <= 0 {
			respondError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsedLimit > 100 {
			parsedLimit = 100
		}
		limit = parsedLimit
	}

	deliveries, err := s.store.ListWebhookDeliveries(r.Context(), status, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list webhook deliveries")
		return
	}

	respondJSON(w, http.StatusOK, deliveries)
}
