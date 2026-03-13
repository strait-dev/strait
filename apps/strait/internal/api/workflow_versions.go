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

func (s *Server) handleWorkflowVersionDiff(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	fromVersionID := chi.URLParam(r, "fromVersionID")
	toVersionID := chi.URLParam(r, "toVersionID")

	fromVersion, err := s.store.GetWorkflowVersionByVersionID(r.Context(), workflowID, fromVersionID)
	if err != nil {
		respondError(w, r, http.StatusNotFound, "from workflow version not found")
		return
	}
	toVersion, err := s.store.GetWorkflowVersionByVersionID(r.Context(), workflowID, toVersionID)
	if err != nil {
		respondError(w, r, http.StatusNotFound, "to workflow version not found")
		return
	}
	fromSteps, err := s.store.ListStepsByWorkflowVersion(r.Context(), workflowID, fromVersion.Version)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list from steps")
		return
	}
	toSteps, err := s.store.ListStepsByWorkflowVersion(r.Context(), workflowID, toVersion.Version)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list to steps")
		return
	}

	fromMap := map[string]bool{}
	for _, st := range fromSteps {
		fromMap[st.StepRef] = true
	}
	toMap := map[string]bool{}
	for _, st := range toSteps {
		toMap[st.StepRef] = true
	}
	added := []string{}
	removed := []string{}
	for ref := range toMap {
		if !fromMap[ref] {
			added = append(added, ref)
		}
	}
	for ref := range fromMap {
		if !toMap[ref] {
			removed = append(removed, ref)
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{"from_version_id": fromVersionID, "to_version_id": toVersionID, "added_steps": added, "removed_steps": removed})
}

func (s *Server) handleWorkflowVersionImpact(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	versionID := chi.URLParam(r, "versionID")
	version, err := s.store.GetWorkflowVersionByVersionID(r.Context(), workflowID, versionID)
	if err != nil {
		respondError(w, r, http.StatusNotFound, "workflow version not found")
		return
	}
	runs, err := s.store.ListWorkflowRuns(r.Context(), workflowID, 500, nil)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow runs")
		return
	}
	pinned := 0
	for _, run := range runs {
		if run.WorkflowVersion == version.Version {
			pinned++
		}
	}
	respondJSON(w, http.StatusOK, map[string]any{"version_id": versionID, "matching_runs": pinned, "sampled_runs": len(runs)})
}
