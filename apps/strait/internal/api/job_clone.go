package api

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
	"strait/internal/store"
)

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
