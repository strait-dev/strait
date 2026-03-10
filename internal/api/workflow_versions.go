package api

import (
	"errors"
	"net/http"

	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListWorkflowVersions(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")

	limit, _, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	versions, err := s.store.ListWorkflowVersions(r.Context(), workflowID, limit)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow versions")
		return
	}

	respondJSON(w, http.StatusOK, versions)
}

func (s *Server) handleGetWorkflowVersion(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	versionID := chi.URLParam(r, "versionID")

	version, err := s.store.GetWorkflowVersionByVersionID(r.Context(), workflowID, versionID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowVersionNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow version not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow version")
		return
	}

	respondJSON(w, http.StatusOK, version)
}

func (s *Server) handleListWorkflowVersionSteps(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	versionID := chi.URLParam(r, "versionID")

	version, err := s.store.GetWorkflowVersionByVersionID(r.Context(), workflowID, versionID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowVersionNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow version not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow version")
		return
	}

	steps, err := s.store.ListStepsByWorkflowVersion(r.Context(), workflowID, version.Version)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow version steps")
		return
	}

	respondJSON(w, http.StatusOK, steps)
}
