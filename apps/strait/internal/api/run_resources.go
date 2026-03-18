package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListRunResources(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	var from, to *time.Time
	if v := r.URL.Query().Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			respondError(w, r, http.StatusBadRequest, "invalid from param: must be RFC3339")
			return
		}
		from = &t
	}
	if v := r.URL.Query().Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			respondError(w, r, http.StatusBadRequest, "invalid to param: must be RFC3339")
			return
		}
		to = &t
	}

	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			respondError(w, r, http.StatusBadRequest, "invalid limit param")
			return
		}
		if n > 1000 {
			n = 1000
		}
		limit = n
	}

	snapshots, err := s.store.ListRunResourceSnapshots(r.Context(), runID, from, to, limit)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list resource snapshots")
		return
	}

	respondJSON(w, http.StatusOK, snapshots)
}
