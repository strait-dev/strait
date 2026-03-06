package api

import (
	"errors"
	"net/http"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
)

type CreateEnvironmentRequest struct {
	ProjectID string            `json:"project_id"`
	Name      string            `json:"name"`
	Slug      string            `json:"slug"`
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
	if !s.config.FFEnvironments {
		respondError(w, http.StatusNotFound, "environments feature is not enabled")
		return
	}

	var req CreateEnvironmentRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ProjectID == "" || req.Name == "" || req.Slug == "" {
		respondError(w, http.StatusBadRequest, "missing required fields")
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
		respondError(w, http.StatusInternalServerError, "failed to create environment")
		return
	}

	respondJSON(w, http.StatusCreated, env)
}

func (s *Server) handleGetEnvironment(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFEnvironments {
		respondError(w, http.StatusNotFound, "environments feature is not enabled")
		return
	}

	envID := chi.URLParam(r, "envID")
	env, err := s.store.GetEnvironment(r.Context(), envID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			respondError(w, http.StatusNotFound, "environment not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get environment")
		return
	}

	resolvedVariables, err := s.store.GetResolvedEnvironmentVariables(r.Context(), envID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			respondError(w, http.StatusNotFound, "environment not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to resolve environment variables")
		return
	}

	respondJSON(w, http.StatusOK, EnvironmentResponse{
		Environment:       *env,
		ResolvedVariables: resolvedVariables,
	})
}

func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFEnvironments {
		respondError(w, http.StatusNotFound, "environments feature is not enabled")
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	envs, err := s.store.ListEnvironments(r.Context(), projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list environments")
		return
	}

	respondJSON(w, http.StatusOK, envs)
}

func (s *Server) handleUpdateEnvironment(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFEnvironments {
		respondError(w, http.StatusNotFound, "environments feature is not enabled")
		return
	}

	envID := chi.URLParam(r, "envID")
	env, err := s.store.GetEnvironment(r.Context(), envID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			respondError(w, http.StatusNotFound, "environment not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get environment")
		return
	}

	var req UpdateEnvironmentRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
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
			respondError(w, http.StatusNotFound, "environment not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to update environment")
		return
	}

	respondJSON(w, http.StatusOK, env)
}

func (s *Server) handleDeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFEnvironments {
		respondError(w, http.StatusNotFound, "environments feature is not enabled")
		return
	}

	envID := chi.URLParam(r, "envID")
	if err := s.store.DeleteEnvironment(r.Context(), envID); err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			respondError(w, http.StatusNotFound, "environment not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to delete environment")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}

func (s *Server) handleGetResolvedVariables(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFEnvironments {
		respondError(w, http.StatusNotFound, "environments feature is not enabled")
		return
	}

	envID := chi.URLParam(r, "envID")
	resolvedVariables, err := s.store.GetResolvedEnvironmentVariables(r.Context(), envID)
	if err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			respondError(w, http.StatusNotFound, "environment not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to resolve environment variables")
		return
	}

	respondJSON(w, http.StatusOK, map[string]map[string]string{"variables": resolvedVariables})
}
