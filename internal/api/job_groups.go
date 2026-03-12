package api

import (
	"errors"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

type CreateJobGroupRequest struct {
	ProjectID   string `json:"project_id" validate:"required"`
	Name        string `json:"name" validate:"required"`
	Slug        string `json:"slug" validate:"required"`
	Description string `json:"description,omitempty"`
}

type UpdateJobGroupRequest struct {
	Name        *string `json:"name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (s *Server) handleCreateJobGroup(w http.ResponseWriter, r *http.Request) {
	var req CreateJobGroupRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	group := &domain.JobGroup{
		ProjectID:   req.ProjectID,
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
	}

	if err := s.store.CreateJobGroup(r.Context(), group); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create job group")
		return
	}

	respondJSON(w, http.StatusCreated, group)
}

func (s *Server) handleGetJobGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupID")
	group, err := s.store.GetJobGroup(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			respondError(w, r, http.StatusNotFound, "job group not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get job group")
		return
	}

	respondJSON(w, http.StatusOK, group)
}

func (s *Server) handleListJobGroups(w http.ResponseWriter, r *http.Request) {
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

	groups, err := s.store.ListJobGroups(r.Context(), projectID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list job groups")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(groups, limit, func(g domain.JobGroup) string {
		return g.CreatedAt.Format(time.RFC3339Nano)
	}))
}
func (s *Server) handleUpdateJobGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupID")
	group, err := s.store.GetJobGroup(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			respondError(w, r, http.StatusNotFound, "job group not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get job group")
		return
	}

	var req UpdateJobGroupRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
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
			respondError(w, r, http.StatusNotFound, "job group not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to update job group")
		return
	}

	respondJSON(w, http.StatusOK, group)
}

func (s *Server) handleDeleteJobGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupID")
	if err := s.store.DeleteJobGroup(r.Context(), groupID); err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			respondError(w, r, http.StatusNotFound, "job group not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to delete job group")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}

func (s *Server) handleListJobsByGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupID")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	jobs, err := s.store.ListJobsByGroup(r.Context(), groupID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list jobs by group")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(jobs, limit, func(j domain.Job) string {
		return j.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handlePauseAllJobsByGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupID")
	if err := s.store.PauseJobsByGroup(r.Context(), groupID); err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			respondError(w, r, http.StatusNotFound, "job group not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to pause jobs in group")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) handleResumeAllJobsByGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupID")
	if err := s.store.ResumeJobsByGroup(r.Context(), groupID); err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			respondError(w, r, http.StatusNotFound, "job group not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to resume jobs in group")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (s *Server) handleGetJobGroupStats(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupID")
	stats, err := s.store.GetJobGroupStats(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			respondError(w, r, http.StatusNotFound, "job group not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get job group stats")
		return
	}

	respondJSON(w, http.StatusOK, stats)
}
