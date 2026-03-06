package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get run")
		return
	}

	respondJSON(w, http.StatusOK, run)
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	projectID := query.Get("project_id")
	if projectID == "" {
		respondError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	var status *domain.RunStatus
	if statusRaw := query.Get("status"); statusRaw != "" {
		parsed := domain.RunStatus(statusRaw)
		status = &parsed
	}

	metadataKeyRaw := query.Get("metadata_key")
	metadataValueRaw := query.Get("metadata_value")
	if metadataValueRaw != "" && metadataKeyRaw == "" {
		respondError(w, http.StatusBadRequest, "metadata_key is required when metadata_value is provided")
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

	limit := 50
	if limitRaw := query.Get("limit"); limitRaw != "" {
		parsedLimit, err := strconv.Atoi(limitRaw)
		if err != nil || parsedLimit <= 0 {
			respondError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsedLimit > 100 {
			parsedLimit = 100
		}
		limit = parsedLimit
	}

	var cursor *time.Time
	if cursorRaw := query.Get("cursor"); cursorRaw != "" {
		parsedCursor, err := time.Parse(time.RFC3339, cursorRaw)
		if err != nil {
			respondError(w, http.StatusBadRequest, "cursor must be an RFC3339 timestamp")
			return
		}
		cursor = &parsedCursor
	}

	runs, err := s.store.ListRunsByProject(r.Context(), projectID, status, metadataKey, metadataValue, limit, cursor)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}

	respondJSON(w, http.StatusOK, runs)
}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get run")
		return
	}

	if run.Status.IsTerminal() {
		respondError(w, http.StatusBadRequest, "run already in terminal state")
		return
	}

	if err := s.store.UpdateRunStatus(r.Context(), run.ID, run.Status, domain.StatusCanceled, map[string]any{
		"finished_at": time.Now(),
		"error":       "canceled by user",
	}); err != nil {
		respondError(w, http.StatusConflict, "failed to cancel run")
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

	// Propagate cancellation to child runs
	children, err := s.store.ListChildRuns(r.Context(), run.ID)
	if err == nil {
		for _, child := range children {
			if !child.Status.IsTerminal() {
				if err := s.store.UpdateRunStatus(r.Context(), child.ID, child.Status, domain.StatusCanceled, map[string]any{
					"finished_at": time.Now(),
					"error":       "parent run canceled",
				}); err != nil {
					slog.Error("failed to cancel child run", "child_run_id", child.ID, "error", err) //nolint:gosec // structured logging sanitizes values
				}
			}
		}
	}

	updatedRun, err := s.store.GetRun(r.Context(), run.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get updated run")
		return
	}

	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handleReplayRun(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFRunReplay {
		respondError(w, http.StatusNotFound, "run replay is not enabled")
		return
	}
	runID := chi.URLParam(r, "runID")
	originalRun, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get run")
		return
	}

	if !isReplayableRunStatus(originalRun.Status) {
		respondError(w, http.StatusBadRequest, "run is not replayable")
		return
	}

	job, err := s.store.GetJob(r.Context(), originalRun.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			respondError(w, http.StatusBadRequest, "job not found for run")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get job")
		return
	}
	if !job.Enabled {
		respondError(w, http.StatusBadRequest, "job is disabled")
		return
	}

	payload := originalRun.Payload
	debugMode := false

	// Checkpoint-based replay: restore from a specific checkpoint sequence
	if fromCheckpointRaw := r.URL.Query().Get("from_checkpoint"); fromCheckpointRaw != "" {
		seq, parseErr := strconv.Atoi(fromCheckpointRaw)
		if parseErr != nil || seq <= 0 {
			respondError(w, http.StatusBadRequest, "from_checkpoint must be a positive integer")
			return
		}
		checkpoints, listErr := s.store.ListRunCheckpoints(r.Context(), runID, 1000)
		if listErr != nil {
			respondError(w, http.StatusInternalServerError, "failed to list checkpoints")
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
			respondError(w, http.StatusNotFound, "checkpoint not found")
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
		JobID:          originalRun.JobID,
		ProjectID:      originalRun.ProjectID,
		Attempt:        1,
		Payload:        payload,
		TriggeredBy:    domain.TriggerManual,
		Priority:       originalRun.Priority,
		IdempotencyKey: originalRun.IdempotencyKey,
		JobVersion:     originalRun.JobVersion,
		ExpiresAt:      &expiresAt,
		DebugMode:      debugMode,
	}

	if err := s.queue.Enqueue(r.Context(), replayRun); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to enqueue replay run")
		return
	}

	respondJSON(w, http.StatusCreated, replayRun)
}

func (s *Server) handleListDeadLetterRuns(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFRunDLQ {
		respondError(w, http.StatusNotFound, "not found")
		return
	}

	query := r.URL.Query()
	projectID := query.Get("project_id")
	if projectID == "" {
		respondError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	limit := 50
	if limitRaw := query.Get("limit"); limitRaw != "" {
		parsedLimit, err := strconv.Atoi(limitRaw)
		if err != nil || parsedLimit <= 0 {
			respondError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsedLimit > 1000 {
			parsedLimit = 1000
		}
		limit = parsedLimit
	}

	runs, err := s.store.ListDeadLetterRuns(r.Context(), projectID, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list dead letter runs")
		return
	}

	respondJSON(w, http.StatusOK, runs)
}

func (s *Server) handleReplayDeadLetterRun(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFRunDLQ {
		respondError(w, http.StatusNotFound, "not found")
		return
	}

	runID := chi.URLParam(r, "runID")
	run, err := s.store.ReplayDeadLetterRun(r.Context(), runID)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "not found"):
			respondError(w, http.StatusNotFound, "run not found")
		case strings.Contains(errMsg, "not dead_letter"):
			respondError(w, http.StatusConflict, "run is not dead_letter")
		default:
			respondError(w, http.StatusInternalServerError, "failed to replay dead letter run")
		}
		return
	}

	respondJSON(w, http.StatusOK, run)
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
	children, err := s.store.ListChildRuns(r.Context(), runID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list children")
		return
	}
	respondJSON(w, http.StatusOK, children)
}

func (s *Server) handleGetDebugBundle(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFDebugBundle {
		respondError(w, http.StatusNotFound, "not found")
		return
	}

	runID := chi.URLParam(r, "runID")
	bundle, err := s.store.GetDebugBundle(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get debug bundle")
		return
	}

	respondJSON(w, http.StatusOK, bundle)
}

func (s *Server) handleSetDebugMode(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFDebugBundle {
		respondError(w, http.StatusNotFound, "not found")
		return
	}

	runID := chi.URLParam(r, "runID")

	var req struct {
		DebugMode bool `json:"debug_mode"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.store.UpdateRunDebugMode(r.Context(), runID, req.DebugMode); err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to update debug mode")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
