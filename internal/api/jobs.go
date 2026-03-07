package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/robfig/cron/v3"
)

type CreateJobRequest struct {
	ProjectID           string            `json:"project_id" validate:"required"`
	GroupID             string            `json:"group_id,omitempty"`
	Name                string            `json:"name" validate:"required"`
	Slug                string            `json:"slug" validate:"required"`
	Description         string            `json:"description,omitempty"`
	Cron                string            `json:"cron,omitempty"`
	PayloadSchema       json.RawMessage   `json:"payload_schema,omitempty"`
	Tags                map[string]string `json:"tags,omitempty"`
	EndpointURL         string            `json:"endpoint_url" validate:"required,url"`
	FallbackEndpointURL string            `json:"fallback_endpoint_url,omitempty" validate:"omitempty,url"`
	MaxAttempts         int               `json:"max_attempts" validate:"omitempty,min=1"`
	TimeoutSecs         int               `json:"timeout_secs" validate:"omitempty,min=1"`
	MaxConcurrency      int               `json:"max_concurrency,omitempty" validate:"omitempty,min=0"`
	ExecutionWindowCron string            `json:"execution_window_cron,omitempty"`
	Timezone            string            `json:"timezone,omitempty"`
	RateLimitMax        int               `json:"rate_limit_max,omitempty" validate:"omitempty,min=0"`
	RateLimitWindowSecs int               `json:"rate_limit_window_secs,omitempty" validate:"omitempty,min=0"`
	DedupWindowSecs     int               `json:"dedup_window_secs,omitempty" validate:"omitempty,min=0"`
	RunTTLSecs          int               `json:"run_ttl_secs,omitempty" validate:"omitempty,min=0"`
	RetryStrategy       string            `json:"retry_strategy,omitempty" validate:"omitempty,oneof=exponential linear fixed custom"`
	RetryDelaysSecs     []int             `json:"retry_delays_secs,omitempty"`
	EnvironmentID       string            `json:"environment_id,omitempty"`
}

type UpdateJobRequest struct {
	Name                *string            `json:"name,omitempty"`
	Slug                *string            `json:"slug,omitempty"`
	GroupID             *string            `json:"group_id,omitempty"`
	Description         *string            `json:"description,omitempty"`
	Cron                *string            `json:"cron,omitempty"`
	PayloadSchema       *json.RawMessage   `json:"payload_schema,omitempty"`
	Tags                *map[string]string `json:"tags,omitempty"`
	EndpointURL         *string            `json:"endpoint_url,omitempty" validate:"omitempty,url"`
	FallbackEndpointURL *string            `json:"fallback_endpoint_url,omitempty" validate:"omitempty,url"`
	MaxAttempts         *int               `json:"max_attempts,omitempty" validate:"omitempty,min=1"`
	TimeoutSecs         *int               `json:"timeout_secs,omitempty" validate:"omitempty,min=1"`
	MaxConcurrency      *int               `json:"max_concurrency,omitempty" validate:"omitempty,min=0"`
	ExecutionWindowCron *string            `json:"execution_window_cron,omitempty"`
	Timezone            *string            `json:"timezone,omitempty"`
	RateLimitMax        *int               `json:"rate_limit_max,omitempty" validate:"omitempty,min=0"`
	RateLimitWindowSecs *int               `json:"rate_limit_window_secs,omitempty" validate:"omitempty,min=0"`
	DedupWindowSecs     *int               `json:"dedup_window_secs,omitempty" validate:"omitempty,min=0"`
	RunTTLSecs          *int               `json:"run_ttl_secs,omitempty" validate:"omitempty,min=0"`
	RetryStrategy       *string            `json:"retry_strategy,omitempty" validate:"omitempty,oneof=exponential linear fixed custom"`
	RetryDelaysSecs     *[]int             `json:"retry_delays_secs,omitempty"`
	EnvironmentID       *string            `json:"environment_id,omitempty"`
	Enabled             *bool              `json:"enabled,omitempty"`
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req CreateJobRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	if err := validateURL(req.EndpointURL); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid endpoint_url: "+err.Error())
		return
	}
	if req.FallbackEndpointURL != "" {
		if err := validateURL(req.FallbackEndpointURL); err != nil {
			respondError(w, r, http.StatusBadRequest, "invalid fallback_endpoint_url: "+err.Error())
			return
		}
	}

	if req.MaxAttempts == 0 {
		req.MaxAttempts = s.config.DefaultJobMaxAttempts
		if req.MaxAttempts == 0 {
			req.MaxAttempts = 3
		}
	}
	if req.TimeoutSecs == 0 {
		req.TimeoutSecs = s.config.DefaultJobTimeoutSecs
		if req.TimeoutSecs == 0 {
			req.TimeoutSecs = 300
		}
	}

	if req.Cron != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(req.Cron); err != nil {
			respondError(w, r, http.StatusBadRequest, "invalid cron expression")
			return
		}
	}

	if req.ExecutionWindowCron != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(req.ExecutionWindowCron); err != nil {
			respondError(w, r, http.StatusBadRequest, "invalid execution_window_cron expression")
			return
		}
	}

	if err := validateRetryConfig(req.RetryStrategy, req.RetryDelaysSecs); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	if len(req.Tags) > 0 {
		if !s.config.FFJobTags {
			respondError(w, r, http.StatusNotFound, "job tags feature is not enabled")
			return
		}
		if err := validateTags(req.Tags); err != nil {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
	}

	job := &domain.Job{
		ProjectID:           req.ProjectID,
		GroupID:             req.GroupID,
		Name:                req.Name,
		Slug:                req.Slug,
		Description:         req.Description,
		Cron:                req.Cron,
		PayloadSchema:       req.PayloadSchema,
		Tags:                req.Tags,
		EndpointURL:         req.EndpointURL,
		FallbackEndpointURL: req.FallbackEndpointURL,
		MaxAttempts:         req.MaxAttempts,
		TimeoutSecs:         req.TimeoutSecs,
		MaxConcurrency:      req.MaxConcurrency,
		ExecutionWindowCron: req.ExecutionWindowCron,
		Timezone:            req.Timezone,
		RateLimitMax:        req.RateLimitMax,
		RateLimitWindowSecs: req.RateLimitWindowSecs,
		DedupWindowSecs:     req.DedupWindowSecs,
		RunTTLSecs:          req.RunTTLSecs,
		RetryStrategy:       req.RetryStrategy,
		RetryDelaysSecs:     req.RetryDelaysSecs,
		EnvironmentID:       req.EnvironmentID,
		Enabled:             true,
	}

	if err := s.store.CreateJob(r.Context(), job); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create job")
		return
	}

	respondJSON(w, http.StatusCreated, job)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	job, err := s.store.GetJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			respondError(w, r, http.StatusNotFound, "job not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get job")
		return
	}

	respondJSON(w, http.StatusOK, job)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	projectID := query.Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}
	tagKey := query.Get("tag_key")
	tagValue := query.Get("tag_value")
	if tagValue != "" && tagKey == "" {
		respondError(w, r, http.StatusBadRequest, "tag_key is required when tag_value is provided")
		return
	}

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	var (
		jobs    []domain.Job
		listErr error
	)
	if tagKey != "" {
		if !s.config.FFJobTags {
			respondError(w, r, http.StatusNotFound, "job tags feature is not enabled")
			return
		}
		jobs, listErr = s.store.ListJobsByTag(r.Context(), projectID, tagKey, tagValue, limit+1, cursor)
	} else {
		jobs, listErr = s.store.ListJobs(r.Context(), projectID, limit+1, cursor)
	}
	if listErr != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list jobs")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(jobs, limit, func(j domain.Job) string {
		return j.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleUpdateJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	job, err := s.store.GetJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			respondError(w, r, http.StatusNotFound, "job not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get job")
		return
	}

	var req UpdateJobRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	if req.Cron != nil && *req.Cron != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(*req.Cron); err != nil {
			respondError(w, r, http.StatusBadRequest, "invalid cron expression")
			return
		}
	}

	if req.ExecutionWindowCron != nil && *req.ExecutionWindowCron != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(*req.ExecutionWindowCron); err != nil {
			respondError(w, r, http.StatusBadRequest, "invalid execution_window_cron expression")
			return
		}
	}

	if req.RetryStrategy != nil {
		if err := validateRetryConfig(*req.RetryStrategy, nil); err != nil {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.RetryDelaysSecs != nil {
		if err := validateRetryConfig("", *req.RetryDelaysSecs); err != nil {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
	}

	if req.Name != nil {
		job.Name = *req.Name
	}
	if req.Slug != nil {
		job.Slug = *req.Slug
	}
	if req.GroupID != nil {
		job.GroupID = *req.GroupID
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
	if req.Tags != nil {
		if !s.config.FFJobTags {
			respondError(w, r, http.StatusNotFound, "job tags feature is not enabled")
			return
		}
		if err := validateTags(*req.Tags); err != nil {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		job.Tags = *req.Tags
	}
	if req.EndpointURL != nil {
		if err := validateURL(*req.EndpointURL); err != nil {
			respondError(w, r, http.StatusBadRequest, "invalid endpoint_url: "+err.Error())
			return
		}
		job.EndpointURL = *req.EndpointURL
	}
	if req.FallbackEndpointURL != nil {
		job.FallbackEndpointURL = *req.FallbackEndpointURL
	}
	if req.MaxAttempts != nil {
		job.MaxAttempts = *req.MaxAttempts
	}
	if req.TimeoutSecs != nil {
		job.TimeoutSecs = *req.TimeoutSecs
	}
	if req.MaxConcurrency != nil {
		job.MaxConcurrency = *req.MaxConcurrency
	}
	if req.ExecutionWindowCron != nil {
		job.ExecutionWindowCron = *req.ExecutionWindowCron
	}
	if req.Timezone != nil {
		job.Timezone = *req.Timezone
	}
	if req.RateLimitMax != nil {
		job.RateLimitMax = *req.RateLimitMax
	}
	if req.RateLimitWindowSecs != nil {
		job.RateLimitWindowSecs = *req.RateLimitWindowSecs
	}
	if req.DedupWindowSecs != nil {
		job.DedupWindowSecs = *req.DedupWindowSecs
	}
	if req.RunTTLSecs != nil {
		job.RunTTLSecs = *req.RunTTLSecs
	}
	if req.RetryStrategy != nil {
		job.RetryStrategy = *req.RetryStrategy
	}
	if req.RetryDelaysSecs != nil {
		job.RetryDelaysSecs = *req.RetryDelaysSecs
	}
	if req.EnvironmentID != nil {
		job.EnvironmentID = *req.EnvironmentID
	}
	if req.Enabled != nil {
		job.Enabled = *req.Enabled
	}

	if job.FallbackEndpointURL != "" {
		if err := validateURL(job.FallbackEndpointURL); err != nil {
			respondError(w, r, http.StatusBadRequest, "invalid fallback_endpoint_url: "+err.Error())
			return
		}
	}

	if err := s.store.UpdateJob(r.Context(), job); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to update job")
		return
	}

	respondJSON(w, http.StatusOK, job)
}

func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	job, err := s.store.GetJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			respondError(w, r, http.StatusNotFound, "job not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get job")
		return
	}

	job.Enabled = false
	if err := s.store.UpdateJob(r.Context(), job); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to delete job")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}

type CloneJobRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (s *Server) handleCloneJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	source, err := s.store.GetJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			respondError(w, r, http.StatusNotFound, "job not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get job")
		return
	}

	var req CloneJobRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.Slug == "" {
		respondError(w, r, http.StatusBadRequest, "name and slug are required")
		return
	}

	clone := &domain.Job{
		ProjectID:           source.ProjectID,
		GroupID:             source.GroupID,
		Name:                req.Name,
		Slug:                req.Slug,
		Description:         source.Description,
		Cron:                source.Cron,
		PayloadSchema:       source.PayloadSchema,
		Tags:                source.Tags,
		EndpointURL:         source.EndpointURL,
		FallbackEndpointURL: source.FallbackEndpointURL,
		MaxAttempts:         source.MaxAttempts,
		TimeoutSecs:         source.TimeoutSecs,
		MaxConcurrency:      source.MaxConcurrency,
		ExecutionWindowCron: source.ExecutionWindowCron,
		Timezone:            source.Timezone,
		RateLimitMax:        source.RateLimitMax,
		RateLimitWindowSecs: source.RateLimitWindowSecs,
		DedupWindowSecs:     source.DedupWindowSecs,
		WebhookURL:          source.WebhookURL,
		WebhookSecret:       source.WebhookSecret,
		RunTTLSecs:          source.RunTTLSecs,
		RetryStrategy:       source.RetryStrategy,
		RetryDelaysSecs:     source.RetryDelaysSecs,
		EnvironmentID:       source.EnvironmentID,
		Enabled:             true,
	}

	if err := s.store.CreateJob(r.Context(), clone); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to clone job")
		return
	}

	respondJSON(w, http.StatusCreated, clone)
}

func validateTags(tags map[string]string) error {
	if len(tags) > 20 {
		return fmt.Errorf("too many tags (max 20)")
	}
	for key, value := range tags {
		if key == "" {
			return fmt.Errorf("tag keys must be non-empty")
		}
		if len(key) > 64 {
			return fmt.Errorf("tag key too long (max 64 characters)")
		}
		if len(value) > 256 {
			return fmt.Errorf("tag value too long (max 256 characters)")
		}
	}
	return nil
}

// validateRetryConfig validates retry_strategy and retry_delays_secs values.
func validateRetryConfig(strategy string, delays []int) error {
	if strategy != "" {
		switch strategy {
		case "exponential", "linear", "fixed", "custom":
			// valid
		default:
			return fmt.Errorf("invalid retry_strategy: must be exponential, linear, fixed, or custom")
		}
	}
	for _, d := range delays {
		if d <= 0 {
			return fmt.Errorf("retry_delays_secs values must be positive")
		}
	}
	return nil
}

// Batch job definition operations (2.38).

const maxBatchSize = 50

type BatchCreateJobsRequest struct {
	Jobs []CreateJobRequest `json:"jobs"`
}

type BatchCreateJobsResponse struct {
	Created []domain.Job `json:"created"`
	Errors  []BatchError `json:"errors,omitempty"`
}

type BatchError struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

type BatchJobIDsRequest struct {
	IDs []string `json:"ids"`
}

type BatchUpdateResult struct {
	Updated int64 `json:"updated"`
}

func (s *Server) handleBatchCreateJobs(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFBatchJobOps {
		respondError(w, r, http.StatusNotFound, "batch job operations feature is not enabled")
		return
	}

	var req BatchCreateJobsRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Jobs) == 0 {
		respondError(w, r, http.StatusBadRequest, "jobs array is required and must not be empty")
		return
	}
	if len(req.Jobs) > maxBatchSize {
		respondError(w, r, http.StatusBadRequest, fmt.Sprintf("too many jobs in batch (max %d)", maxBatchSize))
		return
	}

	var resp BatchCreateJobsResponse
	for i, jobReq := range req.Jobs {
		if jobReq.ProjectID == "" || jobReq.Name == "" || jobReq.Slug == "" || jobReq.EndpointURL == "" {
			resp.Errors = append(resp.Errors, BatchError{Index: i, Message: "missing required fields"})
			continue
		}

		if err := validateURL(jobReq.EndpointURL); err != nil {
			resp.Errors = append(resp.Errors, BatchError{Index: i, Message: "invalid endpoint_url: " + err.Error()})
			continue
		}

		if err := validateRetryConfig(jobReq.RetryStrategy, jobReq.RetryDelaysSecs); err != nil {
			resp.Errors = append(resp.Errors, BatchError{Index: i, Message: err.Error()})
			continue
		}

		if jobReq.MaxAttempts == 0 {
			jobReq.MaxAttempts = 3
		}
		if jobReq.TimeoutSecs == 0 {
			jobReq.TimeoutSecs = 300
		}

		if len(jobReq.Tags) > 0 && !s.config.FFJobTags {
			resp.Errors = append(resp.Errors, BatchError{Index: i, Message: "job tags feature is not enabled"})
			continue
		}
		if len(jobReq.Tags) > 0 {
			if err := validateTags(jobReq.Tags); err != nil {
				resp.Errors = append(resp.Errors, BatchError{Index: i, Message: err.Error()})
				continue
			}
		}

		job := &domain.Job{
			ProjectID:           jobReq.ProjectID,
			GroupID:             jobReq.GroupID,
			Name:                jobReq.Name,
			Slug:                jobReq.Slug,
			Description:         jobReq.Description,
			Cron:                jobReq.Cron,
			PayloadSchema:       jobReq.PayloadSchema,
			Tags:                jobReq.Tags,
			EndpointURL:         jobReq.EndpointURL,
			FallbackEndpointURL: jobReq.FallbackEndpointURL,
			MaxAttempts:         jobReq.MaxAttempts,
			TimeoutSecs:         jobReq.TimeoutSecs,
			MaxConcurrency:      jobReq.MaxConcurrency,
			ExecutionWindowCron: jobReq.ExecutionWindowCron,
			Timezone:            jobReq.Timezone,
			RateLimitMax:        jobReq.RateLimitMax,
			RateLimitWindowSecs: jobReq.RateLimitWindowSecs,
			DedupWindowSecs:     jobReq.DedupWindowSecs,
			RunTTLSecs:          jobReq.RunTTLSecs,
			RetryStrategy:       jobReq.RetryStrategy,
			RetryDelaysSecs:     jobReq.RetryDelaysSecs,
			EnvironmentID:       jobReq.EnvironmentID,
			Enabled:             true,
		}

		if err := s.store.CreateJob(r.Context(), job); err != nil {
			resp.Errors = append(resp.Errors, BatchError{Index: i, Message: "failed to create job"})
			continue
		}

		resp.Created = append(resp.Created, *job)
	}

	if len(resp.Created) == 0 && len(resp.Errors) > 0 {
		respondJSON(w, http.StatusBadRequest, resp)
		return
	}

	respondJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleBatchEnableJobs(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFBatchJobOps {
		respondError(w, r, http.StatusNotFound, "batch job operations feature is not enabled")
		return
	}

	var req BatchJobIDsRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.IDs) == 0 {
		respondError(w, r, http.StatusBadRequest, "ids array is required and must not be empty")
		return
	}
	if len(req.IDs) > maxBatchSize {
		respondError(w, r, http.StatusBadRequest, fmt.Sprintf("too many ids in batch (max %d)", maxBatchSize))
		return
	}

	updated, err := s.store.BatchUpdateJobsEnabled(r.Context(), req.IDs, true)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to enable jobs")
		return
	}

	respondJSON(w, http.StatusOK, BatchUpdateResult{Updated: updated})
}

func (s *Server) handleBatchDisableJobs(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFBatchJobOps {
		respondError(w, r, http.StatusNotFound, "batch job operations feature is not enabled")
		return
	}

	var req BatchJobIDsRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.IDs) == 0 {
		respondError(w, r, http.StatusBadRequest, "ids array is required and must not be empty")
		return
	}
	if len(req.IDs) > maxBatchSize {
		respondError(w, r, http.StatusBadRequest, fmt.Sprintf("too many ids in batch (max %d)", maxBatchSize))
		return
	}

	updated, err := s.store.BatchUpdateJobsEnabled(r.Context(), req.IDs, false)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to disable jobs")
		return
	}

	respondJSON(w, http.StatusOK, BatchUpdateResult{Updated: updated})
}

// JobHealthResponse wraps health stats with the time window.
type JobHealthResponse struct {
	JobID  string    `json:"job_id"`
	Window string    `json:"window"`
	Since  time.Time `json:"since"`
	*store.JobHealthStats
}

func (s *Server) handleGetJobHealth(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFJobHealthScoring {
		respondError(w, r, http.StatusNotFound, "job health scoring feature is not enabled")
		return
	}

	jobID := chi.URLParam(r, "jobID")

	_, err := s.store.GetJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			respondError(w, r, http.StatusNotFound, "job not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get job")
		return
	}

	window := r.URL.Query().Get("window")
	var since time.Time
	switch window {
	case "1h":
		since = time.Now().Add(-time.Hour)
	case "1d":
		since = time.Now().Add(-24 * time.Hour)
	case "30d":
		since = time.Now().Add(-30 * 24 * time.Hour)
	case "7d", "":
		window = "7d"
		since = time.Now().Add(-7 * 24 * time.Hour)
	default:
		respondError(w, r, http.StatusBadRequest, "invalid window: must be 1h, 1d, 7d, or 30d")
		return
	}

	stats, err := s.store.GetJobHealthStats(r.Context(), jobID, since)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to compute health stats")
		return
	}

	respondJSON(w, http.StatusOK, JobHealthResponse{
		JobID:          jobID,
		Window:         window,
		Since:          since,
		JobHealthStats: stats,
	})
}
