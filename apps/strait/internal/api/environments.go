package api

import (
	"errors"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

type CreateEnvironmentRequest struct {
	ProjectID string            `json:"project_id" validate:"required"`
	Name      string            `json:"name" validate:"required"`
	Slug      string            `json:"slug" validate:"required"`
	ParentID  string            `json:"parent_id,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
}

type UpdateEnvironmentRequest struct {
	Name      *string            `json:"name,omitempty"`
	Slug      *string            `json:"slug,omitempty"`
	ParentID  *string            `json:"parent_id,omitempty"`
	Variables *map[string]string `json:"variables,omitempty"`
}

type EnvironmentResponse struct {
	domain.Environment
	ResolvedVariables map[string]string `json:"resolved_variables,omitempty"`
}

func (s *Server) handleCreateEnvironment(w http.ResponseWriter, r *http.Request) {
	var req CreateEnvironmentRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	env := &domain.Environment{
		ProjectID: req.ProjectID,
		Name:      req.Name,
		Slug:      req.Slug,
		ParentID:  req.ParentID,
		Variables: req.Variables,
	}

	if err := s.store.CreateEnvironment(r.Context(), env); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create environment")
		return
	}

	respondJSON(w, http.StatusCreated, env)
}

func (s *Server) handleGetEnvironment(w http.ResponseWriter, r *http.Request) {
	envID := chi.URLParam(r, "envID")
	env, err := s.store.GetEnvironment(r.Context(), envID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			respondError(w, r, http.StatusNotFound, "environment not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get environment")
		return
	}

	resolvedVariables, err := s.store.GetResolvedEnvironmentVariables(r.Context(), envID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			respondError(w, r, http.StatusNotFound, "environment not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to resolve environment variables")
		return
	}

	respondJSON(w, http.StatusOK, EnvironmentResponse{
		Environment:       *env,
		ResolvedVariables: resolvedVariables,
	})
}

func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	envs, err := s.store.ListEnvironments(r.Context(), projectID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list environments")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(envs, limit, func(e domain.Environment) string {
		return e.CreatedAt.Format(time.RFC3339Nano)
	}))
}
func (s *Server) handleUpdateEnvironment(w http.ResponseWriter, r *http.Request) {
	envID := chi.URLParam(r, "envID")
	env, err := s.store.GetEnvironment(r.Context(), envID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			respondError(w, r, http.StatusNotFound, "environment not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get environment")
		return
	}

	var req UpdateEnvironmentRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != nil {
		env.Name = *req.Name
	}
	if req.Slug != nil {
		env.Slug = *req.Slug
	}
	if req.ParentID != nil {
		env.ParentID = *req.ParentID
	}
	if req.Variables != nil {
		env.Variables = *req.Variables
	}

	if err := s.store.UpdateEnvironment(r.Context(), env); err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			respondError(w, r, http.StatusNotFound, "environment not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to update environment")
		return
	}

	respondJSON(w, http.StatusOK, env)
}

func (s *Server) handleDeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	envID := chi.URLParam(r, "envID")
	if err := s.store.DeleteEnvironment(r.Context(), envID); err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			respondError(w, r, http.StatusNotFound, "environment not found")
			return
		}
		if errors.Is(err, store.ErrStandardEnvironment) {
			respondError(w, r, http.StatusForbidden, "cannot delete standard environment")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to delete environment")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}

func (s *Server) handleGetResolvedVariables(w http.ResponseWriter, r *http.Request) {
	envID := chi.URLParam(r, "envID")
	resolvedVariables, err := s.store.GetResolvedEnvironmentVariables(r.Context(), envID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			respondError(w, r, http.StatusNotFound, "environment not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to resolve environment variables")
		return
	}

	respondJSON(w, http.StatusOK, map[string]map[string]string{"variables": resolvedVariables})
}
