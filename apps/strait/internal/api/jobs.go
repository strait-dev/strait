package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/robfig/cron/v3"

	"strait/internal/billing"
	"strait/internal/clickhouse"
	"strait/internal/domain"
	"strait/internal/store"
)

type CreateJobRequest struct {
	ProjectID                 string            `json:"project_id" validate:"required"`
	GroupID                   string            `json:"group_id,omitempty"`
	Name                      string            `json:"name" validate:"required,max=255"`
	Slug                      string            `json:"slug" validate:"required,max=255"`
	Description               string            `json:"description,omitempty" validate:"max=2000"`
	Cron                      string            `json:"cron,omitempty"`
	PayloadSchema             json.RawMessage   `json:"payload_schema,omitempty"`
	Tags                      map[string]string `json:"tags,omitempty"`
	EndpointURL               string            `json:"endpoint_url" validate:"omitempty,url"`
	EndpointSigningSecret     string            `json:"endpoint_signing_secret,omitempty" validate:"omitempty,min=16,max=4096"`
	WebhookSecret             string            `json:"webhook_secret,omitempty" validate:"omitempty,min=16,max=4096" doc:"Alias of endpoint_signing_secret used by the Go SDK. When both are set, webhook_secret wins and a warning is logged."`
	FallbackEndpointURL       string            `json:"fallback_endpoint_url,omitempty" validate:"omitempty,url"`
	MaxAttempts               int               `json:"max_attempts" validate:"omitempty,min=1,max=100"`
	TimeoutSecs               int               `json:"timeout_secs" validate:"omitempty,min=1"`
	MaxConcurrency            int               `json:"max_concurrency,omitempty" validate:"omitempty,min=0"`
	MaxConcurrencyPerKey      int               `json:"max_concurrency_per_key,omitempty" validate:"omitempty,min=0"`
	ExecutionWindowCron       string            `json:"execution_window_cron,omitempty"`
	Timezone                  string            `json:"timezone,omitempty"`
	RateLimitMax              int               `json:"rate_limit_max,omitempty" validate:"omitempty,min=0"`
	RateLimitWindowSecs       int               `json:"rate_limit_window_secs,omitempty" validate:"omitempty,min=0"`
	DedupWindowSecs           int               `json:"dedup_window_secs,omitempty" validate:"omitempty,min=0"`
	RunTTLSecs                int               `json:"run_ttl_secs,omitempty" validate:"omitempty,min=0"`
	RetryStrategy             string            `json:"retry_strategy,omitempty" validate:"omitempty,oneof=exponential linear fixed custom"`
	RetryDelaysSecs           []int             `json:"retry_delays_secs,omitempty"`
	RetryPriorityBoost        int               `json:"retry_priority_boost,omitempty" validate:"omitempty,min=0,max=10"`
	EnvironmentID             string            `json:"environment_id,omitempty"`
	VersionPolicy             string            `json:"version_policy,omitempty" validate:"omitempty,oneof=pin latest minor"`
	DefaultRunMetadata        map[string]string `json:"default_run_metadata,omitempty"`
	ResultSchema              json.RawMessage   `json:"result_schema,omitempty"`
	CronOverlapPolicy         string            `json:"cron_overlap_policy,omitempty" validate:"omitempty,oneof=allow skip cancel_running"`
	DebounceWindowSecs        int               `json:"debounce_window_secs,omitempty" validate:"omitempty,min=0"`
	BatchWindowSecs           int               `json:"batch_window_secs,omitempty" validate:"omitempty,min=0"`
	BatchMaxSize              int               `json:"batch_max_size,omitempty" validate:"omitempty,min=0"`
	ExecutionMode             string            `json:"execution_mode,omitempty" validate:"omitempty,oneof=http worker"`
	Enabled                   *bool             `json:"enabled,omitempty"`
	QueueName                 string            `json:"queue_name,omitempty"`
	PoisonPillThreshold       *int              `json:"poison_pill_threshold,omitempty" validate:"omitempty,min=1" doc:"Consecutive identical errors before auto-quarantine to DLQ. NULL or 0 disables."`
	OnCompleteTriggerWorkflow string            `json:"on_complete_trigger_workflow,omitempty"`
	OnCompleteTriggerJob      string            `json:"on_complete_trigger_job,omitempty"`
	OnCompletePayloadMapping  json.RawMessage   `json:"on_complete_payload_mapping,omitempty"`
	OnFailureTriggerJob       string            `json:"on_failure_trigger_job,omitempty"`
	OnFailureTriggerWorkflow  string            `json:"on_failure_trigger_workflow,omitempty"`
	OnFailurePayloadMapping   json.RawMessage   `json:"on_failure_payload_mapping,omitempty"`
}

const defaultJobQueueName = "default"

type UpdateJobRequest struct {
	Name                      *string            `json:"name,omitempty"`
	Slug                      *string            `json:"slug,omitempty"`
	GroupID                   *string            `json:"group_id,omitempty"`
	Description               *string            `json:"description,omitempty"`
	Cron                      *string            `json:"cron,omitempty"`
	PayloadSchema             *json.RawMessage   `json:"payload_schema,omitempty"`
	Tags                      *map[string]string `json:"tags,omitempty"`
	EndpointURL               *string            `json:"endpoint_url,omitempty" validate:"omitempty,url"`
	EndpointSigningSecret     *string            `json:"endpoint_signing_secret,omitempty" validate:"omitempty,min=16,max=4096"`
	WebhookSecret             *string            `json:"webhook_secret,omitempty" validate:"omitempty,min=16,max=4096" doc:"Alias of endpoint_signing_secret used by the Go SDK. When both are set, webhook_secret wins and a warning is logged."`
	FallbackEndpointURL       *string            `json:"fallback_endpoint_url,omitempty" validate:"omitempty,url"`
	MaxAttempts               *int               `json:"max_attempts,omitempty" validate:"omitempty,min=1,max=100"`
	TimeoutSecs               *int               `json:"timeout_secs,omitempty" validate:"omitempty,min=1"`
	MaxConcurrency            *int               `json:"max_concurrency,omitempty" validate:"omitempty,min=0"`
	MaxConcurrencyPerKey      *int               `json:"max_concurrency_per_key,omitempty" validate:"omitempty,min=0"`
	ExecutionWindowCron       *string            `json:"execution_window_cron,omitempty"`
	Timezone                  *string            `json:"timezone,omitempty"`
	RateLimitMax              *int               `json:"rate_limit_max,omitempty" validate:"omitempty,min=0"`
	RateLimitWindowSecs       *int               `json:"rate_limit_window_secs,omitempty" validate:"omitempty,min=0"`
	DedupWindowSecs           *int               `json:"dedup_window_secs,omitempty" validate:"omitempty,min=0"`
	RunTTLSecs                *int               `json:"run_ttl_secs,omitempty" validate:"omitempty,min=0"`
	RetryStrategy             *string            `json:"retry_strategy,omitempty" validate:"omitempty,oneof=exponential linear fixed custom"`
	RetryDelaysSecs           *[]int             `json:"retry_delays_secs,omitempty"`
	RetryPriorityBoost        *int               `json:"retry_priority_boost,omitempty" validate:"omitempty,min=0,max=10"`
	EnvironmentID             *string            `json:"environment_id,omitempty"`
	Enabled                   *bool              `json:"enabled,omitempty"`
	VersionPolicy             *string            `json:"version_policy,omitempty" validate:"omitempty,oneof=pin latest minor"`
	BackwardsCompatible       *bool              `json:"backwards_compatible,omitempty"`
	DefaultRunMetadata        *map[string]string `json:"default_run_metadata,omitempty"`
	ResultSchema              *json.RawMessage   `json:"result_schema,omitempty"`
	CronOverlapPolicy         *string            `json:"cron_overlap_policy,omitempty" validate:"omitempty,oneof=allow skip cancel_running"`
	DebounceWindowSecs        *int               `json:"debounce_window_secs,omitempty" validate:"omitempty,min=0"`
	BatchWindowSecs           *int               `json:"batch_window_secs,omitempty" validate:"omitempty,min=0"`
	BatchMaxSize              *int               `json:"batch_max_size,omitempty" validate:"omitempty,min=0"`
	ExecutionMode             *string            `json:"execution_mode,omitempty" validate:"omitempty,oneof=http worker"`
	QueueName                 *string            `json:"queue_name,omitempty"`
	PoisonPillThreshold       *int               `json:"poison_pill_threshold,omitempty" validate:"omitempty,min=1" doc:"Consecutive identical errors before auto-quarantine to DLQ. NULL or 0 disables."`
	OnCompleteTriggerWorkflow *string            `json:"on_complete_trigger_workflow,omitempty"`
	OnCompleteTriggerJob      *string            `json:"on_complete_trigger_job,omitempty"`
	OnCompletePayloadMapping  *json.RawMessage   `json:"on_complete_payload_mapping,omitempty"`
	OnFailureTriggerJob       *string            `json:"on_failure_trigger_job,omitempty"`
	OnFailureTriggerWorkflow  *string            `json:"on_failure_trigger_workflow,omitempty"`
	OnFailurePayloadMapping   *json.RawMessage   `json:"on_failure_payload_mapping,omitempty"`
}

// CreateJobInput is the typed input for creating a job.
type CreateJobInput struct {
	Body CreateJobRequest
}

// CreateJobOutput is the typed output for creating a job.
type CreateJobOutput struct {
	Body *domain.Job
}

func (s *Server) handleCreateJob(ctx context.Context, input *CreateJobInput) (*CreateJobOutput, error) {
	req := input.Body

	if err := s.validateCreateJobFields(ctx, &req); err != nil {
		return nil, err
	}

	execMode, err := s.resolveAndCheckExecMode(ctx, &req)
	if err != nil {
		return nil, err
	}

	if err := s.checkJobChainingAllowed(ctx, req.ProjectID, req.OnCompleteTriggerJob, req.OnCompleteTriggerWorkflow); err != nil {
		return nil, err
	}
	if err := s.checkJobChainingAllowed(ctx, req.ProjectID, req.OnFailureTriggerJob, req.OnFailureTriggerWorkflow); err != nil {
		return nil, err
	}
	if err := s.checkCronOverlapPolicy(ctx, req.ProjectID, req.CronOverlapPolicy); err != nil {
		return nil, err
	}
	if err := s.checkCronMinInterval(ctx, req.ProjectID, req.Cron); err != nil {
		return nil, err
	}
	if err := s.checkRunTTLLimit(ctx, req.ProjectID, req.RunTTLSecs); err != nil {
		return nil, err
	}
	if err := s.checkPerJobConcurrencyLimit(ctx, req.ProjectID, req.MaxConcurrency, req.MaxConcurrencyPerKey); err != nil {
		return nil, err
	}

	signingSecret, err := s.resolveCreateJobSigningSecret(req)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to encrypt endpoint signing secret")
	}

	job := &domain.Job{
		ProjectID:                 req.ProjectID,
		GroupID:                   req.GroupID,
		Name:                      req.Name,
		Slug:                      req.Slug,
		Description:               req.Description,
		Cron:                      req.Cron,
		PayloadSchema:             req.PayloadSchema,
		Tags:                      req.Tags,
		EndpointURL:               req.EndpointURL,
		EndpointSigningSecret:     signingSecret,
		FallbackEndpointURL:       req.FallbackEndpointURL,
		MaxAttempts:               req.MaxAttempts,
		TimeoutSecs:               req.TimeoutSecs,
		MaxConcurrency:            req.MaxConcurrency,
		MaxConcurrencyPerKey:      req.MaxConcurrencyPerKey,
		ExecutionWindowCron:       req.ExecutionWindowCron,
		Timezone:                  req.Timezone,
		RateLimitMax:              req.RateLimitMax,
		RateLimitWindowSecs:       req.RateLimitWindowSecs,
		DedupWindowSecs:           req.DedupWindowSecs,
		RunTTLSecs:                req.RunTTLSecs,
		RetryStrategy:             req.RetryStrategy,
		RetryDelaysSecs:           req.RetryDelaysSecs,
		RetryPriorityBoost:        req.RetryPriorityBoost,
		EnvironmentID:             req.EnvironmentID,
		DefaultRunMetadata:        req.DefaultRunMetadata,
		ResultSchema:              req.ResultSchema,
		CronOverlapPolicy:         domain.CronOverlapPolicy(req.CronOverlapPolicy),
		DebounceWindowSecs:        req.DebounceWindowSecs,
		BatchWindowSecs:           req.BatchWindowSecs,
		BatchMaxSize:              req.BatchMaxSize,
		ExecutionMode:             execMode,
		Queue:                     normalizeJobQueueName(req.QueueName),
		PoisonPillThreshold:       req.PoisonPillThreshold,
		OnCompleteTriggerWorkflow: req.OnCompleteTriggerWorkflow,
		OnCompleteTriggerJob:      req.OnCompleteTriggerJob,
		OnCompletePayloadMapping:  req.OnCompletePayloadMapping,
		OnFailureTriggerJob:       req.OnFailureTriggerJob,
		OnFailureTriggerWorkflow:  req.OnFailureTriggerWorkflow,
		OnFailurePayloadMapping:   req.OnFailurePayloadMapping,
		Enabled:                   true,
		VersionPolicy:             domain.VersionPolicyPin,
		CreatedBy:                 actorFromContext(ctx),
		UpdatedBy:                 actorFromContext(ctx),
	}
	if req.Enabled != nil {
		job.Enabled = *req.Enabled
	}

	if req.VersionPolicy != "" {
		job.VersionPolicy = domain.VersionPolicy(req.VersionPolicy)
	}

	if err := s.createJobWithCronScheduleLimit(ctx, job); err != nil {
		if errors.Is(err, store.ErrJobSlugConflict) {
			return nil, huma.Error409Conflict(err.Error())
		}
		var statusErr huma.StatusError
		if errors.As(err, &statusErr) {
			return nil, err
		}
		return nil, huma.Error500InternalServerError("failed to create job")
	}
	s.invalidateWorkerJobCache(ctx, job.ID, job.CacheVersion)

	s.enqueueJobMetadata(job)

	s.emitAuditEvent(ctx, domain.AuditActionJobCreated, "job", job.ID, map[string]any{
		"name":           job.Name,
		"slug":           job.Slug,
		"cron":           job.Cron,
		"execution_mode": string(job.ExecutionMode),
		"group_id":       job.GroupID,
		"environment_id": job.EnvironmentID,
		"enabled":        job.Enabled,
	})

	return &CreateJobOutput{Body: job}, nil
}

func (s *Server) createJobWithCronScheduleLimit(ctx context.Context, job *domain.Job) error {
	orgID, maxSchedules, displayName, err := s.resolveScheduleCreateLimit(ctx, job.ProjectID, job.Cron)
	if err != nil {
		return err
	}

	if creator, ok := s.store.(jobCronScheduleLimitCreator); ok {
		err = creator.CreateJobWithCronScheduleLimit(ctx, job, orgID, maxSchedules)
	} else {
		if err := s.checkScheduleLimit(ctx, job.ProjectID, job.Cron); err != nil {
			return err
		}
		err = s.store.CreateJob(ctx, job)
	}
	if errors.Is(err, store.ErrCronScheduleLimitExceeded) {
		s.dispatchWorkflowRegistrationRejected(ctx, job.ProjectID, "schedule_limit", maxSchedules, maxSchedules)
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d scheduled jobs. Upgrade at /settings/billing", displayName, maxSchedules),
		)
	}
	return err
}

func (s *Server) resolveCreateJobSigningSecret(req CreateJobRequest) (string, error) {
	signingSecret := req.EndpointSigningSecret
	if req.WebhookSecret != "" {
		if signingSecret != "" && signingSecret != req.WebhookSecret {
			slog.Warn("both webhook_secret and endpoint_signing_secret supplied on job create; using webhook_secret",
				"project_id", req.ProjectID, "slug", req.Slug)
		}
		signingSecret = req.WebhookSecret
	}
	return s.encryptEndpointSigningSecret(signingSecret)
}

func sanitizedJobUpdateAuditChanges(req UpdateJobRequest) map[string]any {
	secretChanged := req.EndpointSigningSecret != nil || req.WebhookSecret != nil
	req.EndpointSigningSecret = nil
	req.WebhookSecret = nil

	raw, err := json.Marshal(req)
	if err != nil {
		return map[string]any{"marshal_error": true, "signing_credential_changed": secretChanged}
	}
	var changes map[string]any
	if err := json.Unmarshal(raw, &changes); err != nil {
		return map[string]any{"marshal_error": true, "signing_credential_changed": secretChanged}
	}
	if changes == nil {
		changes = map[string]any{}
	}
	if secretChanged {
		changes["signing_credential_changed"] = true
	}
	return changes
}

// validateCreateJobFields validates the scalar and cron fields on a CreateJobRequest,
// applies defaults, and checks plan gates that do not depend on execution mode.
// It mutates req to apply defaults (MaxAttempts, TimeoutSecs, RetryPriorityBoost).
func (s *Server) validateCreateJobFields(ctx context.Context, req *CreateJobRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return huma.Error403Forbidden("project_id does not match authenticated project")
	}
	if err := requireEnvironmentMatch(ctx, req.EnvironmentID); err != nil {
		return huma.Error403Forbidden("environment_id does not match authenticated environment")
	}
	if err := validateJobName(req.Name); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if err := validateJobSlug(req.Slug); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if req.EndpointURL != "" {
		if err := s.validateEndpointURL(req.EndpointURL); err != nil {
			return huma.Error400BadRequest("invalid endpoint_url: " + err.Error())
		}
	}
	if req.FallbackEndpointURL != "" {
		if err := s.validateEndpointURL(req.FallbackEndpointURL); err != nil {
			return huma.Error400BadRequest("invalid fallback_endpoint_url: " + err.Error())
		}
	}
	if req.MaxAttempts == 0 {
		req.MaxAttempts = s.defaultJobMaxAttempts()
	}
	if req.TimeoutSecs == 0 {
		req.TimeoutSecs = s.defaultJobTimeoutSecs()
	}
	if req.TimeoutSecs > 86400 {
		return huma.Error400BadRequest("timeout_secs must not exceed 86400 (24 hours)")
	}
	if req.RetryPriorityBoost == 0 {
		req.RetryPriorityBoost = 1
	}
	if err := validateCreateJobCronFields(req); err != nil {
		return err
	}
	if err := validateRetryConfig(req.RetryStrategy, req.RetryDelaysSecs); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if len(req.Tags) > 0 {
		if err := validateTags(req.Tags); err != nil {
			return huma.Error400BadRequest(err.Error())
		}
	}
	if req.DebounceWindowSecs > 0 && req.BatchWindowSecs > 0 {
		return huma.Error400BadRequest("debounce_window_secs and batch_window_secs are mutually exclusive")
	}
	if err := s.validateWindowsAgainstRetention(req.RateLimitWindowSecs, req.DedupWindowSecs); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if err := validateQueueName(req.QueueName); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	return nil
}

// validateCreateJobCronFields validates the cron and execution_window_cron expressions.
func validateCreateJobCronFields(req *CreateJobRequest) error {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if req.Cron != "" {
		if err := validateCronFieldCount(req.Cron); err != nil {
			return huma.Error400BadRequest(err.Error())
		}
		if _, err := parser.Parse(req.Cron); err != nil {
			return huma.Error400BadRequest("invalid cron expression")
		}
	}
	if req.ExecutionWindowCron != "" {
		if err := validateCronFieldCount(req.ExecutionWindowCron); err != nil {
			return huma.Error400BadRequest(err.Error())
		}
		if _, err := parser.Parse(req.ExecutionWindowCron); err != nil {
			return huma.Error400BadRequest("invalid execution_window_cron expression")
		}
	}
	return nil
}

// resolveAndCheckExecMode determines the execution mode and validates it
// against plan gates. Returns the resolved ExecutionMode.
func (s *Server) resolveAndCheckExecMode(ctx context.Context, req *CreateJobRequest) (domain.ExecutionMode, error) {
	execMode := domain.ExecutionMode(req.ExecutionMode)
	if execMode == "" {
		execMode = domain.ExecutionModeHTTP
	}
	switch execMode {
	case domain.ExecutionModeHTTP:
		if err := validateEndpointNotEmpty(req.EndpointURL); err != nil {
			return "", huma.Error400BadRequest(err.Error())
		}
		if err := s.checkHTTPModeAllowed(ctx, execMode, req.ProjectID); err != nil {
			return "", err
		}
	case domain.ExecutionModeWorker:
		// Worker mode: execution is handled by a connected worker process.
	}
	return execMode, nil
}

// GetJobInput is the typed input for getting a single job.
type GetJobInput struct {
	JobID string `path:"jobID"`
}

// GetJobOutput is the typed output for getting a single job.
type GetJobOutput struct {
	Body *domain.Job
}

func (s *Server) handleGetJob(ctx context.Context, input *GetJobInput) (*GetJobOutput, error) {
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if job == nil {
		return nil, huma.Error404NotFound("job not found")
	}

	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}

	return &GetJobOutput{Body: job}, nil
}

// ListJobsInput is the typed input for listing jobs.
type ListJobsInput struct {
	Slug     string `query:"slug"`
	TagKey   string `query:"tag_key"`
	TagValue string `query:"tag_value"`
	Limit    string `query:"limit"`
	Cursor   string `query:"cursor"`
}

// ListJobsOutput is the typed output for listing jobs.
type ListJobsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListJobs(ctx context.Context, input *ListJobsInput) (*ListJobsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.TagValue != "" && input.TagKey == "" {
		return nil, huma.Error400BadRequest("tag_key is required when tag_value is provided")
	}

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	// Slug lookup: return a single-item list when ?slug= is provided.
	if input.Slug != "" {
		emptyPage := func() *ListJobsOutput {
			return &ListJobsOutput{Body: paginatedResult([]domain.Job{}, limit, func(j domain.Job) string {
				return j.CreatedAt.Format(time.RFC3339Nano)
			})}
		}
		job, jobErr := s.store.GetJobBySlug(ctx, projectID, input.Slug)
		if jobErr != nil {
			if errors.Is(jobErr, store.ErrJobNotFound) {
				return emptyPage(), nil
			}
			return nil, huma.Error500InternalServerError("failed to look up job by slug")
		}
		if callerEnv := environmentIDFromContext(ctx); callerEnv != "" && job.EnvironmentID != callerEnv {
			return emptyPage(), nil
		}
		return &ListJobsOutput{Body: paginatedResult([]domain.Job{*job}, limit, func(j domain.Job) string {
			return j.CreatedAt.Format(time.RFC3339Nano)
		})}, nil
	}

	var (
		jobs    []domain.Job
		listErr error
	)
	if input.TagKey != "" {
		jobs, listErr = s.store.ListJobsByTag(ctx, projectID, input.TagKey, input.TagValue, limit+1, cursor)
	} else {
		jobs, listErr = s.store.ListJobs(ctx, projectID, limit+1, cursor)
	}
	if listErr != nil {
		return nil, huma.Error500InternalServerError("failed to list jobs")
	}
	jobs = filterJobsForEnvironment(ctx, jobs)

	return &ListJobsOutput{Body: paginatedResult(jobs, limit, func(j domain.Job) string {
		return j.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

// UpdateJobInput is the typed input for updating a job.
type UpdateJobInput struct {
	JobID string `path:"jobID"`
	Body  UpdateJobRequest
}

// UpdateJobOutput is the typed output for updating a job.
type UpdateJobOutput struct {
	Body *domain.Job
}

func (s *Server) handleUpdateJob(ctx context.Context, input *UpdateJobInput) (*UpdateJobOutput, error) {
	req := input.Body

	job, err := s.loadMutableJob(ctx, input.JobID)
	if err != nil {
		return nil, err
	}

	if err := s.validateUpdateJobRequest(req, job); err != nil {
		return nil, err
	}

	addsCronSchedule := req.Cron != nil && *req.Cron != "" && job.Cron == ""
	if err := s.applyJobBasicUpdate(ctx, job, req); err != nil {
		return nil, err
	}
	if err := s.applyJobEndpointUpdate(ctx, job, req); err != nil {
		return nil, err
	}
	if err := s.applyJobExecutionUpdate(ctx, job, req); err != nil {
		return nil, err
	}
	if err := s.applyJobMetadataUpdate(ctx, job, req); err != nil {
		return nil, err
	}
	if err := s.applyJobChainingUpdate(ctx, job, req); err != nil {
		return nil, err
	}
	if err := s.finalizeUpdatedJobFields(job); err != nil {
		return nil, err
	}

	if err := s.persistUpdatedJob(ctx, job, req, addsCronSchedule); err != nil {
		return nil, err
	}

	return &UpdateJobOutput{Body: job}, nil
}

func (s *Server) applyJobBasicUpdate(ctx context.Context, job *domain.Job, req UpdateJobRequest) error {
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
		if *req.Cron != "" {
			if err := s.checkCronMinInterval(ctx, job.ProjectID, *req.Cron); err != nil {
				return err
			}
		}
		job.Cron = *req.Cron
	}
	if req.PayloadSchema != nil {
		job.PayloadSchema = *req.PayloadSchema
	}
	if req.Tags != nil {
		if err := validateTags(*req.Tags); err != nil {
			return huma.Error400BadRequest(err.Error())
		}
		job.Tags = *req.Tags
	}
	return nil
}

func (s *Server) applyJobEndpointUpdate(ctx context.Context, job *domain.Job, req UpdateJobRequest) error {
	nextEndpointURL := job.EndpointURL
	nextFallbackEndpointURL := job.FallbackEndpointURL
	if req.EndpointURL != nil {
		nextEndpointURL = *req.EndpointURL
	}
	if req.FallbackEndpointURL != nil {
		nextFallbackEndpointURL = *req.FallbackEndpointURL
	}
	if err := s.requireSecretsWriteForSecretBearingEndpointChange(ctx, job, nextEndpointURL, nextFallbackEndpointURL); err != nil {
		return err
	}
	if req.EndpointURL != nil {
		if err := s.validateEndpointURL(*req.EndpointURL); err != nil {
			return huma.Error400BadRequest("invalid endpoint_url: " + err.Error())
		}
		job.EndpointURL = *req.EndpointURL
	}
	if req.WebhookSecret != nil || req.EndpointSigningSecret != nil {
		var signingSecret string
		switch {
		case req.WebhookSecret != nil && req.EndpointSigningSecret != nil && *req.WebhookSecret != *req.EndpointSigningSecret:
			slog.Warn("both webhook_secret and endpoint_signing_secret supplied on job update; using webhook_secret",
				"job_id", job.ID, "project_id", job.ProjectID)
			signingSecret = *req.WebhookSecret
		case req.WebhookSecret != nil:
			signingSecret = *req.WebhookSecret
		default:
			signingSecret = *req.EndpointSigningSecret
		}
		signingSecret, err := s.encryptEndpointSigningSecret(signingSecret)
		if err != nil {
			return huma.Error500InternalServerError("failed to encrypt endpoint signing secret")
		}
		job.EndpointSigningSecret = signingSecret
	}
	if req.FallbackEndpointURL != nil {
		job.FallbackEndpointURL = *req.FallbackEndpointURL
	}
	return nil
}

func (s *Server) applyJobExecutionUpdate(ctx context.Context, job *domain.Job, req UpdateJobRequest) error {
	if req.MaxAttempts != nil {
		job.MaxAttempts = *req.MaxAttempts
	}
	if req.TimeoutSecs != nil {
		if *req.TimeoutSecs > 86400 {
			return huma.Error400BadRequest("timeout_secs must not exceed 86400 (24 hours)")
		}
		job.TimeoutSecs = *req.TimeoutSecs
	}
	if req.MaxConcurrency != nil || req.MaxConcurrencyPerKey != nil {
		newMax := job.MaxConcurrency
		if req.MaxConcurrency != nil {
			newMax = *req.MaxConcurrency
		}
		newPerKey := job.MaxConcurrencyPerKey
		if req.MaxConcurrencyPerKey != nil {
			newPerKey = *req.MaxConcurrencyPerKey
		}
		if err := s.checkPerJobConcurrencyLimit(ctx, job.ProjectID, newMax, newPerKey); err != nil {
			return err
		}
		if req.MaxConcurrency != nil {
			job.MaxConcurrency = *req.MaxConcurrency
		}
		if req.MaxConcurrencyPerKey != nil {
			job.MaxConcurrencyPerKey = *req.MaxConcurrencyPerKey
		}
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
		if err := s.checkRunTTLLimit(ctx, job.ProjectID, *req.RunTTLSecs); err != nil {
			return err
		}
		job.RunTTLSecs = *req.RunTTLSecs
	}
	if req.RetryStrategy != nil {
		job.RetryStrategy = *req.RetryStrategy
	}
	if req.RetryDelaysSecs != nil {
		job.RetryDelaysSecs = *req.RetryDelaysSecs
	}
	if req.RetryPriorityBoost != nil {
		job.RetryPriorityBoost = *req.RetryPriorityBoost
	}
	if req.EnvironmentID != nil {
		if err := requireEnvironmentMatch(ctx, *req.EnvironmentID); err != nil {
			return huma.Error403Forbidden("environment_id does not match authenticated environment")
		}
		job.EnvironmentID = *req.EnvironmentID
	}
	return nil
}

func (s *Server) applyJobMetadataUpdate(ctx context.Context, job *domain.Job, req UpdateJobRequest) error {
	if req.Enabled != nil {
		job.Enabled = *req.Enabled
	}
	if req.VersionPolicy != nil {
		job.VersionPolicy = domain.VersionPolicy(*req.VersionPolicy)
	}
	if req.BackwardsCompatible != nil {
		job.BackwardsCompatible = *req.BackwardsCompatible
	}
	if req.DefaultRunMetadata != nil {
		job.DefaultRunMetadata = *req.DefaultRunMetadata
	}
	if req.ResultSchema != nil {
		job.ResultSchema = *req.ResultSchema
	}
	if req.CronOverlapPolicy != nil && *req.CronOverlapPolicy != "" {
		if err := s.checkCronOverlapPolicy(ctx, job.ProjectID, *req.CronOverlapPolicy); err != nil {
			return err
		}
		job.CronOverlapPolicy = domain.CronOverlapPolicy(*req.CronOverlapPolicy)
	}
	if req.ExecutionMode != nil {
		mode := domain.ExecutionMode(*req.ExecutionMode)
		if err := s.checkHTTPModeAllowed(ctx, mode, job.ProjectID); err != nil {
			return err
		}
		job.ExecutionMode = mode
	}
	if req.QueueName != nil {
		if err := validateQueueName(*req.QueueName); err != nil {
			return huma.Error400BadRequest(err.Error())
		}
		job.Queue = normalizeJobQueueName(*req.QueueName)
	}
	if req.PoisonPillThreshold != nil {
		job.PoisonPillThreshold = req.PoisonPillThreshold
	}
	return nil
}

func (s *Server) applyJobChainingUpdate(ctx context.Context, job *domain.Job, req UpdateJobRequest) error {
	if req.OnCompleteTriggerWorkflow != nil {
		if err := s.checkJobChainingAllowed(ctx, job.ProjectID, *req.OnCompleteTriggerWorkflow, ""); err != nil {
			return err
		}
		job.OnCompleteTriggerWorkflow = *req.OnCompleteTriggerWorkflow
	}
	if req.OnCompleteTriggerJob != nil {
		if err := s.checkJobChainingAllowed(ctx, job.ProjectID, *req.OnCompleteTriggerJob, ""); err != nil {
			return err
		}
		job.OnCompleteTriggerJob = *req.OnCompleteTriggerJob
	}
	if req.OnCompletePayloadMapping != nil {
		job.OnCompletePayloadMapping = *req.OnCompletePayloadMapping
	}
	if req.OnFailureTriggerJob != nil {
		if err := s.checkJobChainingAllowed(ctx, job.ProjectID, *req.OnFailureTriggerJob, ""); err != nil {
			return err
		}
		job.OnFailureTriggerJob = *req.OnFailureTriggerJob
	}
	if req.OnFailureTriggerWorkflow != nil {
		if err := s.checkJobChainingAllowed(ctx, job.ProjectID, "", *req.OnFailureTriggerWorkflow); err != nil {
			return err
		}
		job.OnFailureTriggerWorkflow = *req.OnFailureTriggerWorkflow
	}
	if req.OnFailurePayloadMapping != nil {
		job.OnFailurePayloadMapping = *req.OnFailurePayloadMapping
	}
	return nil
}

func (s *Server) finalizeUpdatedJobFields(job *domain.Job) error {
	if job.FallbackEndpointURL != "" {
		if err := s.validateEndpointURL(job.FallbackEndpointURL); err != nil {
			return huma.Error400BadRequest("invalid fallback_endpoint_url: " + err.Error())
		}
	}
	if job.ExecutionMode == domain.ExecutionModeHTTP {
		if err := validateEndpointNotEmpty(job.EndpointURL); err != nil {
			return huma.Error400BadRequest(err.Error())
		}
	}
	job.Queue = normalizeJobQueueName(job.Queue)
	return nil
}

func (s *Server) loadMutableJob(ctx context.Context, jobID string) (*domain.Job, error) {
	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	return job, nil
}

// validateUpdateJobRequest checks request-local and cross-field constraints
// before handleUpdateJob mutates the loaded job in place.
func (s *Server) validateUpdateJobRequest(req UpdateJobRequest, current *domain.Job) error {
	if err := s.validate.Struct(&req); err != nil {
		return newValidationError(err)
	}
	if req.Name != nil {
		if err := validateJobName(*req.Name); err != nil {
			return huma.Error400BadRequest(err.Error())
		}
	}
	if req.Slug != nil {
		if err := validateJobSlug(*req.Slug); err != nil {
			return huma.Error400BadRequest(err.Error())
		}
	}
	if req.EndpointURL != nil {
		if err := validateEndpointNotEmpty(*req.EndpointURL); err != nil {
			return huma.Error400BadRequest(err.Error())
		}
	}
	if err := validateOptionalCron(req.Cron, "invalid cron expression"); err != nil {
		return err
	}
	if err := validateOptionalCron(req.ExecutionWindowCron, "invalid execution_window_cron expression"); err != nil {
		return err
	}
	if req.RetryStrategy != nil {
		if err := validateRetryConfig(*req.RetryStrategy, nil); err != nil {
			return huma.Error400BadRequest(err.Error())
		}
	}
	if req.RetryDelaysSecs != nil {
		if err := validateRetryConfig("", *req.RetryDelaysSecs); err != nil {
			return huma.Error400BadRequest(err.Error())
		}
	}
	rateLimitWindowSecs := current.RateLimitWindowSecs
	if req.RateLimitWindowSecs != nil {
		rateLimitWindowSecs = *req.RateLimitWindowSecs
	}
	dedupWindowSecs := current.DedupWindowSecs
	if req.DedupWindowSecs != nil {
		dedupWindowSecs = *req.DedupWindowSecs
	}
	if err := s.validateWindowsAgainstRetention(rateLimitWindowSecs, dedupWindowSecs); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	return nil
}

func validateOptionalCron(expr *string, invalidMessage string) error {
	if expr == nil || *expr == "" {
		return nil
	}
	if err := validateCronFieldCount(*expr); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := parser.Parse(*expr); err != nil {
		return huma.Error400BadRequest(invalidMessage)
	}
	return nil
}

func (s *Server) persistUpdatedJob(ctx context.Context, job *domain.Job, req UpdateJobRequest, addsCronSchedule bool) error {
	job.UpdatedBy = actorFromContext(ctx)

	var err error
	if addsCronSchedule {
		err = s.updateJobWithCronScheduleLimit(ctx, job)
	} else {
		err = s.store.UpdateJob(ctx, job)
	}
	if err != nil {
		if errors.Is(err, store.ErrJobVersionConflict) {
			return huma.Error409Conflict("job was modified concurrently -- retry with latest version")
		}
		var statusErr huma.StatusError
		if errors.As(err, &statusErr) {
			return err
		}
		return huma.Error500InternalServerError("failed to update job")
	}
	s.invalidateWorkerJobCache(ctx, job.ID, job.CacheVersion)

	s.enqueueJobMetadata(job)

	s.emitAuditEvent(ctx, domain.AuditActionJobUpdated, "job", job.ID, map[string]any{
		"changes": sanitizedJobUpdateAuditChanges(req),
		"name":    job.Name,
		"slug":    job.Slug,
	})

	return nil
}

func (s *Server) updateJobWithCronScheduleLimit(ctx context.Context, job *domain.Job) error {
	orgID, maxSchedules, displayName, err := s.resolveScheduleCreateLimit(ctx, job.ProjectID, job.Cron)
	if err != nil {
		return err
	}

	if updater, ok := s.store.(jobCronScheduleLimitUpdater); ok {
		err = updater.UpdateJobWithCronScheduleLimit(ctx, job, orgID, maxSchedules)
	} else {
		if err := s.checkScheduleLimit(ctx, job.ProjectID, job.Cron); err != nil {
			return err
		}
		err = s.store.UpdateJob(ctx, job)
	}
	if errors.Is(err, store.ErrCronScheduleLimitExceeded) {
		s.dispatchWorkflowRegistrationRejected(ctx, job.ProjectID, "schedule_limit", maxSchedules, maxSchedules)
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d scheduled jobs. Upgrade at /settings/billing", displayName, maxSchedules),
		)
	}
	return err
}

// enqueueJobMetadata sends a job metadata record to the ClickHouse exporter
// so the job_metadata table stays in sync with Postgres.
func (s *Server) enqueueJobMetadata(job *domain.Job) {
	if s.chExporter == nil || job == nil {
		return
	}
	s.chExporter.Enqueue(clickhouse.JobMetadataRecord{
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Slug:      job.Slug,
	})
}

// DeleteJobInput is the typed input for deleting a job.
type DeleteJobInput struct {
	JobID string `path:"jobID"`
}

func (s *Server) handleDeleteJob(ctx context.Context, input *DeleteJobInput) (*struct{}, error) {
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}

	if err := s.store.DeleteJob(ctx, input.JobID); err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		if errors.Is(err, store.ErrJobHasActiveRuns) {
			return nil, huma.Error409Conflict("job has active runs — cancel them first or wait for completion")
		}
		return nil, huma.Error500InternalServerError("failed to delete job")
	}
	s.invalidateWorkerJobCache(ctx, input.JobID, time.Now().UnixNano())

	slog.Info("job deleted",
		"job_id", input.JobID,
		"actor", actorFromContext(ctx),
		"project_id", projectIDFromContext(ctx))
	s.emitAuditEvent(ctx, domain.AuditActionJobDeleted, "job", input.JobID, nil)

	return nil, nil
}

type CloneJobRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// CloneJobInput is the typed input for cloning a job.
type CloneJobInput struct {
	JobID string `path:"jobID"`
	Body  CloneJobRequest
}

// CloneJobOutput is the typed output for cloning a job.
type CloneJobOutput struct {
	Body *domain.Job
}

func (s *Server) handleCloneJob(ctx context.Context, input *CloneJobInput) (*CloneJobOutput, error) {
	source, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}

	if err := requireProjectMatch(ctx, source.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, source.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}

	req := input.Body
	if req.Name == "" || req.Slug == "" {
		return nil, huma.Error400BadRequest("name and slug are required")
	}
	if err := validateJobName(req.Name); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := validateJobSlug(req.Slug); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	// Enforce plan gates on the cloned job's properties.
	if err := s.checkHTTPModeAllowed(ctx, source.ExecutionMode, source.ProjectID); err != nil {
		return nil, err
	}
	if err := s.checkJobChainingAllowed(ctx, source.ProjectID, source.OnCompleteTriggerJob, source.OnCompleteTriggerWorkflow); err != nil {
		return nil, err
	}
	if err := s.checkJobChainingAllowed(ctx, source.ProjectID, source.OnFailureTriggerJob, source.OnFailureTriggerWorkflow); err != nil {
		return nil, err
	}
	if err := s.checkCronOverlapPolicy(ctx, source.ProjectID, string(source.CronOverlapPolicy)); err != nil {
		return nil, err
	}
	if err := s.checkCronMinInterval(ctx, source.ProjectID, source.Cron); err != nil {
		return nil, err
	}
	if err := s.checkRunTTLLimit(ctx, source.ProjectID, source.RunTTLSecs); err != nil {
		return nil, err
	}
	if err := s.checkPerJobConcurrencyLimit(ctx, source.ProjectID, source.MaxConcurrency, source.MaxConcurrencyPerKey); err != nil {
		return nil, err
	}

	clone := &domain.Job{
		ProjectID:                 source.ProjectID,
		GroupID:                   source.GroupID,
		Name:                      req.Name,
		Slug:                      req.Slug,
		Description:               source.Description,
		Cron:                      source.Cron,
		PayloadSchema:             source.PayloadSchema,
		Tags:                      source.Tags,
		EndpointURL:               source.EndpointURL,
		FallbackEndpointURL:       source.FallbackEndpointURL,
		MaxAttempts:               source.MaxAttempts,
		TimeoutSecs:               source.TimeoutSecs,
		MaxConcurrency:            source.MaxConcurrency,
		MaxConcurrencyPerKey:      source.MaxConcurrencyPerKey,
		ExecutionWindowCron:       source.ExecutionWindowCron,
		Timezone:                  source.Timezone,
		RateLimitMax:              source.RateLimitMax,
		RateLimitWindowSecs:       source.RateLimitWindowSecs,
		DedupWindowSecs:           source.DedupWindowSecs,
		WebhookURL:                source.WebhookURL,
		WebhookSecret:             source.WebhookSecret,
		RunTTLSecs:                source.RunTTLSecs,
		RetryStrategy:             source.RetryStrategy,
		RetryDelaysSecs:           source.RetryDelaysSecs,
		RetryPriorityBoost:        source.RetryPriorityBoost,
		DLQAlertThreshold:         source.DLQAlertThreshold,
		QueueDepthAlertThreshold:  source.QueueDepthAlertThreshold,
		EnvironmentID:             source.EnvironmentID,
		DefaultRunMetadata:        source.DefaultRunMetadata,
		ResultSchema:              source.ResultSchema,
		DebounceWindowSecs:        source.DebounceWindowSecs,
		BatchWindowSecs:           source.BatchWindowSecs,
		BatchMaxSize:              source.BatchMaxSize,
		Enabled:                   true,
		VersionPolicy:             source.VersionPolicy,
		BackwardsCompatible:       source.BackwardsCompatible,
		CronOverlapPolicy:         source.CronOverlapPolicy,
		ExecutionMode:             source.ExecutionMode,
		Queue:                     normalizeJobQueueName(source.Queue),
		PoisonPillThreshold:       source.PoisonPillThreshold,
		OnCompleteTriggerWorkflow: source.OnCompleteTriggerWorkflow,
		OnCompleteTriggerJob:      source.OnCompleteTriggerJob,
		OnCompletePayloadMapping:  source.OnCompletePayloadMapping,
		OnFailureTriggerJob:       source.OnFailureTriggerJob,
		OnFailureTriggerWorkflow:  source.OnFailureTriggerWorkflow,
		OnFailurePayloadMapping:   source.OnFailurePayloadMapping,
		EndpointSigningSecret:     source.EndpointSigningSecret,
		CreatedBy:                 actorFromContext(ctx),
		UpdatedBy:                 actorFromContext(ctx),
	}

	if err := s.createJobWithCronScheduleLimit(ctx, clone); err != nil {
		var statusErr huma.StatusError
		if errors.As(err, &statusErr) {
			return nil, err
		}
		return nil, huma.Error500InternalServerError("failed to clone job")
	}

	s.emitAuditEvent(ctx, domain.AuditActionJobCloned, "job", clone.ID, map[string]any{
		"source_job_id": source.ID,
		"new_name":      clone.Name,
		"new_slug":      clone.Slug,
	})

	return &CloneJobOutput{Body: clone}, nil
}

// queueNameRe is the allowed pattern for queue names: alphanumerics, dashes, underscores, 1–63 chars.
var queueNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,63}$`)

// validateQueueName returns an error if the queue name is non-empty and does not match
// the required pattern ^[A-Za-z0-9_-]{1,63}$.
func validateQueueName(name string) error {
	if name == "" {
		return nil
	}
	if !queueNameRe.MatchString(name) {
		return fmt.Errorf("queue_name must match ^[A-Za-z0-9_-]{1,63}$")
	}
	return nil
}

func normalizeJobQueueName(name string) string {
	if name == "" {
		return defaultJobQueueName
	}
	return name
}

func filterJobsForEnvironment(ctx context.Context, jobs []domain.Job) []domain.Job {
	callerEnv := environmentIDFromContext(ctx)
	if callerEnv == "" {
		return jobs
	}
	filtered := jobs[:0]
	for _, job := range jobs {
		if job.EnvironmentID == callerEnv {
			filtered = append(filtered, job)
		}
	}
	return filtered
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

func (s *Server) defaultJobMaxAttempts() int {
	if s.config != nil && s.config.DefaultJobMaxAttempts > 0 {
		return s.config.DefaultJobMaxAttempts
	}
	return 3
}

func (s *Server) defaultJobTimeoutSecs() int {
	if s.config != nil && s.config.DefaultJobTimeoutSecs > 0 {
		return s.config.DefaultJobTimeoutSecs
	}
	return 300
}

func (s *Server) validateWindowsAgainstRetention(rateLimitWindowSecs, dedupWindowSecs int) error {
	if s.config == nil {
		return nil
	}
	maxSecs := int(s.config.RunRetentionShort.Seconds())
	if maxSecs <= 0 {
		return nil
	}
	if rateLimitWindowSecs > maxSecs {
		return fmt.Errorf("rate_limit_window_secs (%d) exceeds hot retention (%d seconds)", rateLimitWindowSecs, maxSecs)
	}
	if dedupWindowSecs > maxSecs {
		return fmt.Errorf("dedup_window_secs (%d) exceeds hot retention (%d seconds)", dedupWindowSecs, maxSecs)
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

type BatchCreateJobsInput struct {
	Body BatchCreateJobsRequest
}

type BatchCreateJobsOutput struct {
	Body BatchCreateJobsResponse
}

func (s *Server) handleBatchCreateJobs(ctx context.Context, input *BatchCreateJobsInput) (*BatchCreateJobsOutput, error) {
	req := input.Body

	if len(req.Jobs) == 0 {
		return nil, huma.Error400BadRequest("jobs array is required and must not be empty")
	}
	if len(req.Jobs) > maxBatchSize {
		return nil, huma.Error400BadRequest(fmt.Sprintf("too many jobs in batch (max %d)", maxBatchSize))
	}

	var resp BatchCreateJobsResponse
	for i, jobReq := range req.Jobs {
		if err := s.validateCreateJobFields(ctx, &jobReq); err != nil {
			resp.Errors = append(resp.Errors, BatchError{Index: i, Message: batchJobErrorMessage(err)})
			continue
		}
		execMode, err := s.resolveAndCheckExecMode(ctx, &jobReq)
		if err != nil {
			resp.Errors = append(resp.Errors, BatchError{Index: i, Message: batchJobErrorMessage(err)})
			continue
		}
		if err := s.checkCronOverlapPolicy(ctx, jobReq.ProjectID, jobReq.CronOverlapPolicy); err != nil {
			resp.Errors = append(resp.Errors, BatchError{Index: i, Message: batchJobErrorMessage(err)})
			continue
		}
		if err := s.checkCronMinInterval(ctx, jobReq.ProjectID, jobReq.Cron); err != nil {
			resp.Errors = append(resp.Errors, BatchError{Index: i, Message: batchJobErrorMessage(err)})
			continue
		}
		if err := s.checkRunTTLLimit(ctx, jobReq.ProjectID, jobReq.RunTTLSecs); err != nil {
			resp.Errors = append(resp.Errors, BatchError{Index: i, Message: batchJobErrorMessage(err)})
			continue
		}
		if err := s.checkPerJobConcurrencyLimit(ctx, jobReq.ProjectID, jobReq.MaxConcurrency, jobReq.MaxConcurrencyPerKey); err != nil {
			resp.Errors = append(resp.Errors, BatchError{Index: i, Message: batchJobErrorMessage(err)})
			continue
		}
		signingSecret, err := s.resolveCreateJobSigningSecret(jobReq)
		if err != nil {
			resp.Errors = append(resp.Errors, BatchError{Index: i, Message: "failed to encrypt endpoint signing secret"})
			continue
		}

		job := &domain.Job{
			ProjectID:             jobReq.ProjectID,
			GroupID:               jobReq.GroupID,
			Name:                  jobReq.Name,
			Slug:                  jobReq.Slug,
			Description:           jobReq.Description,
			Cron:                  jobReq.Cron,
			PayloadSchema:         jobReq.PayloadSchema,
			Tags:                  jobReq.Tags,
			EndpointURL:           jobReq.EndpointURL,
			EndpointSigningSecret: signingSecret,
			FallbackEndpointURL:   jobReq.FallbackEndpointURL,
			MaxAttempts:           jobReq.MaxAttempts,
			TimeoutSecs:           jobReq.TimeoutSecs,
			MaxConcurrency:        jobReq.MaxConcurrency,
			ExecutionWindowCron:   jobReq.ExecutionWindowCron,
			Timezone:              jobReq.Timezone,
			RateLimitMax:          jobReq.RateLimitMax,
			RateLimitWindowSecs:   jobReq.RateLimitWindowSecs,
			DedupWindowSecs:       jobReq.DedupWindowSecs,
			RunTTLSecs:            jobReq.RunTTLSecs,
			RetryStrategy:         jobReq.RetryStrategy,
			RetryDelaysSecs:       jobReq.RetryDelaysSecs,
			RetryPriorityBoost:    jobReq.RetryPriorityBoost,
			EnvironmentID:         jobReq.EnvironmentID,
			Enabled:               true,
			ExecutionMode:         execMode,
			Queue:                 normalizeJobQueueName(jobReq.QueueName),
			CronOverlapPolicy:     domain.CronOverlapPolicy(jobReq.CronOverlapPolicy),
			VersionPolicy:         domain.VersionPolicyPin,
			CreatedBy:             actorFromContext(ctx),
			UpdatedBy:             actorFromContext(ctx),
		}

		if err := s.createJobWithCronScheduleLimit(ctx, job); err != nil {
			resp.Errors = append(resp.Errors, BatchError{Index: i, Message: batchJobErrorMessage(err)})
			continue
		}

		resp.Created = append(resp.Created, *job)
	}

	if len(resp.Created) == 0 && len(resp.Errors) > 0 {
		return nil, &rawStatusError{status: http.StatusBadRequest, body: resp}
	}

	if len(resp.Created) > 0 {
		ids := make([]string, 0, len(resp.Created))
		for i := range resp.Created {
			ids = append(ids, resp.Created[i].ID)
		}
		s.emitAuditEvent(ctx, domain.AuditActionJobBatchCreated, "job", "", map[string]any{
			"count":   len(ids),
			"job_ids": ids,
			"errors":  len(resp.Errors),
		})
	}

	return &BatchCreateJobsOutput{Body: resp}, nil
}

func batchJobErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var rse *rawStatusError
	if errors.As(err, &rse) {
		if msg, ok := rse.body.(string); ok && msg != "" {
			return msg
		}
	}
	return err.Error()
}

// BatchEnableJobsInput is the typed input for batch enabling jobs.
type BatchEnableJobsInput struct {
	Body BatchJobIDsRequest
}

// BatchUpdateResultOutput is the typed output for batch update operations.
type BatchUpdateResultOutput struct {
	Body BatchUpdateResult
}

func (s *Server) handleBatchEnableJobs(ctx context.Context, input *BatchEnableJobsInput) (*BatchUpdateResultOutput, error) {
	req := input.Body

	if len(req.IDs) == 0 {
		return nil, huma.Error400BadRequest("ids array is required and must not be empty")
	}
	if len(req.IDs) > maxBatchSize {
		return nil, huma.Error400BadRequest(fmt.Sprintf("too many ids in batch (max %d)", maxBatchSize))
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" && !isInternalCaller(ctx) {
		return nil, huma.Error400BadRequest("project context is required -- authenticate with an API key")
	}
	if projectID == "" && isInternalCaller(ctx) {
		s.emitInternalSecretBypassAudit(ctx, "batch_enable_jobs.project_match", "handleBatchEnableJobs", "job", "")
	}

	ids, err := s.filterBatchJobIDsForCaller(ctx, req.IDs)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return &BatchUpdateResultOutput{Body: BatchUpdateResult{Updated: 0}}, nil
	}

	updated, err := s.store.BatchUpdateJobsEnabled(ctx, ids, true, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to enable jobs")
	}

	s.emitAuditEvent(ctx, domain.AuditActionJobBatchEnabled, "job", "", map[string]any{
		"count":   updated,
		"job_ids": ids,
	})

	return &BatchUpdateResultOutput{Body: BatchUpdateResult{Updated: updated}}, nil
}

// BatchDisableJobsInput is the typed input for batch disabling jobs.
type BatchDisableJobsInput struct {
	Body BatchJobIDsRequest
}

func (s *Server) handleBatchDisableJobs(ctx context.Context, input *BatchDisableJobsInput) (*BatchUpdateResultOutput, error) {
	req := input.Body

	if len(req.IDs) == 0 {
		return nil, huma.Error400BadRequest("ids array is required and must not be empty")
	}
	if len(req.IDs) > maxBatchSize {
		return nil, huma.Error400BadRequest(fmt.Sprintf("too many ids in batch (max %d)", maxBatchSize))
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" && !isInternalCaller(ctx) {
		return nil, huma.Error400BadRequest("project context is required -- authenticate with an API key")
	}
	if projectID == "" && isInternalCaller(ctx) {
		s.emitInternalSecretBypassAudit(ctx, "batch_disable_jobs.project_match", "handleBatchDisableJobs", "job", "")
	}

	ids, err := s.filterBatchJobIDsForCaller(ctx, req.IDs)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return &BatchUpdateResultOutput{Body: BatchUpdateResult{Updated: 0}}, nil
	}

	updated, err := s.store.BatchUpdateJobsEnabled(ctx, ids, false, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to disable jobs")
	}

	s.emitAuditEvent(ctx, domain.AuditActionJobBatchDisabled, "job", "", map[string]any{
		"count":   updated,
		"job_ids": ids,
	})

	return &BatchUpdateResultOutput{Body: BatchUpdateResult{Updated: updated}}, nil
}

func (s *Server) filterBatchJobIDsForCaller(ctx context.Context, ids []string) ([]string, error) {
	if projectIDFromContext(ctx) == "" || environmentIDFromContext(ctx) == "" {
		return ids, nil
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		job, err := s.store.GetJob(ctx, id)
		if err != nil {
			if errors.Is(err, store.ErrJobNotFound) {
				continue
			}
			return nil, huma.Error500InternalServerError("failed to get job")
		}
		if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
			continue
		}
		if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
			continue
		}
		filtered = append(filtered, id)
	}
	return filtered, nil
}

// JobHealthResponse wraps health stats with the time window.
type JobHealthResponse struct {
	JobID  string    `json:"job_id"`
	Window string    `json:"window"`
	Since  time.Time `json:"since"`
	*store.JobHealthStats
}

// GetJobHealthInput is the typed input for getting job health stats.
type GetJobHealthInput struct {
	JobID  string `path:"jobID"`
	Window string `query:"window"`
}

// GetJobHealthOutput is the typed output for getting job health stats.
type GetJobHealthOutput struct {
	Body JobHealthResponse
}

func (s *Server) handleGetJobHealth(ctx context.Context, input *GetJobHealthInput) (*GetJobHealthOutput, error) {
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}

	window := input.Window
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
		return nil, huma.Error400BadRequest("invalid window: must be 1h, 1d, 7d, or 30d")
	}

	stats, err := s.store.GetJobHealthStats(ctx, input.JobID, since)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to compute health stats")
	}

	return &GetJobHealthOutput{Body: JobHealthResponse{
		JobID:          input.JobID,
		Window:         window,
		Since:          since,
		JobHealthStats: stats,
	}}, nil
}

// parsePaginationFromStrings parses limit and cursor from string query params.
func parsePaginationFromStrings(limitStr, cursorStr string) (int, *time.Time, error) {
	limit := defaultPageLimit
	if limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed <= 0 {
			return 0, nil, &paginationError{msg: "limit must be a positive integer"}
		}
		if parsed > maxPageLimit {
			parsed = maxPageLimit
		}
		limit = parsed
	}

	var cursor *time.Time
	if cursorStr != "" {
		t, err := time.Parse(time.RFC3339Nano, cursorStr)
		if err != nil {
			return 0, nil, &paginationError{msg: "invalid cursor format"}
		}
		cursor = &t
	}

	return limit, cursor, nil
}

const maxPauseReasonLen = 500

// PauseJobRequest is the optional body for pausing a job.
type PauseJobRequest struct {
	Reason string `json:"reason,omitempty" maxLength:"500"`
}

// PauseJobInput is the typed input for pausing a job.
type PauseJobInput struct {
	JobID string `path:"jobID"`
	Body  PauseJobRequest
}

// PauseJobOutput is the typed output for pausing a job.
type PauseJobOutput struct {
	Body *domain.Job
}

func (s *Server) handlePauseJob(ctx context.Context, input *PauseJobInput) (*PauseJobOutput, error) {
	if len(input.Body.Reason) > maxPauseReasonLen {
		return nil, huma.Error400BadRequest(fmt.Sprintf("reason must be %d characters or fewer", maxPauseReasonLen))
	}

	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}

	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}

	alreadyPaused := job.Paused

	if !alreadyPaused {
		if err := s.store.PauseJob(ctx, input.JobID, input.Body.Reason); err != nil {
			if errors.Is(err, store.ErrJobNotFound) {
				return nil, huma.Error404NotFound("job not found")
			}
			return nil, huma.Error500InternalServerError("failed to pause job")
		}

		slog.Info("job paused",
			"job_id", input.JobID,
			"reason", input.Body.Reason,
			"actor", actorFromContext(ctx),
			"project_id", projectIDFromContext(ctx))
		s.emitAuditEvent(ctx, domain.AuditActionJobPaused, "job", input.JobID, map[string]any{
			"reason": input.Body.Reason,
		})
	}

	updated, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated job")
	}

	return &PauseJobOutput{Body: updated}, nil
}

// ResumeJobInput is the typed input for resuming a job.
type ResumeJobInput struct {
	JobID string `path:"jobID"`
}

// ResumeJobOutput is the typed output for resuming a job.
type ResumeJobOutput struct {
	Body *domain.Job
}

func (s *Server) handleResumeJob(ctx context.Context, input *ResumeJobInput) (*ResumeJobOutput, error) {
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}

	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}

	wasPaused := job.Paused

	if wasPaused {
		if err := s.store.ResumeJob(ctx, input.JobID); err != nil {
			if errors.Is(err, store.ErrJobNotFound) {
				return nil, huma.Error404NotFound("job not found")
			}
			return nil, huma.Error500InternalServerError("failed to resume job")
		}

		slog.Info("job resumed",
			"job_id", input.JobID,
			"actor", actorFromContext(ctx),
			"project_id", projectIDFromContext(ctx))
		s.emitAuditEvent(ctx, domain.AuditActionJobResumed, "job", input.JobID, nil)
	}

	updated, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated job")
	}

	return &ResumeJobOutput{Body: updated}, nil
}

// checkHTTPModeAllowed verifies that HTTP execution mode is allowed for the org's plan.
// Returns nil if allowed, or a 400 error if the plan doesn't support HTTP mode.
func (s *Server) checkHTTPModeAllowed(ctx context.Context, mode domain.ExecutionMode, projectID string) error {
	if mode != domain.ExecutionModeHTTP {
		return nil
	}
	if !s.edition.RequiresHTTPModeGating() {
		return nil
	}
	if s.billingEnforcer == nil {
		return planGateUnavailable("http_mode_enforcer", errors.New("billing enforcer not configured"))
	}

	orgID, err := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil || orgID == "" {
		if err != nil {
			return planGateUnavailable("http_mode_org_lookup", err)
		}
		return nil
	}

	limits, err := s.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		return planGateUnavailable("http_mode_plan_lookup", err)
	}

	if !limits.AllowsHTTPMode {
		billing.RecordHTTPModeGateRejected(ctx, string(limits.PlanTier), "job_create")
		return huma.Error400BadRequest("HTTP execution mode is unavailable for this organization. Contact support if this persists.")
	}
	return nil
}

// validateCronFieldCount checks that a cron expression has exactly 5 fields
// (minute, hour, day-of-month, month, day-of-week). The cron parser is
// configured without seconds support, so 6-field expressions are rejected.
func validateCronFieldCount(expr string) error {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("cron expression must have exactly 5 fields (minute hour day-of-month month day-of-week), got %d", len(fields))
	}
	return nil
}
