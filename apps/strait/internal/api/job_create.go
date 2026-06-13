package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

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

	s.emitAuditEvent(auditContextWithProject(ctx, job.ProjectID), domain.AuditActionJobCreated, "job", job.ID, map[string]any{
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
