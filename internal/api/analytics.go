package api

import (
	"net/http"
	"strconv"
)

func (s *Server) handleGetPerformanceAnalytics(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())

	periodHours := 24
	if v := r.URL.Query().Get("period_hours"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 || parsed > 720 {
			respondError(w, r, http.StatusBadRequest, "period_hours must be between 1 and 720")
			return
		}
		periodHours = parsed
	}

	analytics, err := s.store.GetPerformanceAnalytics(r.Context(), projectID, periodHours)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get analytics")
		return
	}

	respondJSON(w, http.StatusOK, analytics)
}
