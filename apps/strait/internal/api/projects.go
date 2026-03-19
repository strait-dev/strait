package api

import (
	"errors"
	"log/slog"
	"net/http"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

// CreateProjectRequest is the request body for creating a project.
type CreateProjectRequest struct {
	ID    string `json:"id" validate:"required"`
	OrgID string `json:"org_id" validate:"required"`
	Name  string `json:"name" validate:"required,min=2"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req CreateProjectRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, req) {
		return
	}

	project := &domain.Project{
		ID:    req.ID,
		OrgID: req.OrgID,
		Name:  req.Name,
	}

	if err := s.store.CreateProject(r.Context(), project); err != nil {
		slog.Error("failed to create project", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to create project")
		return
	}

	if err := s.store.SeedProjectSystemRoles(r.Context(), project.ID); err != nil {
		slog.Error("failed to seed system roles for project", "project_id", project.ID, "error", err)
	}

	respondJSON(w, http.StatusCreated, project)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	project, err := s.store.GetProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			respondError(w, r, http.StatusNotFound, "project not found")
			return
		}
		slog.Error("failed to get project", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to get project")
		return
	}

	respondJSON(w, http.StatusOK, project)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		respondError(w, r, http.StatusBadRequest, APIError{
			Code:    ErrorCodeValidationError,
			Message: "org_id query parameter is required",
		})
		return
	}

	projects, err := s.store.ListProjectsByOrg(r.Context(), orgID)
	if err != nil {
		slog.Error("failed to list projects", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to list projects")
		return
	}

	if projects == nil {
		projects = []domain.Project{}
	}

	respondJSON(w, http.StatusOK, projects)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	err := s.store.DeleteProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			respondError(w, r, http.StatusNotFound, "project not found")
			return
		}
		slog.Error("failed to delete project", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to delete project")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
