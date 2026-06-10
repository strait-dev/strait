package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
	"strait/internal/store"
)

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
