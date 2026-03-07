package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleSDKComplete(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	var req struct {
		Result json.RawMessage `json:"result,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	// Fetch current run to validate FSM transition dynamically
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}

	now := time.Now()
	fields := map[string]any{
		"finished_at": now,
	}
	if len(req.Result) > 0 {
		fields["result"] = req.Result
	}

	err = s.store.UpdateRunStatus(r.Context(), runID, run.Status, domain.StatusCompleted, fields)
	if err != nil {
		slog.Error("failed to complete run", "run_id", runID, "error", err)
		if errors.Is(err, store.ErrRunConflict) {
			respondError(w, r, http.StatusConflict, "run status conflict")
		} else {
			respondError(w, r, http.StatusInternalServerError, "failed to update run")
		}
		return
	}

	if s.workflowCallback != nil {
		completedRun := *run
		completedRun.Status = domain.StatusCompleted
		if cbErr := s.workflowCallback.OnJobRunTerminal(r.Context(), &completedRun); cbErr != nil {
			slog.Error("workflow callback failed", "run_id", runID, "error", cbErr)
		}
	}
	if err := s.resumeWaitingParentIfReady(r.Context(), run); err != nil {
		slog.Error("failed to resume waiting parent", "run_id", runID, "error", err)
	}

	if s.pubsub != nil {
		payload, _ := json.Marshal(map[string]any{
			"type":      "status_change",
			"run_id":    runID,
			"from":      string(run.Status),
			"to":        "completed",
			"timestamp": now.UTC(),
		})
		channel := fmt.Sprintf("run:%s", runID)
		if err := s.pubsub.Publish(r.Context(), channel, payload); err != nil {
			slog.Warn("failed to publish event", "run_id", runID, "error", err)
		}
	}

	updatedRun, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get updated run")
		return
	}
	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handleSDKFail(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	var req struct {
		Error string `json:"error" validate:"required"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	// Fetch current run to validate FSM transition dynamically
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}

	now := time.Now()
	err = s.store.UpdateRunStatus(r.Context(), runID, run.Status, domain.StatusFailed, map[string]any{
		"finished_at": now,
		"error":       req.Error,
	})
	if err != nil {
		slog.Error("failed to fail run", "run_id", runID, "error", err)
		if errors.Is(err, store.ErrRunConflict) {
			respondError(w, r, http.StatusConflict, "run status conflict")
		} else {
			respondError(w, r, http.StatusInternalServerError, "failed to update run")
		}
		return
	}

	if s.workflowCallback != nil {
		failedRun := *run
		failedRun.Status = domain.StatusFailed
		failedRun.Error = req.Error
		if cbErr := s.workflowCallback.OnJobRunTerminal(r.Context(), &failedRun); cbErr != nil {
			slog.Error("workflow callback failed", "run_id", runID, "error", cbErr)
		}
	}
	if err := s.resumeWaitingParentIfReady(r.Context(), run); err != nil {
		slog.Error("failed to resume waiting parent", "run_id", runID, "error", err)
	}

	if s.pubsub != nil {
		payload, _ := json.Marshal(map[string]any{
			"type":      "status_change",
			"run_id":    runID,
			"from":      string(run.Status),
			"to":        "failed",
			"error":     req.Error,
			"timestamp": now.UTC(),
		})
		channel := fmt.Sprintf("run:%s", runID)
		if err := s.pubsub.Publish(r.Context(), channel, payload); err != nil {
			slog.Warn("failed to publish event", "run_id", runID, "error", err)
		}
	}

	updatedRun, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get updated run")
		return
	}
	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handleSDKSpawn(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	parentRunID := chi.URLParam(r, "runID")

	var req struct {
		JobSlug   string          `json:"job_slug" validate:"required"`
		ProjectID string          `json:"project_id" validate:"required"`
		Payload   json.RawMessage `json:"payload,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	job, err := s.store.GetJobBySlug(r.Context(), req.ProjectID, req.JobSlug)
	if err != nil || job == nil {
		respondError(w, r, http.StatusNotFound, "job not found")
		return
	}

	parentRun, err := s.store.GetRun(r.Context(), parentRunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "parent run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get parent run")
		return
	}
	if parentRun == nil {
		respondError(w, r, http.StatusNotFound, "parent run not found")
		return
	}
	if parentRun.Status == domain.StatusExecuting {
		if err := s.store.UpdateRunStatus(r.Context(), parentRun.ID, domain.StatusExecuting, domain.StatusWaiting, map[string]any{}); err != nil {
			slog.Error("failed to transition parent run to waiting", "parent_run_id", parentRun.ID, "error", err)
		}
	}

	run := &domain.JobRun{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Payload:     req.Payload,
		TriggeredBy: domain.TriggerSpawn,
		ParentRunID: parentRunID,
	}

	if err := s.queue.Enqueue(r.Context(), run); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to enqueue child run")
		return
	}

	respondJSON(w, http.StatusCreated, run)
}

func (s *Server) handleSDKContinue(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	parentRunID := chi.URLParam(r, "runID")

	if !s.config.FFRunContinuation {
		respondError(w, r, http.StatusNotFound, "run continuation is not enabled")
		return
	}

	var req struct {
		Payload json.RawMessage `json:"payload,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	parentRun, err := s.store.GetRun(r.Context(), parentRunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}

	if parentRun.Status != domain.StatusExecuting && parentRun.Status != domain.StatusWaiting {
		respondError(w, r, http.StatusConflict, "run must be executing or waiting to continue")
		return
	}

	const maxLineageDepth = 10
	if parentRun.LineageDepth >= maxLineageDepth {
		respondError(w, r, http.StatusBadRequest, fmt.Sprintf("max lineage depth (%d) exceeded", maxLineageDepth))
		return
	}

	job, err := s.store.GetJob(r.Context(), parentRun.JobID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get job")
		return
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

	if err := s.queue.Enqueue(r.Context(), continuationRun); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to enqueue continuation run")
		return
	}

	respondJSON(w, http.StatusCreated, continuationRun)
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
