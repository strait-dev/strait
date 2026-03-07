package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListJobVersions(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	versions, err := s.store.ListJobVersionsByJob(r.Context(), jobID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list job versions")
		return
	}

	respondJSON(w, http.StatusOK, versions)
}
