package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type SDKCompleteRequest struct {
	Result json.RawMessage `json:"result,omitempty"`
}
type SDKCompleteInput struct {
	RunID string `path:"runID"`
	Body  SDKCompleteRequest
}
type SDKCompleteOutput struct{ Body *domain.JobRun }

func (s *Server) handleSDKComplete(ctx context.Context, input *SDKCompleteInput) (*SDKCompleteOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if s.config != nil && s.config.MaxResultSize > 0 && int64(len(req.Result)) > s.config.MaxResultSize {
		return nil, huma.Error413RequestEntityTooLarge(fmt.Sprintf("result size %d exceeds maximum %d bytes", len(req.Result), s.config.MaxResultSize))
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get run")
	}
	if len(req.Result) > 0 {
		job, jobErr := s.store.GetJob(ctx, run.JobID)
		if jobErr == nil && job != nil && len(job.ResultSchema) > 0 {
			if schemaErr := validatePayloadAgainstSchema(req.Result, job.ResultSchema); schemaErr != nil {
				return nil, &typedAPIError{
					status: 422,
					apiError: APIError{
						Code:    "result_schema_validation_failed",
						Message: "result schema validation failed",
						Details: []string{schemaErr.Error()},
					},
				}
			}
		}
	}
	now := time.Now()
	fields := map[string]any{"finished_at": now}
	if len(req.Result) > 0 {
		fields["result"] = req.Result
	}
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		err = guardedStore.UpdateRunStatusForActiveRun(ctx, runID, run.Status, domain.StatusCompleted, fields, runTokenAttemptFromContext(ctx))
	} else {
		err = s.store.UpdateRunStatus(ctx, runID, run.Status, domain.StatusCompleted, fields)
	}
	if err != nil {
		slog.Error("failed to complete run", "run_id", runID, "error", err)
		if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
			return nil, sdkErr
		}
		if errors.Is(err, store.ErrRunConflict) {
			return nil, huma.Error409Conflict("run status conflict")
		}
		return nil, huma.Error500InternalServerError("failed to update run")
	}
	if s.workflowCallback != nil {
		completedRun := *run
		completedRun.Status = domain.StatusCompleted
		if cbErr := s.workflowCallback.OnJobRunTerminal(ctx, &completedRun); cbErr != nil {
			slog.Error("workflow callback failed", "run_id", runID, "error", cbErr)
		}
	}
	if err := s.resumeWaitingParentIfReady(ctx, run); err != nil {
		slog.Error("failed to resume waiting parent", "run_id", runID, "error", err)
	}
	if s.pubsub != nil {
		payload, err := marshalSDKStatusChangePayload(runID, string(run.Status), "completed", now.UTC())
		if err != nil {
			slog.Warn("failed to marshal status change payload", "run_id", runID, "error", err)
		} else {
			if err := s.pubsub.Publish(ctx, apiRunPubSubChannel(runID), payload); err != nil {
				slog.Warn("failed to publish event", "run_id", runID, "error", err)
			}
		}
	}
	updatedRun, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated run")
	}
	return &SDKCompleteOutput{Body: updatedRun}, nil
}

type SDKFailRequest struct {
	Error string `json:"error" validate:"required"`
}
type SDKFailInput struct {
	RunID string `path:"runID"`
	Body  SDKFailRequest
}
type SDKFailOutput struct{ Body *domain.JobRun }

func (s *Server) handleSDKFail(ctx context.Context, input *SDKFailInput) (*SDKFailOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get run")
	}
	now := time.Now()
	failFields := map[string]any{"finished_at": now, "error": req.Error}
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		err = guardedStore.UpdateRunStatusForActiveRun(ctx, runID, run.Status, domain.StatusFailed, failFields, runTokenAttemptFromContext(ctx))
	} else {
		err = s.store.UpdateRunStatus(ctx, runID, run.Status, domain.StatusFailed, failFields)
	}
	if err != nil {
		slog.Error("failed to fail run", "run_id", runID, "error", err)
		if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
			return nil, sdkErr
		}
		if errors.Is(err, store.ErrRunConflict) {
			return nil, huma.Error409Conflict("run status conflict")
		}
		return nil, huma.Error500InternalServerError("failed to update run")
	}
	if s.workflowCallback != nil {
		failedRun := *run
		failedRun.Status = domain.StatusFailed
		failedRun.Error = req.Error
		if cbErr := s.workflowCallback.OnJobRunTerminal(ctx, &failedRun); cbErr != nil {
			slog.Error("workflow callback failed", "run_id", runID, "error", cbErr)
		}
	}
	if err := s.resumeWaitingParentIfReady(ctx, run); err != nil {
		slog.Error("failed to resume waiting parent", "run_id", runID, "error", err)
	}
	if s.pubsub != nil {
		payload, err := marshalSDKFailedStatusChangePayload(runID, string(run.Status), "failed", req.Error, now.UTC())
		if err != nil {
			slog.Warn("failed to marshal status change payload", "run_id", runID, "error", err)
		} else {
			if err := s.pubsub.Publish(ctx, apiRunPubSubChannel(runID), payload); err != nil {
				slog.Warn("failed to publish event", "run_id", runID, "error", err)
			}
		}
	}
	updatedRun, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated run")
	}
	return &SDKFailOutput{Body: updatedRun}, nil
}

type SDKSpawnRequest struct {
	JobSlug          string          `json:"job_slug" validate:"required"`
	ProjectID        string          `json:"project_id" validate:"required"`
	Payload          json.RawMessage `json:"payload,omitempty"`
	AwaitCompletion  bool            `json:"await_completion,omitempty"`
	AwaitTimeoutSecs int             `json:"await_timeout_secs,omitempty"`
	TargetAPIKey     string          `json:"target_api_key,omitempty"`
}
type SDKSpawnInput struct {
	RunID string `path:"runID"`
	Body  SDKSpawnRequest
}
type SDKSpawnOutput struct{ Body any }

func (s *Server) handleSDKSpawn(ctx context.Context, input *SDKSpawnInput) (*SDKSpawnOutput, error) {
	parentRunID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	parentRun, err := s.store.GetRun(ctx, parentRunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("parent run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get parent run")
	}
	if parentRun == nil {
		return nil, huma.Error404NotFound("parent run not found")
	}
	isCrossProject := req.ProjectID != parentRun.ProjectID
	var targetAPIKey *domain.APIKey
	if isCrossProject {
		if req.TargetAPIKey == "" {
			return nil, huma.Error400BadRequest("target_api_key is required for cross-project spawn")
		}
		keyHash := hashAPIKey(req.TargetAPIKey)
		apiKey, keyErr := s.lookupAPIKeyForAuth(ctx, keyHash)
		if keyErr != nil {
			return nil, huma.Error401Unauthorized("invalid target api key")
		}
		targetAPIKey = apiKey
		if apiKey.RevokedAt != nil {
			return nil, huma.Error401Unauthorized("target api key has been revoked")
		}
		now := time.Now()
		if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(now) {
			return nil, huma.Error401Unauthorized("target api key has expired")
		}
		if apiKey.ProjectID != req.ProjectID {
			return nil, huma.Error403Forbidden("target api key does not belong to the specified project")
		}
		if !domain.HasScope(apiKey.Scopes, domain.ScopeJobsTrigger) {
			return nil, huma.Error403Forbidden("target api key cannot trigger jobs")
		}
	}
	job, err := s.store.GetJobBySlug(ctx, req.ProjectID, req.JobSlug)
	if err != nil || job == nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if isCrossProject {
		if err := requireAPIKeyEnvironmentMatch(targetAPIKey, job.EnvironmentID); err != nil {
			return nil, huma.Error404NotFound("job not found")
		}
	}
	if err := validateRunCreationJobID(job.ID); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	_ = isCrossProject
	awaitTimeoutSecs := 0
	if req.AwaitCompletion {
		var timeoutErr error
		awaitTimeoutSecs, timeoutErr = normalizeSDKEventTimeoutSecs(req.AwaitTimeoutSecs)
		if timeoutErr != nil {
			return nil, huma.Error400BadRequest("await_" + timeoutErr.Error())
		}
	}
	if req.AwaitCompletion && parentRun.Status == domain.StatusExecuting {
		if err := s.ensureSDKRunActive(ctx, parentRun.ID); err != nil {
			return nil, err
		}
		if err := s.store.UpdateRunStatus(ctx, parentRun.ID, domain.StatusExecuting, domain.StatusWaiting, map[string]any{}); err != nil {
			slog.Error("failed to transition parent run to waiting", "parent_run_id", parentRun.ID, "error", err)
			return nil, huma.Error500InternalServerError("failed to transition parent to waiting")
		}
	}
	run := &domain.JobRun{JobID: job.ID, ProjectID: job.ProjectID, Payload: req.Payload, TriggeredBy: domain.TriggerSpawn, ParentRunID: parentRunID}
	if err := s.ensureSDKRunActive(ctx, parentRunID); err != nil {
		return nil, err
	}
	if err := s.queue.Enqueue(ctx, run); err != nil {
		if errors.Is(err, domain.ErrIdempotencyConflict) {
			slog.Warn("spawn idempotency conflict", "parent_run_id", parentRunID, "child_run_id", run.ID)
			return nil, huma.Error409Conflict("idempotency key conflict: a run with this key is already active")
		}
		if apiErr := enqueueAPIError(err); apiErr != nil {
			return nil, apiErr
		}
		return nil, huma.Error500InternalServerError("failed to enqueue child run")
	}
	if req.AwaitCompletion {
		eventKey := fmt.Sprintf("spawn-await:%s", run.ID)
		now := time.Now()
		expiresAt := now.Add(time.Duration(awaitTimeoutSecs) * time.Second)
		trigger := &domain.EventTrigger{
			ID:            uuid.Must(uuid.NewV7()).String(),
			EventKey:      eventKey,
			ProjectID:     parentRun.ProjectID,
			EnvironmentID: environmentIDFromContext(ctx),
			SourceType:    domain.EventSourceJobRun,
			JobRunID:      parentRun.ID,
			Status:        domain.EventTriggerStatusWaiting,
			TimeoutSecs:   awaitTimeoutSecs,
			RequestedAt:   now,
			ExpiresAt:     expiresAt,
			TriggerType:   "event",
		}
		if err := s.store.CreateEventTrigger(ctx, trigger); err != nil {
			slog.Warn("failed to create await event trigger", "parent_run_id", parentRun.ID, "child_run_id", run.ID, "event_key", eventKey, "error", err)
		} else if s.metrics != nil {
			s.metrics.EventTriggersCreated.Add(ctx, 1, metric.WithAttributes(
				attribute.String("source_type", trigger.SourceType),
				attribute.String("project_id", trigger.ProjectID),
			))
		}
	}
	resp := map[string]any{
		"id":            run.ID,
		"job_id":        run.JobID,
		"project_id":    run.ProjectID,
		"status":        run.Status,
		"parent_run_id": run.ParentRunID,
		"triggered_by":  run.TriggeredBy,
	}
	if req.AwaitCompletion {
		resp["await_completion"] = true
		resp["await_event_key"] = fmt.Sprintf("spawn-await:%s", run.ID)
	}
	return &SDKSpawnOutput{Body: resp}, nil
}

func requireAPIKeyEnvironmentMatch(apiKey *domain.APIKey, resourceEnvironmentID string) error {
	if apiKey == nil || apiKey.EnvironmentID == "" {
		return nil
	}
	if apiKey.EnvironmentID != resourceEnvironmentID {
		return errEnvironmentMismatch
	}
	return nil
}

type SDKContinueRequest struct {
	Payload json.RawMessage `json:"payload,omitempty"`
}
type SDKContinueInput struct {
	RunID string `path:"runID"`
	Body  SDKContinueRequest
}
type SDKContinueOutput struct{ Body *domain.JobRun }

func (s *Server) handleSDKContinue(ctx context.Context, input *SDKContinueInput) (*SDKContinueOutput, error) {
	parentRunID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	parentRun, err := s.store.GetRun(ctx, parentRunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get run")
	}
	if parentRun.Status != domain.StatusExecuting && parentRun.Status != domain.StatusWaiting {
		return nil, huma.Error409Conflict("run must be executing or waiting to continue")
	}
	const maxLineageDepth = 10
	if parentRun.LineageDepth >= maxLineageDepth {
		return nil, huma.Error400BadRequest(fmt.Sprintf("max lineage depth (%d) exceeded", maxLineageDepth))
	}
	job, err := s.store.GetJob(ctx, parentRun.JobID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if err := validateRunCreationJobID(job.ID); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	payload := req.Payload
	if len(payload) == 0 {
		payload = parentRun.Payload
	}
	continuationRun := &domain.JobRun{
		JobID:          job.ID,
		ProjectID:      job.ProjectID,
		Payload:        payload,
		TriggeredBy:    domain.TriggerManual,
		ContinuationOf: parentRunID,
		LineageDepth:   parentRun.LineageDepth + 1,
		Priority:       parentRun.Priority,
	}
	if err := s.ensureSDKRunActive(ctx, parentRunID); err != nil {
		return nil, err
	}
	if err := s.queue.Enqueue(ctx, continuationRun); err != nil {
		if errors.Is(err, domain.ErrIdempotencyConflict) {
			slog.Warn("continuation idempotency conflict", "parent_run_id", parentRunID, "continuation_run_id", continuationRun.ID)
			return nil, huma.Error409Conflict("idempotency key conflict: a run with this key is already active")
		}
		if apiErr := enqueueAPIError(err); apiErr != nil {
			return nil, apiErr
		}
		return nil, huma.Error500InternalServerError("failed to enqueue continuation run")
	}
	return &SDKContinueOutput{Body: continuationRun}, nil
}

func (s *Server) resumeWaitingParentIfReady(ctx context.Context, run *domain.JobRun) error {
	if run == nil || run.ParentRunID == "" {
		return nil
	}
	allTerminal, err := s.store.AreAllDescendantsTerminal(ctx, run.ParentRunID)
	if err != nil {
		return err
	}
	if !allTerminal {
		return nil
	}
	parent, err := s.store.GetRun(ctx, run.ParentRunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil
		}
		return err
	}
	if parent.Status != domain.StatusWaiting {
		return nil
	}
	return s.store.UpdateRunStatus(ctx, parent.ID, domain.StatusWaiting, domain.StatusQueued, map[string]any{
		"started_at":    nil,
		"finished_at":   nil,
		"next_retry_at": nil,
	})
}
