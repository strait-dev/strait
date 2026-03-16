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

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}

	respondJSON(w, http.StatusOK, run)
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	projectID := query.Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	var status *domain.RunStatus
	if statusRaw := query.Get("status"); statusRaw != "" {
		parsed := domain.RunStatus(statusRaw)
		if !parsed.IsValid() {
			respondError(w, r, http.StatusBadRequest, "status is invalid")
			return
		}
		status = &parsed
	}

	tagKey := query.Get("tag_key")
	tagValue := query.Get("tag_value")

	// Support bracket notation: ?tags[key]=value
	if tagKey == "" {
		for param, values := range query {
			if k, ok := parseBracketParam(param, "tags"); ok && len(values) > 0 {
				tagKey = k
				tagValue = values[0]
				break
			}
		}
	}

	if tagValue != "" && tagKey == "" {
		respondError(w, r, http.StatusBadRequest, "tag_key is required when tag_value is provided")
		return
	}
	if tagKey != "" {
		if err := validateTags(map[string]string{tagKey: tagValue}); err != nil {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
	}

	metadataKeyRaw := query.Get("metadata_key")
	metadataValueRaw := query.Get("metadata_value")

	// Support bracket notation: ?metadata[key]=value
	if metadataKeyRaw == "" {
		for param, values := range query {
			if k, ok := parseBracketParam(param, "metadata"); ok && len(values) > 0 {
				metadataKeyRaw = k
				metadataValueRaw = values[0]
				break
			}
		}
	}

	if metadataValueRaw != "" && metadataKeyRaw == "" {
		respondError(w, r, http.StatusBadRequest, "metadata_key is required when metadata_value is provided")
		return
	}

	if tagKey != "" && metadataKeyRaw != "" {
		respondError(w, r, http.StatusBadRequest, "tag_key and metadata_key filters are mutually exclusive")
		return
	}

	var metadataKey *string
	if metadataKeyRaw != "" {
		metadataKey = &metadataKeyRaw
	}

	var metadataValue *string
	if metadataValueRaw != "" {
		metadataValue = &metadataValueRaw
	}

	var triggeredBy *string
	if tb := query.Get("triggered_by"); tb != "" {
		triggeredBy = &tb
	}

	var batchID *string
	if bid := query.Get("batch_id"); bid != "" {
		batchID = &bid
	}

	var payloadContains json.RawMessage
	if pc := query.Get("payload_contains"); pc != "" {
		payloadContains = json.RawMessage(pc)
	}

	var executionMode *domain.ExecutionMode
	if em := query.Get("execution_mode"); em != "" {
		parsed := domain.ExecutionMode(em)
		if !parsed.IsValid() {
			respondError(w, r, http.StatusBadRequest, "execution_mode is invalid")
			return
		}
		executionMode = &parsed
	}

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	var runs []domain.JobRun
	if tagKey != "" {
		runs, err = s.store.ListRunsByTag(r.Context(), projectID, tagKey, tagValue, limit+1, cursor)
	} else {
		runs, err = s.store.ListRunsByProject(r.Context(), projectID, status, metadataKey, metadataValue, triggeredBy, batchID, payloadContains, executionMode, limit+1, cursor)
	}
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list runs")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(runs, limit, func(run domain.JobRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}

	if run.Status.IsTerminal() {
		respondError(w, r, http.StatusBadRequest, "run already in terminal state")
		return
	}

	if err := s.store.UpdateRunStatus(r.Context(), run.ID, run.Status, domain.StatusCanceled, map[string]any{
		"finished_at": time.Now(),
		"error":       "canceled by user",
	}); err != nil {
		respondError(w, r, http.StatusConflict, "failed to cancel run")
		return
	}

	if s.workflowCallback != nil {
		canceledRun := *run
		canceledRun.Status = domain.StatusCanceled
		canceledRun.Error = "canceled by user"
		if cbErr := s.workflowCallback.OnJobRunTerminal(r.Context(), &canceledRun); cbErr != nil {
			slog.Error("workflow callback failed", "error", cbErr)
		}
	}

	// Stop managed container if running — use detached context so client
	// disconnect doesn't abort the stop, and cap at 10s to avoid blocking
	// if the Fly API is unresponsive.
	if s.containerRuntime != nil && run.ExecutionMode == domain.ExecutionModeManaged && run.MachineID != "" {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		if stopErr := s.containerRuntime.Stop(stopCtx, run.MachineID); stopErr != nil {
			slog.Warn("failed to stop managed container on cancel",
				"run_id", run.ID, "machine_id", run.MachineID, "error", stopErr)
		}
	}

	canceledCount := s.cancelChildRunsRecursive(r.Context(), run.ID)
	if canceledCount > 0 && s.metrics != nil {
		s.metrics.ChildCancellationsTotal.Add(r.Context(), canceledCount)
	}

	updatedRun, err := s.store.GetRun(r.Context(), run.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get updated run")
		return
	}

	respondJSON(w, http.StatusOK, updatedRun)
}

// maxCancelDepth limits recursive child cancellation to prevent runaway traversal.
const maxCancelDepth = 20

// cancelChildRunsRecursive uses CancelChildRunsByParentIDs to bulk-cancel
// children at each depth level, avoiding N+1 individual UpdateRunStatus calls.
func (s *Server) cancelChildRunsRecursive(ctx context.Context, parentRunID string) int64 {
	now := time.Now()
	parentIDs := []string{parentRunID}
	var total int64

	for depth := range maxCancelDepth {
		select {
		case <-ctx.Done():
			return total
		default:
		}

		if len(parentIDs) == 0 {
			break
		}

		canceled, err := s.store.CancelChildRunsByParentIDs(ctx, parentIDs, now, "parent run canceled")
		if err != nil {
			slog.Error("failed to bulk cancel child runs", "depth", depth, "parent_count", len(parentIDs), "error", err)
			break
		}
		if canceled == 0 {
			break
		}
		total += canceled

		// Collect IDs of the just-canceled children to recurse into their children.
		// We need to list them to get their IDs for the next depth level.
		nextParentIDs := make([]string, 0)
		for _, pid := range parentIDs {
			var cursor *time.Time
			for {
				children, listErr := s.store.ListChildRuns(ctx, pid, 100, cursor)
				if listErr != nil || len(children) == 0 {
					break
				}
				for _, child := range children {
					nextParentIDs = append(nextParentIDs, child.ID)
				}
				lastCreatedAt := children[len(children)-1].CreatedAt
				cursor = &lastCreatedAt
			}
		}
		parentIDs = nextParentIDs
	}

	return total
}

func (s *Server) handleGetRunDependencyStatus(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}

	deps, err := s.store.ListJobDependencies(r.Context(), run.JobID, 1000, nil)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list job dependencies")
		return
	}

	satisfied, err := s.store.AreJobDependenciesSatisfied(r.Context(), run)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to evaluate dependencies")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"run_id":                 run.ID,
		"job_id":                 run.JobID,
		"status":                 run.Status,
		"dependencies":           deps,
		"dependencies_satisfied": satisfied,
	})
}

func (s *Server) handleReplayRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	originalRun, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}

	if !isReplayableRunStatus(originalRun.Status) {
		respondError(w, r, http.StatusBadRequest, "run is not replayable")
		return
	}

	job, err := s.store.GetJob(r.Context(), originalRun.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			respondError(w, r, http.StatusBadRequest, "job not found for run")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get job")
		return
	}
	if !job.Enabled {
		respondError(w, r, http.StatusBadRequest, "job is disabled")
		return
	}

	payload := originalRun.Payload
	debugMode := false

	// Checkpoint-based replay: restore from a specific checkpoint sequence
	if fromCheckpointRaw := r.URL.Query().Get("from_checkpoint"); fromCheckpointRaw != "" {
		seq, parseErr := strconv.Atoi(fromCheckpointRaw)
		if parseErr != nil || seq <= 0 {
			respondError(w, r, http.StatusBadRequest, "from_checkpoint must be a positive integer")
			return
		}
		checkpoints, listErr := s.store.ListRunCheckpoints(r.Context(), runID, 1000, nil)
		if listErr != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to list checkpoints")
			return
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
			respondError(w, r, http.StatusNotFound, "checkpoint not found")
			return
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
		CreatedBy:    actorFromContext(r.Context()),
		ExpiresAt:    &expiresAt,
		DebugMode:    debugMode,
	}

	if err := s.queue.Enqueue(r.Context(), replayRun); err != nil {
		if errors.Is(err, domain.ErrIdempotencyConflict) {
			slog.Warn("replay idempotency conflict",
				"original_run_id", runID,
				"replay_run_id", replayRun.ID)
			respondError(w, r, http.StatusConflict, "idempotency key conflict: a run with this key is already active")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to enqueue replay run")
		return
	}

	respondJSON(w, http.StatusCreated, replayRun)
}

func (s *Server) handleListDeadLetterRuns(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	projectID := query.Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	runs, err := s.store.ListDeadLetterRuns(r.Context(), projectID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list dead letter runs")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(runs, limit, func(run domain.JobRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleReplayDeadLetterRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	run, err := s.store.ReplayDeadLetterRun(r.Context(), runID)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "not found"):
			respondError(w, r, http.StatusNotFound, "run not found")
		case strings.Contains(errMsg, "not dead_letter"):
			respondError(w, r, http.StatusConflict, "run is not dead_letter")
		default:
			respondError(w, r, http.StatusInternalServerError, "failed to replay dead letter run")
		}
		return
	}

	respondJSON(w, http.StatusOK, run)
}

func (s *Server) handleBulkReplayDeadLetterRuns(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RunIDs    []string `json:"run_ids"`
		ProjectID string   `json:"project_id"`
		Limit     int      `json:"limit"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	hasRunIDs := len(req.RunIDs) > 0
	hasProjectID := req.ProjectID != ""
	if hasRunIDs == hasProjectID {
		respondError(w, r, http.StatusBadRequest, APIError{
			Code:    ErrorCodeValidationError,
			Message: "provide either run_ids or project_id",
		})
		return
	}

	if hasRunIDs {
		if len(req.RunIDs) > 500 {
			respondError(w, r, http.StatusBadRequest, APIError{
				Code:    ErrorCodeValidationError,
				Message: "too many run_ids (max 500)",
			})
			return
		}
	} else {
		if req.Limit <= 0 {
			req.Limit = 100
		}
		if req.Limit > 500 {
			respondError(w, r, http.StatusBadRequest, APIError{
				Code:    ErrorCodeValidationError,
				Message: "limit must be <= 500",
			})
			return
		}
	}

	runs, err := s.store.BulkReplayDeadLetterRuns(r.Context(), req.RunIDs, req.ProjectID, req.Limit)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "at least one") || strings.Contains(errMsg, "provide either"):
			respondError(w, r, http.StatusBadRequest, APIError{Code: ErrorCodeValidationError, Message: errMsg})
		case strings.Contains(errMsg, "no dead_letter"):
			respondError(w, r, http.StatusConflict, APIError{Code: ErrorCodeConflict, Message: errMsg})
		default:
			respondError(w, r, http.StatusInternalServerError, "failed to bulk replay dead letter runs")
		}
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"replayed": runs, "count": len(runs)})
}

func isReplayableRunStatus(status domain.RunStatus) bool {
	switch status {
	case domain.StatusFailed, domain.StatusTimedOut, domain.StatusCrashed, domain.StatusSystemFailed:
		return true
	default:
		return false
	}
}

func (s *Server) handleListChildRuns(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	children, err := s.store.ListChildRuns(r.Context(), runID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list children")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(children, limit, func(run domain.JobRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleGetDebugBundle(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	bundle, err := s.store.GetDebugBundle(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get debug bundle")
		return
	}

	respondJSON(w, http.StatusOK, bundle)
}

func (s *Server) handleSetDebugMode(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	var req struct {
		DebugMode bool `json:"debug_mode"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.store.UpdateRunDebugMode(r.Context(), runID, req.DebugMode); err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to update debug mode")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListRunLineage(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	runs, err := s.store.ListRunLineage(r.Context(), runID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list run lineage")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(runs, limit, func(run domain.JobRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleResetIdempotencyKey(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	if err := s.store.ResetRunIdempotencyKey(r.Context(), runID); err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found or not eligible for idempotency reset")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to reset idempotency key")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "reset", "run_id": runID})
}

type RescheduleRunRequest struct {
	ScheduledAt time.Time       `json:"scheduled_at" validate:"required"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

func (s *Server) handleRescheduleRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	var req RescheduleRunRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}

	if err := s.store.RescheduleRun(r.Context(), runID, req.ScheduledAt, req.Payload); err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found or not eligible for rescheduling")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to reschedule run")
		return
	}

	updatedRun, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to fetch rescheduled run")
		return
	}

	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handleBulkReplayRuns(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RunIDs []string `json:"run_ids" validate:"required,min=1,max=100"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
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
		original, err := s.store.GetRun(r.Context(), runID)
		if err != nil {
			results = append(results, replayResult{OriginalRunID: runID, Status: "failed", Error: "run not found"})
			continue
		}
		if !isReplayableRunStatus(original.Status) {
			results = append(results, replayResult{OriginalRunID: runID, Status: "skipped", Error: "run is not replayable"})
			continue
		}

		job, err := s.store.GetJob(r.Context(), original.JobID)
		if err != nil || !job.Enabled {
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
			CreatedBy:    actorFromContext(r.Context()),
			ExpiresAt:    &expiresAt,
		}

		if err := s.queue.Enqueue(r.Context(), replayRun); err != nil {
			results = append(results, replayResult{OriginalRunID: runID, Status: "failed", Error: "enqueue failed"})
			continue
		}

		results = append(results, replayResult{OriginalRunID: runID, NewRunID: replayRun.ID, Status: "replayed"})
		replayed++
	}

	respondJSON(w, http.StatusOK, map[string]any{"results": results, "total": len(req.RunIDs), "replayed": replayed})
}

func (s *Server) handlePauseRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}

	if run.ExecutionMode != domain.ExecutionModeManaged {
		respondError(w, r, http.StatusBadRequest, "only managed runs can be paused")
		return
	}
	if run.Status == domain.StatusPaused {
		respondJSON(w, http.StatusOK, run)
		return
	}
	if run.Status != domain.StatusExecuting {
		respondError(w, r, http.StatusBadRequest, "run must be in executing state to pause")
		return
	}

	if err := s.store.UpdateRunStatus(r.Context(), run.ID, domain.StatusExecuting, domain.StatusPaused, map[string]any{
		"metadata": map[string]string{"_paused_machine_id": run.MachineID},
	}); err != nil {
		respondError(w, r, http.StatusConflict, "failed to pause run")
		return
	}

	// Stop managed container (non-fatal).
	if s.containerRuntime != nil && run.MachineID != "" {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		if stopErr := s.containerRuntime.Stop(stopCtx, run.MachineID); stopErr != nil {
			slog.Warn("failed to stop managed container on run pause",
				"run_id", run.ID, "machine_id", run.MachineID, "error", stopErr)
		}
	}

	updatedRun, err := s.store.GetRun(r.Context(), run.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get updated run")
		return
	}
	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handleResumeRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}

	if run.Status != domain.StatusPaused {
		respondError(w, r, http.StatusBadRequest, "run is not paused")
		return
	}

	if err := s.store.UpdateRunStatus(r.Context(), run.ID, domain.StatusPaused, domain.StatusQueued, map[string]any{
		"started_at":  nil,
		"finished_at": nil,
		"machine_id":  nil,
		"metadata":    map[string]string{},
	}); err != nil {
		respondError(w, r, http.StatusConflict, "failed to resume run")
		return
	}

	updatedRun, err := s.store.GetRun(r.Context(), run.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get updated run")
		return
	}
	respondJSON(w, http.StatusOK, updatedRun)
}

type restartRunRequest struct {
	MachinePreset string `json:"machine_preset,omitempty"`
}

func (s *Server) handleRestartRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}

	if run.ExecutionMode != domain.ExecutionModeManaged {
		respondError(w, r, http.StatusBadRequest, "only managed runs can be restarted")
		return
	}
	if run.Status != domain.StatusExecuting && run.Status != domain.StatusPaused {
		respondError(w, r, http.StatusBadRequest, "run must be executing or paused to restart")
		return
	}

	var req restartRunRequest
	if r.ContentLength > 0 {
		if err := s.decodeJSON(r, &req); err != nil {
			respondError(w, r, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	if req.MachinePreset != "" && !domain.MachinePreset(req.MachinePreset).IsValid() {
		respondError(w, r, http.StatusBadRequest, "invalid machine_preset")
		return
	}

	// Stop container if running.
	if s.containerRuntime != nil && run.MachineID != "" && run.Status == domain.StatusExecuting {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		if stopErr := s.containerRuntime.Stop(stopCtx, run.MachineID); stopErr != nil {
			slog.Warn("failed to stop managed container on restart",
				"run_id", run.ID, "machine_id", run.MachineID, "error", stopErr)
		}
	}

	metadata := map[string]string{}
	if req.MachinePreset != "" {
		metadata["_preset_override"] = req.MachinePreset
	}

	if err := s.store.UpdateRunStatus(r.Context(), run.ID, run.Status, domain.StatusQueued, map[string]any{
		"started_at":  nil,
		"finished_at": nil,
		"machine_id":  nil,
		"metadata":    metadata,
	}); err != nil {
		respondError(w, r, http.StatusConflict, "failed to restart run")
		return
	}

	updatedRun, err := s.store.GetRun(r.Context(), run.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get updated run")
		return
	}
	respondJSON(w, http.StatusOK, updatedRun)
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
