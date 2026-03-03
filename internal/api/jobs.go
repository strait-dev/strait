package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"orchestrator/internal/domain"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"
)

type CreateJobRequest struct {
	ProjectID     string          `json:"project_id"`
	Name          string          `json:"name"`
	Slug          string          `json:"slug"`
	Description   string          `json:"description,omitempty"`
	Cron          string          `json:"cron,omitempty"`
	PayloadSchema json.RawMessage `json:"payload_schema,omitempty"`
	EndpointURL   string          `json:"endpoint_url"`
	MaxAttempts   int             `json:"max_attempts"`
	TimeoutSecs   int             `json:"timeout_secs"`
}

type UpdateJobRequest struct {
	Name          *string          `json:"name,omitempty"`
	Slug          *string          `json:"slug,omitempty"`
	Description   *string          `json:"description,omitempty"`
	Cron          *string          `json:"cron,omitempty"`
	PayloadSchema *json.RawMessage `json:"payload_schema,omitempty"`
	EndpointURL   *string          `json:"endpoint_url,omitempty"`
	MaxAttempts   *int             `json:"max_attempts,omitempty"`
	TimeoutSecs   *int             `json:"timeout_secs,omitempty"`
	Enabled       *bool            `json:"enabled,omitempty"`
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req CreateJobRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ProjectID == "" || req.Name == "" || req.Slug == "" || req.EndpointURL == "" {
		respondError(w, http.StatusBadRequest, "missing required fields")
		return
	}

	if err := validateURL(req.EndpointURL); err != nil {
		respondError(w, http.StatusBadRequest, "invalid endpoint_url: "+err.Error())
		return
	}

	if req.MaxAttempts == 0 {
		req.MaxAttempts = 3
	}
	if req.TimeoutSecs == 0 {
		req.TimeoutSecs = 300
	}

	if req.Cron != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(req.Cron); err != nil {
			respondError(w, http.StatusBadRequest, "invalid cron expression")
			return
		}
	}

	job := &domain.Job{
		ProjectID:     req.ProjectID,
		Name:          req.Name,
		Slug:          req.Slug,
		Description:   req.Description,
		Cron:          req.Cron,
		PayloadSchema: req.PayloadSchema,
		EndpointURL:   req.EndpointURL,
		MaxAttempts:   req.MaxAttempts,
		TimeoutSecs:   req.TimeoutSecs,
		Enabled:       true,
	}

	if err := s.store.CreateJob(r.Context(), job); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create job")
		return
	}

	respondJSON(w, http.StatusCreated, job)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	job, err := s.store.GetJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "job not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get job")
		return
	}

	respondJSON(w, http.StatusOK, job)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	jobs, err := s.store.ListJobs(r.Context(), projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}

	respondJSON(w, http.StatusOK, jobs)
}

func (s *Server) handleUpdateJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	job, err := s.store.GetJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "job not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get job")
		return
	}

	var req UpdateJobRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Cron != nil && *req.Cron != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(*req.Cron); err != nil {
			respondError(w, http.StatusBadRequest, "invalid cron expression")
			return
		}
	}

	if req.Name != nil {
		job.Name = *req.Name
	}
	if req.Slug != nil {
		job.Slug = *req.Slug
	}
	if req.Description != nil {
		job.Description = *req.Description
	}
	if req.Cron != nil {
		job.Cron = *req.Cron
	}
	if req.PayloadSchema != nil {
		job.PayloadSchema = *req.PayloadSchema
	}
	if req.EndpointURL != nil {
		job.EndpointURL = *req.EndpointURL
	}
	if req.MaxAttempts != nil {
		job.MaxAttempts = *req.MaxAttempts
	}
	if req.TimeoutSecs != nil {
		job.TimeoutSecs = *req.TimeoutSecs
	}
	if req.Enabled != nil {
		job.Enabled = *req.Enabled
	}

	if err := s.store.UpdateJob(r.Context(), job); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update job")
		return
	}

	respondJSON(w, http.StatusOK, job)
}

func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	job, err := s.store.GetJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "job not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get job")
		return
	}

	job.Enabled = false
	if err := s.store.UpdateJob(r.Context(), job); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete job")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}
