package api

import (
	"errors"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

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

func (s *Server) handleGetJobVersion(w http.ResponseWriter, r *http.Request) {
	versionID := chi.URLParam(r, "versionID")

	version, err := s.store.GetJobVersionByVersionID(r.Context(), versionID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			respondError(w, r, http.StatusNotFound, "version not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get job version")
		return
	}

	respondJSON(w, http.StatusOK, version)
}
