package api

import (
	"net/http"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

// ProjectSettingsResponse is the API representation of project settings.
type ProjectSettingsResponse struct {
	ProjectID     string `json:"project_id"`
	DefaultRegion string `json:"default_region"`
	PlanTier      string `json:"plan_tier"`
}

// UpdateProjectSettingsRequest is the request body for updating project settings.
type UpdateProjectSettingsRequest struct {
	DefaultRegion *string `json:"default_region,omitempty"`
}

func (s *Server) handleGetProjectSettings(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	quota, err := s.store.GetProjectQuota(r.Context(), projectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get project settings")
		return
	}

	resp := ProjectSettingsResponse{
		ProjectID: projectID,
		PlanTier:  string(domain.PlanFree),
	}
	if quota != nil {
		resp.DefaultRegion = quota.DefaultRegion
		if quota.PlanTier != "" {
			resp.PlanTier = quota.PlanTier
		}
	}

	respondJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUpdateProjectSettings(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	var req UpdateProjectSettingsRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DefaultRegion != nil {
		region := *req.DefaultRegion
		// Validate region code and plan-based gating.
		if !s.validateRegionForPlan(w, r, projectID, region) {
			return
		}
		if err := s.store.UpdateProjectDefaultRegion(r.Context(), projectID, region); err != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to update project settings")
			return
		}
	}

	// Return the updated settings.
	s.handleGetProjectSettings(w, r)
}
