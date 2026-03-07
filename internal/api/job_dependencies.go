package api

import (
	"errors"
	"net/http"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
)

type CreateJobDependencyRequest struct {
	DependsOnJobID string `json:"depends_on_job_id" validate:"required"`
	Condition      string `json:"condition,omitempty"`
}

func (s *Server) handleCreateJobDependency(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFJobDependencies {
		respondError(w, r, http.StatusNotFound, "job dependencies feature is not enabled")
		return
	}

	jobID := chi.URLParam(r, "jobID")

	if _, err := s.store.GetJob(r.Context(), jobID); err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			respondError(w, r, http.StatusNotFound, "job not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get job")
		return
	}

	var req CreateJobDependencyRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	if req.DependsOnJobID == jobID {
		respondError(w, r, http.StatusBadRequest, "job cannot depend on itself")
		return
	}

	if _, err := s.store.GetJob(r.Context(), req.DependsOnJobID); err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			respondError(w, r, http.StatusBadRequest, "depends_on_job_id does not exist")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get dependency job")
		return
	}

	condition := req.Condition
	if condition == "" {
		condition = "completed"
	}
	if !isValidDependencyCondition(condition) {
		respondError(w, r, http.StatusBadRequest, "condition must be one of: completed, failed, any")
		return
	}

	dep := &domain.JobDependency{
		JobID:          jobID,
		DependsOnJobID: req.DependsOnJobID,
		Condition:      condition,
	}

	if err := s.store.CreateJobDependency(r.Context(), dep); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create job dependency")
		return
	}

	respondJSON(w, http.StatusCreated, dep)
}

func (s *Server) handleListJobDependencies(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFJobDependencies {
		respondError(w, r, http.StatusNotFound, "job dependencies feature is not enabled")
		return
	}

	jobID := chi.URLParam(r, "jobID")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	deps, err := s.store.ListJobDependencies(r.Context(), jobID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list job dependencies")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(deps, limit, func(d domain.JobDependency) string {
		return d.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleDeleteJobDependency(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFJobDependencies {
		respondError(w, r, http.StatusNotFound, "job dependencies feature is not enabled")
		return
	}

	jobID := chi.URLParam(r, "jobID")
	if _, err := s.store.GetJob(r.Context(), jobID); err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			respondError(w, r, http.StatusNotFound, "job not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get job")
		return
	}

	depID := chi.URLParam(r, "depID")
	if err := s.store.DeleteJobDependency(r.Context(), depID); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to delete job dependency")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}

func isValidDependencyCondition(condition string) bool {
	switch condition {
	case "completed", "failed", "any":
		return true
	default:
		return false
	}
}
