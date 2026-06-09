package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
	"strait/internal/store"
)

// GetRunInput is the typed input for getting a single run.
type GetRunInput struct {
	RunID string `path:"runID"`
}

// GetRunOutput is the typed output for getting a single run.
type GetRunOutput struct {
	Body *domain.JobRun
}

func (s *Server) handleGetRun(ctx context.Context, input *GetRunInput) (*GetRunOutput, error) {
	run, err := s.getRunForAccess(ctx, input.RunID)
	if err != nil {
		return nil, err
	}

	return &GetRunOutput{Body: run}, nil
}

// ListRunsInput is the typed input for listing runs.
type ListRunsInput struct {
	Status          string   `query:"status"`
	Statuses        []string `query:"statuses"`
	TagKey          string   `query:"tag_key"`
	TagValue        string   `query:"tag_value"`
	MetadataKey     string   `query:"metadata_key"`
	MetadataValue   string   `query:"metadata_value"`
	TriggeredBy     string   `query:"triggered_by"`
	BatchID         string   `query:"batch_id"`
	PayloadContains string   `query:"payload_contains"`
	ExecutionMode   string   `query:"execution_mode"`
	ErrorClass      string   `query:"error_class"`
	Limit           string   `query:"limit"`
	Cursor          string   `query:"cursor"`
}

// ListRunsOutput is the typed output for listing runs.
type ListRunsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListRuns(ctx context.Context, input *ListRunsInput) (*ListRunsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	query, err := newListRunsQuery(input)
	if err != nil {
		return nil, err
	}

	var (
		runs    []domain.JobRun
		listErr error
	)
	if query.usesFilteredStorePath(environmentIDFromContext(ctx)) {
		runs, listErr = s.listRunsWithFilters(
			ctx,
			projectID,
			query.statusQuery,
			query.statuses,
			query.tagKey,
			query.tagValue,
			query.metadataKey,
			query.metadataValue,
			query.triggeredBy,
			query.batchID,
			query.payloadContains,
			query.executionMode,
			query.errorClass,
			query.limit+1,
			query.cursor,
		)
	} else {
		runs, listErr = s.store.ListRunsByProject(ctx, projectID, query.statusQuery, query.metadataKey, query.metadataValue, query.triggeredBy, query.batchID, query.payloadContains, query.executionMode, query.errorClass, query.limit+1, query.cursor)
	}
	if listErr != nil {
		return nil, huma.Error500InternalServerError("failed to list runs")
	}

	return &ListRunsOutput{
		Body: paginatedResult(runs, query.limit, func(run domain.JobRun) string {
			return run.CreatedAt.Format(time.RFC3339Nano)
		}),
	}, nil
}

func buildRunStatusFilter(status string, statuses []string) (map[domain.RunStatus]struct{}, *domain.RunStatus, error) {
	if status == "" && len(statuses) == 0 {
		return nil, nil, nil
	}

	filter := make(map[domain.RunStatus]struct{}, 1+len(statuses))
	addStatus := func(raw string) error {
		parsed := domain.RunStatus(raw)
		if !parsed.IsValid() {
			return huma.Error400BadRequest("status is invalid")
		}
		filter[parsed] = struct{}{}
		return nil
	}

	if status != "" {
		if err := addStatus(status); err != nil {
			return nil, nil, err
		}
	}
	for _, raw := range statuses {
		if err := addStatus(raw); err != nil {
			return nil, nil, err
		}
	}

	if len(filter) == 1 {
		for parsed := range filter {
			single := parsed
			return filter, &single, nil
		}
	}

	return filter, nil, nil
}

// CancelRunInput is the typed input for canceling a run.
type CancelRunInput struct {
	RunID string `path:"runID"`
}

// CancelRunOutput is the typed output for canceling a run.
type CancelRunOutput struct {
	Body *domain.JobRun
}

func (s *Server) handleCancelRun(ctx context.Context, input *CancelRunInput) (*CancelRunOutput, error) {
	run, err := s.getRunForAccess(ctx, input.RunID)
	if err != nil {
		return nil, err
	}

	if run.Status.IsTerminal() {
		return nil, huma.Error400BadRequest("run already in terminal state")
	}

	if err := s.store.UpdateRunStatus(ctx, run.ID, run.Status, domain.StatusCanceled, map[string]any{
		"finished_at": time.Now(),
		"error":       "canceled by user",
	}); err != nil {
		return nil, huma.Error409Conflict("failed to cancel run")
	}

	if s.workflowCallback != nil {
		canceledRun := *run
		canceledRun.Status = domain.StatusCanceled
		canceledRun.Error = "canceled by user"
		if cbErr := s.workflowCallback.OnJobRunTerminal(ctx, &canceledRun); cbErr != nil {
			slog.Error("workflow callback failed", "error", cbErr)
		}
	}

	canceledCount := s.cancelChildRunsRecursive(ctx, run.ID)
	if canceledCount > 0 && s.metrics != nil {
		s.metrics.ChildCancellationsTotal.Add(ctx, canceledCount)
	}

	updatedRun, err := s.store.GetRun(ctx, run.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated run")
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunCancelled, "run", run.ID, map[string]any{
		"job_id":            run.JobID,
		"previous_status":   string(run.Status),
		"children_canceled": canceledCount,
	})

	return &CancelRunOutput{Body: updatedRun}, nil
}

// GetRunDependencyStatusInput is the typed input for getting run dependency status.
type GetRunDependencyStatusInput struct {
	RunID string `path:"runID"`
}

// GetRunDependencyStatusOutput is the typed output for getting run dependency status.
type GetRunDependencyStatusOutput struct {
	Body map[string]any
}

func (s *Server) handleGetRunDependencyStatus(ctx context.Context, input *GetRunDependencyStatusInput) (*GetRunDependencyStatusOutput, error) {
	run, err := s.getRunForAccess(ctx, input.RunID)
	if err != nil {
		return nil, err
	}

	deps, err := s.listCachedJobDependencies(ctx, run.JobID, 1000)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list job dependencies")
	}

	satisfied, err := s.store.AreJobDependenciesSatisfied(ctx, run)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to evaluate dependencies")
	}

	return &GetRunDependencyStatusOutput{Body: map[string]any{
		"run_id":                 run.ID,
		"job_id":                 run.JobID,
		"status":                 run.Status,
		"dependencies":           deps,
		"dependencies_satisfied": satisfied,
	}}, nil
}

// ReplayRunInput is the typed input for replaying a run.
type ReplayRunInput struct {
	RunID          string `path:"runID"`
	FromCheckpoint string `query:"from_checkpoint"`
}

// ReplayRunOutput is the typed output for replaying a run.
type ReplayRunOutput struct {
	Body *domain.JobRun
}

func (s *Server) handleReplayRun(ctx context.Context, input *ReplayRunInput) (*ReplayRunOutput, error) {
	originalRun, err := s.getRunForAccess(ctx, input.RunID)
	if err != nil {
		return nil, err
	}

	if !isReplayableRunStatus(originalRun.Status) {
		return nil, huma.Error400BadRequest("run is not replayable")
	}

	job, err := s.store.GetJob(ctx, originalRun.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error400BadRequest("job not found for run")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if !job.Enabled {
		return nil, huma.Error400BadRequest("job is disabled")
	}

	payload := originalRun.Payload
	debugMode := false

	// Checkpoint-based replay: restore from a specific checkpoint sequence
	if input.FromCheckpoint != "" {
		seq, parseErr := strconv.Atoi(input.FromCheckpoint)
		if parseErr != nil || seq <= 0 {
			return nil, huma.Error400BadRequest("from_checkpoint must be a positive integer")
		}
		checkpoints, listErr := s.store.ListRunCheckpoints(ctx, input.RunID, 1000, nil)
		if listErr != nil {
			return nil, huma.Error500InternalServerError("failed to list checkpoints")
		}
		var found bool
		for _, cp := range checkpoints {
			if cp.Sequence == seq {
				payload = cp.State
				found = true
				break
			}
		}
		if !found {
			return nil, huma.Error404NotFound("checkpoint not found")
		}
		debugMode = true
	}

	now := time.Now()
	var expiresAt time.Time
	if job.RunTTLSecs > 0 {
		expiresAt = now.Add(time.Duration(job.RunTTLSecs) * time.Second)
	} else {
		expiresAt = now.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
	}

	replayRun := &domain.JobRun{
		JobID:       originalRun.JobID,
		ProjectID:   originalRun.ProjectID,
		Attempt:     1,
		Payload:     payload,
		TriggeredBy: domain.TriggerManual,
		Priority:    originalRun.Priority,
		// Deliberately do NOT copy IdempotencyKey: replays are independent
		// operations. Copying the key would conflict with any active run
		// that shares the same key (the DB unique partial index only covers
		// non-terminal statuses).
		JobVersion:   originalRun.JobVersion,
		JobVersionID: job.VersionID,
		Tags:         originalRun.Tags,
		CreatedBy:    actorFromContext(ctx),
		ExpiresAt:    &expiresAt,
		DebugMode:    debugMode,
		Metadata:     sentryRunMetadata(ctx, "POST /v1/runs/{runID}/replay", nil),
	}

	if err := s.queue.Enqueue(ctx, replayRun); err != nil {
		if errors.Is(err, domain.ErrIdempotencyConflict) {
			slog.Warn("replay idempotency conflict",
				"original_run_id", input.RunID,
				"replay_run_id", replayRun.ID)
			return nil, huma.Error409Conflict("idempotency key conflict: a run with this key is already active")
		}
		if apiErr := enqueueAPIError(err); apiErr != nil {
			return nil, apiErr
		}
		return nil, huma.Error500InternalServerError("failed to enqueue replay run")
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunReplayed, "run", replayRun.ID, map[string]any{
		"original_run_id": input.RunID,
		"job_id":          originalRun.JobID,
		"from_checkpoint": input.FromCheckpoint,
		"debug_mode":      debugMode,
	})

	return &ReplayRunOutput{Body: replayRun}, nil
}

// ListDeadLetterRunsInput is the typed input for listing dead letter runs.
type ListDeadLetterRunsInput struct {
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

// ListDeadLetterRunsOutput is the typed output for listing dead letter runs.
type ListDeadLetterRunsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListDeadLetterRuns(ctx context.Context, input *ListDeadLetterRunsInput) (*ListDeadLetterRunsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	var runs []domain.JobRun
	if environmentIDFromContext(ctx) != "" {
		runs, err = s.listDeadLetterRunsForEnvironment(ctx, projectID, limit+1, cursor)
	} else {
		runs, err = s.store.ListDeadLetterRuns(ctx, projectID, limit+1, cursor)
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list dead letter runs")
	}

	return &ListDeadLetterRunsOutput{Body: paginatedResult(runs, limit, func(run domain.JobRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

// ReplayDeadLetterRunInput is the typed input for replaying a dead letter run.
type ReplayDeadLetterRunInput struct {
	RunID string `path:"runID"`
}

// ReplayDeadLetterRunOutput is the typed output for replaying a dead letter run.
type ReplayDeadLetterRunOutput struct {
	Body *domain.JobRun
}

type existingRunEnqueuer interface {
	EnqueueExisting(context.Context, *domain.JobRun) error
}

func (s *Server) enqueueExistingRunIfSupported(ctx context.Context, run *domain.JobRun) error {
	if run == nil || run.Status != domain.StatusQueued {
		return nil
	}
	enqueuer, ok := s.queue.(existingRunEnqueuer)
	if !ok {
		return nil
	}
	return enqueuer.EnqueueExisting(ctx, run)
}

func (s *Server) handleReplayDeadLetterRun(ctx context.Context, input *ReplayDeadLetterRunInput) (*ReplayDeadLetterRunOutput, error) {
	if err := s.requireRunAccess(ctx, input.RunID); err != nil {
		return nil, err
	}

	run, err := s.store.ReplayDeadLetterRun(ctx, input.RunID)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrRunNotFound):
			return nil, huma.Error404NotFound("run not found")
		case errors.Is(err, store.ErrRunConflict):
			return nil, huma.Error409Conflict("run is not in dead_letter status")
		default:
			return nil, huma.Error500InternalServerError("failed to replay dead letter run")
		}
	}
	if err := s.enqueueExistingRunIfSupported(ctx, run); err != nil {
		if apiErr := enqueueAPIError(err); apiErr != nil {
			return nil, apiErr
		}
		return nil, huma.Error500InternalServerError("failed to enqueue replayed run")
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunReplayedDeadletter, "run", run.ID, map[string]any{
		"original_run_id": input.RunID,
		"job_id":          run.JobID,
	})

	return &ReplayDeadLetterRunOutput{Body: run}, nil
}

// BulkReplayDeadLetterRunsRequest is the request body for bulk replaying dead letter runs.
type BulkReplayDeadLetterRunsRequest struct {
	RunIDs    []string `json:"run_ids"`
	ProjectID string   `json:"project_id"`
	Limit     int      `json:"limit"`
}

// BulkReplayDeadLetterRunsInput is the typed input for bulk replaying dead letter runs.
type BulkReplayDeadLetterRunsInput struct {
	Body BulkReplayDeadLetterRunsRequest
}

// BulkReplayDeadLetterRunsOutput is the typed output for bulk replaying dead letter runs.
type BulkReplayDeadLetterRunsOutput struct {
	Body map[string]any
}

func (s *Server) handleBulkReplayDeadLetterRuns(ctx context.Context, input *BulkReplayDeadLetterRunsInput) (*BulkReplayDeadLetterRunsOutput, error) {
	req := input.Body

	hasRunIDs := len(req.RunIDs) > 0
	hasProjectID := req.ProjectID != ""
	if hasRunIDs == hasProjectID {
		return nil, &typedAPIError{
			status: http.StatusUnprocessableEntity,
			apiError: APIError{
				Code:    ErrorCodeValidationFailed,
				Message: "provide either run_ids or project_id",
			},
		}
	}

	// Enforce tenant isolation: if project_id is supplied, it must match the caller's project.
	if hasProjectID {
		if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
			return nil, huma.Error403Forbidden("project access denied")
		}
	}

	// Enforce tenant isolation: if run_ids are supplied, verify each belongs to the caller's project.
	if hasRunIDs {
		if len(req.RunIDs) > 500 {
			return nil, &typedAPIError{
				status: http.StatusUnprocessableEntity,
				apiError: APIError{
					Code:    ErrorCodeValidationFailed,
					Message: "too many run_ids (max 500)",
				},
			}
		}
		for _, runID := range req.RunIDs {
			if _, err := s.getRunForAccess(ctx, runID); err != nil {
				return nil, err
			}
		}
	} else {
		if req.Limit <= 0 {
			req.Limit = 100
		}
		if req.Limit > 500 {
			return nil, &typedAPIError{
				status: http.StatusUnprocessableEntity,
				apiError: APIError{
					Code:    ErrorCodeValidationFailed,
					Message: "limit must be <= 500",
				},
			}
		}
	}

	var (
		runs []domain.JobRun
		err  error
	)
	if !hasRunIDs && environmentIDFromContext(ctx) != "" {
		runs, err = s.bulkReplayDeadLetterRunsForEnvironment(ctx, projectIDFromContext(ctx), req.Limit)
	} else if hasRunIDs {
		runs, err = s.store.BulkReplayDeadLetterRuns(ctx, req.RunIDs, "", 0)
	} else {
		// Scope bulk replay to the caller's project when using project_id mode.
		runs, err = s.store.BulkReplayDeadLetterRuns(ctx, nil, req.ProjectID, req.Limit)
	}
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "at least one") || strings.Contains(errMsg, "provide either"):
			return nil, &typedAPIError{
				status:   http.StatusUnprocessableEntity,
				apiError: APIError{Code: ErrorCodeValidationFailed, Message: errMsg},
			}
		case strings.Contains(errMsg, "no dead_letter"):
			return nil, &typedAPIError{
				status:   http.StatusConflict,
				apiError: APIError{Code: ErrorCodeConflict, Message: errMsg},
			}
		default:
			return nil, huma.Error500InternalServerError("failed to bulk replay dead letter runs")
		}
	}
	for i := range runs {
		if err := s.enqueueExistingRunIfSupported(ctx, &runs[i]); err != nil {
			if apiErr := enqueueAPIError(err); apiErr != nil {
				return nil, apiErr
			}
			return nil, huma.Error500InternalServerError("failed to enqueue replayed run")
		}
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunBulkReplayedDeadletter, "run", "", map[string]any{
		"count":      len(runs),
		"project_id": projectIDFromContext(ctx),
		"run_ids":    req.RunIDs,
	})

	return &BulkReplayDeadLetterRunsOutput{Body: map[string]any{"replayed": runs, "count": len(runs)}}, nil
}

func isReplayableRunStatus(status domain.RunStatus) bool {
	switch status {
	case domain.StatusFailed, domain.StatusTimedOut, domain.StatusCrashed, domain.StatusSystemFailed:
		return true
	default:
		return false
	}
}

// ListChildRunsInput is the typed input for listing child runs.
type ListChildRunsInput struct {
	RunID  string `path:"runID"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

// ListChildRunsOutput is the typed output for listing child runs.
type ListChildRunsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListChildRuns(ctx context.Context, input *ListChildRunsInput) (*ListChildRunsOutput, error) {
	if err := s.requireRunAccess(ctx, input.RunID); err != nil {
		return nil, err
	}

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	children, err := s.store.ListChildRuns(ctx, input.RunID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list children")
	}

	return &ListChildRunsOutput{Body: paginatedResult(children, limit, func(run domain.JobRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

// GetDebugBundleInput is the typed input for getting a debug bundle.
type GetDebugBundleInput struct {
	RunID string `path:"runID"`
}

// GetDebugBundleOutput is the typed output for getting a debug bundle.
type GetDebugBundleOutput struct {
	Body *domain.DebugBundle
}

func (s *Server) handleGetDebugBundle(ctx context.Context, input *GetDebugBundleInput) (*GetDebugBundleOutput, error) {
	if err := s.requireRunAccess(ctx, input.RunID); err != nil {
		return nil, err
	}

	bundle, err := s.store.GetDebugBundle(ctx, input.RunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get debug bundle")
	}

	return &GetDebugBundleOutput{Body: bundle}, nil
}

// SetDebugModeRequest is the request body for setting debug mode.
type SetDebugModeRequest struct {
	DebugMode bool `json:"debug_mode"`
}

// SetDebugModeInput is the typed input for setting debug mode.
type SetDebugModeInput struct {
	RunID string `path:"runID"`
	Body  SetDebugModeRequest
}

// SetDebugModeOutput is the typed output for setting debug mode.
type SetDebugModeOutput struct {
	Body map[string]string
}

func (s *Server) handleSetDebugMode(ctx context.Context, input *SetDebugModeInput) (*SetDebugModeOutput, error) {
	if err := s.requireRunAccess(ctx, input.RunID); err != nil {
		return nil, err
	}

	if err := s.store.UpdateRunDebugMode(ctx, input.RunID, input.Body.DebugMode); err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to update debug mode")
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunDebugModeSet, "run", input.RunID, map[string]any{
		"enabled": input.Body.DebugMode,
	})

	return &SetDebugModeOutput{Body: map[string]string{"status": "ok"}}, nil
}

// ListRunLineageInput is the typed input for listing run lineage.
type ListRunLineageInput struct {
	RunID  string `path:"runID"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

// ListRunLineageOutput is the typed output for listing run lineage.
type ListRunLineageOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListRunLineage(ctx context.Context, input *ListRunLineageInput) (*ListRunLineageOutput, error) {
	if err := s.requireRunAccess(ctx, input.RunID); err != nil {
		return nil, err
	}

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	runs, err := s.store.ListRunLineage(ctx, input.RunID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list run lineage")
	}

	return &ListRunLineageOutput{Body: paginatedResult(runs, limit, func(run domain.JobRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

// ResetIdempotencyKeyInput is the typed input for resetting a run's idempotency key.
type ResetIdempotencyKeyInput struct {
	RunID string `path:"runID"`
}

// ResetIdempotencyKeyOutput is the typed output for resetting a run's idempotency key.
type ResetIdempotencyKeyOutput struct {
	Body map[string]string
}

func (s *Server) handleResetIdempotencyKey(ctx context.Context, input *ResetIdempotencyKeyInput) (*ResetIdempotencyKeyOutput, error) {
	if err := s.requireRunAccess(ctx, input.RunID); err != nil {
		return nil, err
	}

	if err := s.store.ResetRunIdempotencyKey(ctx, input.RunID); err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found or not eligible for idempotency reset")
		}
		return nil, huma.Error500InternalServerError("failed to reset idempotency key")
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunIdempotencyKeyReset, "run", input.RunID, nil)

	return &ResetIdempotencyKeyOutput{Body: map[string]string{"status": "reset", "run_id": input.RunID}}, nil
}

// RescheduleRunRequest is the request body for rescheduling a run.
type RescheduleRunRequest struct {
	ScheduledAt time.Time       `json:"scheduled_at" validate:"required"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

// RescheduleRunInput is the typed input for rescheduling a run.
type RescheduleRunInput struct {
	RunID string `path:"runID"`
	Body  RescheduleRunRequest
}

// RescheduleRunOutput is the typed output for rescheduling a run.
type RescheduleRunOutput struct {
	Body *domain.JobRun
}

func (s *Server) handleRescheduleRun(ctx context.Context, input *RescheduleRunInput) (*RescheduleRunOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	run, err := s.getRunForAccess(ctx, input.RunID)
	if err != nil {
		return nil, err
	}

	if err := s.store.RescheduleRun(ctx, input.RunID, req.ScheduledAt, req.Payload); err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found or not eligible for rescheduling")
		}
		return nil, huma.Error500InternalServerError("failed to reschedule run")
	}

	updatedRun, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to fetch rescheduled run")
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunRescheduled, "run", input.RunID, map[string]any{
		"job_id":           run.JobID,
		"new_scheduled_at": req.ScheduledAt,
		"payload_changed":  len(req.Payload) > 0,
	})

	return &RescheduleRunOutput{Body: updatedRun}, nil
}

// BulkReplayRunsRequest is the request body for bulk replaying runs.
type BulkReplayRunsRequest struct {
	RunIDs []string `json:"run_ids" validate:"required,min=1,max=100"`
}

// BulkReplayRunsInput is the typed input for bulk replaying runs.
type BulkReplayRunsInput struct {
	Body BulkReplayRunsRequest
}

// BulkReplayRunsOutput is the typed output for bulk replaying runs.
type BulkReplayRunsOutput struct {
	Body map[string]any
}

func (s *Server) handleBulkReplayRuns(ctx context.Context, input *BulkReplayRunsInput) (*BulkReplayRunsOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	type replayResult struct {
		OriginalRunID string `json:"original_run_id"`
		NewRunID      string `json:"new_run_id,omitempty"`
		Status        string `json:"status"`
		Error         string `json:"error,omitempty"`
	}

	results := make([]replayResult, 0, len(req.RunIDs))
	replayed := 0

	for _, runID := range req.RunIDs {
		original, err := s.getRunForAccess(ctx, runID)
		if err != nil {
			results = append(results, replayResult{OriginalRunID: runID, Status: "failed", Error: "run not found"})
			continue
		}
		if !isReplayableRunStatus(original.Status) {
			results = append(results, replayResult{OriginalRunID: runID, Status: "skipped", Error: "run is not replayable"})
			continue
		}

		job, err := s.store.GetJob(ctx, original.JobID)
		if err != nil || job == nil || !job.Enabled {
			results = append(results, replayResult{OriginalRunID: runID, Status: "failed", Error: "job not found or disabled"})
			continue
		}

		now := time.Now()
		var expiresAt time.Time
		if job.RunTTLSecs > 0 {
			expiresAt = now.Add(time.Duration(job.RunTTLSecs) * time.Second)
		} else {
			expiresAt = now.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
		}

		replayRun := &domain.JobRun{
			JobID:        original.JobID,
			ProjectID:    original.ProjectID,
			Attempt:      1,
			Payload:      original.Payload,
			TriggeredBy:  domain.TriggerManual,
			Priority:     original.Priority,
			JobVersion:   original.JobVersion,
			JobVersionID: job.VersionID,
			Tags:         original.Tags,
			CreatedBy:    actorFromContext(ctx),
			ExpiresAt:    &expiresAt,
			Metadata:     sentryRunMetadata(ctx, "POST /v1/runs/bulk-replay", nil),
		}

		if err := s.queue.Enqueue(ctx, replayRun); err != nil {
			results = append(results, replayResult{OriginalRunID: runID, Status: "failed", Error: "enqueue failed"})
			continue
		}

		results = append(results, replayResult{OriginalRunID: runID, NewRunID: replayRun.ID, Status: "replayed"})
		replayed++
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunBulkReplayed, "run", "", map[string]any{
		"count":   replayed,
		"total":   len(req.RunIDs),
		"run_ids": req.RunIDs,
	})

	return &BulkReplayRunsOutput{Body: map[string]any{"results": results, "total": len(req.RunIDs), "replayed": replayed}}, nil
}

// PauseRunInput is the typed input for pausing a run.
type PauseRunInput struct {
	RunID string `path:"runID"`
}

// PauseRunOutput is the typed output for pausing a run.
type PauseRunOutput struct {
	Body *domain.JobRun
}

func (s *Server) handlePauseRun(ctx context.Context, input *PauseRunInput) (*PauseRunOutput, error) {
	run, err := s.getRunForAccess(ctx, input.RunID)
	if err != nil {
		return nil, err
	}

	if run.Status == domain.StatusPaused {
		return &PauseRunOutput{Body: run}, nil
	}
	if run.Status != domain.StatusExecuting {
		return nil, huma.Error400BadRequest("run must be in executing state to pause")
	}

	if err := s.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusPaused, map[string]any{
		"metadata": map[string]string{},
	}); err != nil {
		return nil, huma.Error409Conflict("failed to pause run")
	}

	updatedRun, err := s.store.GetRun(ctx, run.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated run")
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunPaused, "run", run.ID, map[string]any{
		"job_id": run.JobID,
	})

	return &PauseRunOutput{Body: updatedRun}, nil
}

// ResumeRunInput is the typed input for resuming a run.
type ResumeRunInput struct {
	RunID string `path:"runID"`
}

// ResumeRunOutput is the typed output for resuming a run.
type ResumeRunOutput struct {
	Body *domain.JobRun
}

func (s *Server) handleResumeRun(ctx context.Context, input *ResumeRunInput) (*ResumeRunOutput, error) {
	run, err := s.getRunForAccess(ctx, input.RunID)
	if err != nil {
		return nil, err
	}

	if run.Status != domain.StatusPaused {
		return nil, huma.Error400BadRequest("run is not paused")
	}

	if err := s.store.UpdateRunStatus(ctx, run.ID, domain.StatusPaused, domain.StatusQueued, map[string]any{
		"started_at":  nil,
		"finished_at": nil,
		"metadata":    map[string]string{},
	}); err != nil {
		return nil, huma.Error409Conflict("failed to resume run")
	}

	updatedRun, err := s.store.GetRun(ctx, run.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated run")
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunResumed, "run", run.ID, map[string]any{
		"job_id": run.JobID,
	})

	return &ResumeRunOutput{Body: updatedRun}, nil
}

// RestartRunInput is the typed input for restarting a run.
type RestartRunInput struct {
	RunID string `path:"runID"`
	Body  restartRunRequest
}

// RestartRunOutput is the typed output for restarting a run.
type RestartRunOutput struct {
	Body *domain.JobRun
}

type restartRunRequest struct{}

func (s *Server) handleRestartRun(ctx context.Context, input *RestartRunInput) (*RestartRunOutput, error) {
	run, err := s.getRunForAccess(ctx, input.RunID)
	if err != nil {
		return nil, err
	}

	if !isRestartableRunStatus(run.Status) {
		return nil, huma.Error400BadRequest("run must be executing or paused to restart")
	}

	if err := s.store.UpdateRunStatus(ctx, run.ID, run.Status, domain.StatusQueued, map[string]any{
		"started_at":  nil,
		"finished_at": nil,
		"metadata":    map[string]string{},
	}); err != nil {
		return nil, huma.Error409Conflict("failed to restart run")
	}

	updatedRun, err := s.store.GetRun(ctx, run.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated run")
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunRestarted, "run", run.ID, map[string]any{
		"job_id": run.JobID,
	})

	return &RestartRunOutput{Body: updatedRun}, nil
}

func isRestartableRunStatus(status domain.RunStatus) bool {
	switch status {
	case domain.StatusExecuting, domain.StatusPaused:
		return true
	default:
		return false
	}
}

// parseBracketParam extracts the key from bracket notation params like "metadata[key]".
// Returns the inner key and true if the param matches "prefix[key]".
func parseBracketParam(param, prefix string) (string, bool) {
	if !strings.HasPrefix(param, prefix+"[") || !strings.HasSuffix(param, "]") {
		return "", false
	}
	key := param[len(prefix)+1 : len(param)-1]
	if key == "" {
		return "", false
	}
	return key, true
}

func (s *Server) listRunsWithFilters(
	ctx context.Context,
	projectID string,
	statusQuery *domain.RunStatus,
	statuses map[domain.RunStatus]struct{},
	tagKey, tagValue string,
	metadataKey, metadataValue, triggeredBy, batchID *string,
	payloadContains json.RawMessage,
	executionMode *domain.ExecutionMode,
	errorClass *string,
	limit int,
	cursor *time.Time,
) ([]domain.JobRun, error) {
	if lister, ok := s.store.(interface {
		ListRunsByProjectFiltered(context.Context, string, *domain.RunStatus, []domain.RunStatus, string, string, *string, *string, *string, *string, *string, json.RawMessage, *domain.ExecutionMode, *string, int, *time.Time) ([]domain.JobRun, error)
	}); ok {
		statusList := make([]domain.RunStatus, 0, len(statuses))
		for status := range statuses {
			statusList = append(statusList, status)
		}
		envID := environmentIDFromContext(ctx)
		var envFilter *string
		if envID != "" {
			envFilter = &envID
		}
		return lister.ListRunsByProjectFiltered(
			ctx,
			projectID,
			statusQuery,
			statusList,
			tagKey,
			tagValue,
			envFilter,
			metadataKey,
			metadataValue,
			triggeredBy,
			batchID,
			payloadContains,
			executionMode,
			errorClass,
			limit,
			cursor,
		)
	}

	jobEnvCache := make(map[string]bool)
	filtered := make([]domain.JobRun, 0, limit)
	pageCursor := cursor
	fetchLimit := max(limit, 25)

	for {
		var (
			page []domain.JobRun
			err  error
		)
		page, err = s.store.ListRunsByProject(
			ctx,
			projectID,
			statusQuery,
			metadataKey,
			metadataValue,
			triggeredBy,
			batchID,
			payloadContains,
			executionMode,
			errorClass,
			fetchLimit,
			pageCursor,
		)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			return filtered, nil
		}

		for _, run := range page {
			if !runMatchesTagFilter(run, tagKey, tagValue) || !runMatchesStatusFilter(run, statuses) {
				continue
			}
			if environmentIDFromContext(ctx) != "" {
				allowed, err := s.runMatchesEnvironment(ctx, run, jobEnvCache)
				if err != nil {
					return nil, err
				}
				if !allowed {
					continue
				}
			}
			filtered = append(filtered, run)
			if len(filtered) >= limit {
				return filtered, nil
			}
		}

		if len(page) < fetchLimit {
			return filtered, nil
		}
		lastCreatedAt := page[len(page)-1].CreatedAt
		pageCursor = &lastCreatedAt
	}
}

func runMatchesStatusFilter(run domain.JobRun, statuses map[domain.RunStatus]struct{}) bool {
	if len(statuses) == 0 {
		return true
	}
	_, ok := statuses[run.Status]
	return ok
}

func runMatchesTagFilter(run domain.JobRun, tagKey, tagValue string) bool {
	if tagKey == "" {
		return true
	}
	if len(run.Tags) == 0 {
		return false
	}
	value, ok := run.Tags[tagKey]
	if !ok {
		return false
	}
	if tagValue == "" {
		return true
	}
	return value == tagValue
}
