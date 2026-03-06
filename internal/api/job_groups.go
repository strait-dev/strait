package api

import (
	"errors"
	"net/http"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
)

type CreateJobGroupRequest struct {
	ProjectID   string `json:"project_id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
}

type UpdateJobGroupRequest struct {
	Name        *string `json:"name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (s *Server) handleCreateJobGroup(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFJobGroups {
		respondError(w, http.StatusBadRequest, "job groups feature is not enabled")
		return
	}

	var req CreateJobGroupRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ProjectID == "" || req.Name == "" || req.Slug == "" {
		respondError(w, http.StatusBadRequest, "missing required fields")
		return
	}

	group := &domain.JobGroup{
		ProjectID:   req.ProjectID,
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
	}

	if err := s.store.CreateJobGroup(r.Context(), group); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create job group")
		return
	}

	respondJSON(w, http.StatusCreated, group)
}

func (s *Server) handleGetJobGroup(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFJobGroups {
		respondError(w, http.StatusBadRequest, "job groups feature is not enabled")
		return
	}

	groupID := chi.URLParam(r, "groupID")
	group, err := s.store.GetJobGroup(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			respondError(w, http.StatusNotFound, "job group not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get job group")
		return
	}

	respondJSON(w, http.StatusOK, group)
}

func (s *Server) handleListJobGroups(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFJobGroups {
		respondError(w, http.StatusBadRequest, "job groups feature is not enabled")
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	groups, err := s.store.ListJobGroups(r.Context(), projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list job groups")
		return
	}

	respondJSON(w, http.StatusOK, groups)
}

func (s *Server) handleUpdateJobGroup(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFJobGroups {
		respondError(w, http.StatusBadRequest, "job groups feature is not enabled")
		return
	}

	groupID := chi.URLParam(r, "groupID")
	group, err := s.store.GetJobGroup(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			respondError(w, http.StatusNotFound, "job group not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get job group")
		return
	}

	var req UpdateJobGroupRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != nil {
		group.Name = *req.Name
	}
	if req.Slug != nil {
		group.Slug = *req.Slug
	}
	if req.Description != nil {
		group.Description = *req.Description
	}

	if err := s.store.UpdateJobGroup(r.Context(), group); err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			respondError(w, http.StatusNotFound, "job group not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to update job group")
		return
	}

	respondJSON(w, http.StatusOK, group)
}

func (s *Server) handleDeleteJobGroup(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFJobGroups {
		respondError(w, http.StatusBadRequest, "job groups feature is not enabled")
		return
	}

	groupID := chi.URLParam(r, "groupID")
	if err := s.store.DeleteJobGroup(r.Context(), groupID); err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			respondError(w, http.StatusNotFound, "job group not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to delete job group")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}

func (s *Server) handleListJobsByGroup(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFJobGroups {
		respondError(w, http.StatusBadRequest, "job groups feature is not enabled")
		return
	}

	groupID := chi.URLParam(r, "groupID")
	jobs, err := s.store.ListJobsByGroup(r.Context(), groupID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list jobs by group")
		return
	}

	respondJSON(w, http.StatusOK, jobs)
}
