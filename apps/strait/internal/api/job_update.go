package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
	"strait/internal/store"
)

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
