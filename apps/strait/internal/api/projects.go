package api

import (
	"errors"
	"log/slog"
	"net/http"

	"strait/internal/billing"
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
	// Project creation is an org-level operation; API keys are project-scoped
	// and have no org context, so restrict to internal-secret callers.
	if scopesFromContext(r.Context()) != nil {
		respondError(w, r, http.StatusForbidden, "project creation requires internal secret")
		return
	}

	var req CreateProjectRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, req) {
		return
	}

	if s.billingEnforcer != nil {
		if err := s.billingEnforcer.CheckProjectLimit(r.Context(), req.OrgID); err != nil {
			var le *billing.LimitError
			if errors.As(err, &le) {
				respondError(w, r, http.StatusForbidden, le)
				return
			}
			slog.Error("failed to check project limit", "error", err)
			respondError(w, r, http.StatusInternalServerError, "failed to check project limit")
			return
		}
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

	// Auto-activate referral on first project creation.
	if s.referralService != nil {
		projects, listErr := s.store.ListProjectsByOrg(r.Context(), req.OrgID)
		if listErr != nil {
			slog.Warn("failed to count projects for referral auto-activation", "org_id", req.OrgID, "error", listErr)
		} else if len(projects) == 1 {
			if activateErr := s.referralService.AutoActivateReferral(r.Context(), req.OrgID); activateErr != nil {
				slog.Warn("failed to auto-activate referral", "org_id", req.OrgID, "error", activateErr)
			}
		}
	}

	respondJSON(w, http.StatusCreated, project)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	// Project listing is an org-level operation; API keys are project-scoped.
	if scopesFromContext(r.Context()) != nil {
		respondError(w, r, http.StatusForbidden, "project listing requires internal secret")
		return
	}

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

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	// API key callers can only read their own project.
	if scopesFromContext(r.Context()) != nil {
		if projectIDFromContext(r.Context()) != projectID {
			respondError(w, r, http.StatusForbidden, "access denied")
			return
		}
	}

	// Internal-secret callers: verify project belongs to caller's org context.
	if scopesFromContext(r.Context()) == nil {
		if err := s.validateProjectBelongsToCallerOrg(r.Context(), projectID); err != nil {
			// Gracefully allow when no billing enforcer or no project context
			// (backwards compat for callers without X-Project-Id).
			if projectIDFromContext(r.Context()) != "" {
				respondError(w, r, http.StatusForbidden, "access denied")
				return
			}
		}
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

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	// API key callers can only delete their own project.
	if scopesFromContext(r.Context()) != nil {
		if projectIDFromContext(r.Context()) != projectID {
			respondError(w, r, http.StatusForbidden, "access denied")
			return
		}
	}

	// Internal-secret callers: verify project belongs to caller's org context.
	if scopesFromContext(r.Context()) == nil {
		if err := s.validateProjectBelongsToCallerOrg(r.Context(), projectID); err != nil {
			if projectIDFromContext(r.Context()) != "" {
				respondError(w, r, http.StatusForbidden, "access denied")
				return
			}
		}
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
