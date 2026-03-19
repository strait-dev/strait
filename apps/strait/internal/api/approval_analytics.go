package api

import (
	"net/http"
	"time"
)

const maxApprovalStatsRange = 90 * 24 * time.Hour

func (s *Server) handleGetApprovalStats(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())

	from, to, ok := parseCostTimeRange(w, r)
	if !ok {
		return
	}

	if to.Before(from) {
		respondError(w, r, http.StatusBadRequest, "from must be before to")
		return
	}
	if to.Sub(from) > maxApprovalStatsRange {
		respondError(w, r, http.StatusBadRequest, "time range must not exceed 90 days")
		return
	}

	stats, err := s.store.GetApprovalStats(r.Context(), projectID, from, to)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get approval stats")
		return
	}

	respondJSON(w, http.StatusOK, stats)
}
