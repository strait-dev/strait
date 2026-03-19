package api

import (
	"net/http"
	"strconv"
)

func (s *Server) handleGetCostInsights(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())

	from, to, ok := parseCostTimeRange(w, r)
	if !ok {
		return
	}

	threshold := 2.0
	if v := r.URL.Query().Get("threshold"); v != "" {
		parsed, parseErr := strconv.ParseFloat(v, 64)
		if parseErr != nil || parsed <= 0 {
			respondError(w, r, http.StatusBadRequest, "threshold must be a positive number")
			return
		}
		threshold = parsed
	}

	outliers, err := s.store.GetCostOutliers(r.Context(), projectID, from, to, threshold)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get cost insights")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"outliers":  outliers,
		"threshold": threshold,
	})
}
