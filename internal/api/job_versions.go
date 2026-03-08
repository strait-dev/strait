package api

import (
	"net/http"
	"time"

	"orchestrator/internal/domain"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListJobVersions(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	versions, err := s.store.ListJobVersionsByJob(r.Context(), jobID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list job versions")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(versions, limit, func(v domain.JobVersion) string {
		return v.CreatedAt.Format(time.RFC3339Nano)
	}))
}
