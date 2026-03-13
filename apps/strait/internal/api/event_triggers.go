package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// SendEventRequest is the payload for POST /v1/events/{eventKey}/send.
type SendEventRequest struct {
	Payload json.RawMessage `json:"payload,omitempty"`
}

// validateEventKey returns an error string if the key is invalid, empty string if OK.
func validateEventKey(key string) string {
	if len(key) == 0 {
		return "event key is required"
	}
	if len(key) > 512 {
		return "event key must be at most 512 characters"
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 0x20 { // control characters including \x00
			return "event key contains invalid characters (control characters not allowed)"
		}
	}
	return ""
}

// handleSendEvent delivers an event to a waiting event trigger.
func (s *Server) handleSendEvent(w http.ResponseWriter, r *http.Request) {
	eventKey := chi.URLParam(r, "eventKey")
	if errMsg := validateEventKey(eventKey); errMsg != "" {
		respondError(w, r, http.StatusBadRequest, errMsg)
		return
	}

	var req SendEventRequest
	// Decode body if present. ContentLength == 0 means explicitly empty;
	// ContentLength == -1 means unknown (chunked), which we should still try.
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.decodeJSON(r, &req); err != nil {
			respondError(w, r, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	trigger, err := s.store.GetEventTriggerByEventKey(r.Context(), eventKey)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get event trigger")
		return
	}
	if trigger == nil {
		respondError(w, r, http.StatusNotFound, "event trigger not found")
		return
	}

	// Scope to project when authenticated via API key (not internal secret).
	if projectID := projectIDFromContext(r.Context()); projectID != "" && trigger.ProjectID != projectID {
		respondError(w, r, http.StatusForbidden, "event trigger does not belong to this project")
		return
	}

	if trigger.Status != domain.EventTriggerStatusWaiting {
		// Idempotent re-send: if already received with the same payload, return 200.
		if trigger.Status == domain.EventTriggerStatusReceived && payloadsMatch(trigger.ResponsePayload, req.Payload) {
			respondJSON(w, http.StatusOK, trigger)
			return
		}
		respondError(w, r, http.StatusConflict, "event trigger is not in waiting state")
		return
	}

	now := time.Now()

	// For job_run sources, use atomic receive+requeue to prevent inconsistency.
	// For workflow steps, update trigger then call callback (multi-step progression
	// is inherently non-atomic; the reconciliation reaper is the safety net).
	if trigger.SourceType == domain.EventSourceJobRun && trigger.JobRunID != "" {
		if err := s.store.ReceiveEventAndRequeueRun(r.Context(), trigger.ID, req.Payload, now, trigger.JobRunID); err != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to receive event")
			return
		}
	} else if trigger.SourceType == domain.EventSourceWorkflowStep && trigger.WorkflowStepRunID != "" {
		if err := s.runInTx(r.Context(), func(txStore APIStore) error {
			if err := txStore.UpdateEventTriggerStatus(r.Context(), trigger.ID, domain.EventTriggerStatusReceived, req.Payload, &now, ""); err != nil {
				return fmt.Errorf("update event trigger status: %w", err)
			}
			return txStore.UpdateStepRunStatus(r.Context(), trigger.WorkflowStepRunID, domain.StepCompleted, map[string]any{
				"output":      req.Payload,
				"finished_at": now,
			})
		}); err != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to receive event")
			return
		}

		// Drive workflow progression outside the transaction.
		trigger.Status = domain.EventTriggerStatusReceived
		trigger.ResponsePayload = req.Payload
		trigger.ReceivedAt = &now
		if trigger.WorkflowRunID != "" && s.workflowCallback != nil {
			if err := s.workflowCallback.OnEventReceived(r.Context(), trigger); err != nil {
				slog.Error("event received but failed to resume workflow",
					"event_key", eventKey, "trigger_id", trigger.ID, "error", err)
			}
		}
	} else {
		// Non-workflow-step source: update trigger status directly.
		if err := s.store.UpdateEventTriggerStatus(
			r.Context(), trigger.ID, domain.EventTriggerStatusReceived, req.Payload, &now, "",
		); err != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to update event trigger")
			return
		}

		trigger.Status = domain.EventTriggerStatusReceived
		trigger.ResponsePayload = req.Payload
		trigger.ReceivedAt = &now

		if err := s.resumeEventSource(r.Context(), trigger); err != nil {
			slog.Error("event received but failed to resume execution",
				"event_key", eventKey, "trigger_id", trigger.ID, "error", err)
		}
	}

	// Ensure in-memory trigger is up-to-date for the response (covers both branches).
	trigger.Status = domain.EventTriggerStatusReceived
	trigger.ResponsePayload = req.Payload
	trigger.ReceivedAt = &now

	// Record who sent the event (audit trail — non-fatal on error).
	sentBy := senderIdentity(r.Context())
	if err := s.store.SetEventTriggerSentBy(r.Context(), trigger.ID, sentBy); err != nil {
		slog.Warn("failed to set sent_by", "trigger_id", trigger.ID, "error", err)
	}
	trigger.SentBy = sentBy

	// Direct publish for sub-millisecond SSE delivery (CDC is the catch-all).
	s.publishTriggerStatusChange(r.Context(), trigger)

	// Record metrics.
	if s.metrics != nil {
		attrs := metric.WithAttributes(
			attribute.String("source_type", trigger.SourceType),
			attribute.String("project_id", trigger.ProjectID),
		)
		s.metrics.EventTriggersReceived.Add(r.Context(), 1, attrs)
		waitDuration := now.Sub(trigger.RequestedAt).Seconds()
		s.metrics.EventTriggerWaitDuration.Record(r.Context(), waitDuration, attrs)
	}

	respondJSON(w, http.StatusOK, trigger)
}

// resumeEventSource resumes the workflow step or job run that was waiting on the event.
func (s *Server) resumeEventSource(ctx context.Context, trigger *domain.EventTrigger) error {
	switch trigger.SourceType {
	case domain.EventSourceWorkflowStep:
		if trigger.WorkflowStepRunID == "" {
			return nil
		}
		// Trigger workflow progression via callback — OnEventReceived handles
		// both the step completion and fan-in/progression in one place.
		if s.workflowCallback != nil {
			return s.workflowCallback.OnEventReceived(ctx, trigger)
		}
		return nil

	case domain.EventSourceJobRun:
		if trigger.JobRunID == "" {
			return nil
		}
		// Re-queue the job run.
		if err := s.store.UpdateRunStatus(ctx, trigger.JobRunID, domain.StatusWaiting, domain.StatusQueued, map[string]any{
			"checkpoint_data": trigger.ResponsePayload,
		}); err != nil {
			return err
		}
		return nil
	}
	return nil
}

// handleGetEventTrigger returns a single event trigger by key.
func (s *Server) handleGetEventTrigger(w http.ResponseWriter, r *http.Request) {
	eventKey := chi.URLParam(r, "eventKey")
	if errMsg := validateEventKey(eventKey); errMsg != "" {
		respondError(w, r, http.StatusBadRequest, errMsg)
		return
	}

	trigger, err := s.store.GetEventTriggerByEventKey(r.Context(), eventKey)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get event trigger")
		return
	}
	if trigger == nil {
		respondError(w, r, http.StatusNotFound, "event trigger not found")
		return
	}

	// Scope to project when authenticated via API key (not internal secret).
	if projectID := projectIDFromContext(r.Context()); projectID != "" && trigger.ProjectID != projectID {
		respondError(w, r, http.StatusNotFound, "event trigger not found")
		return
	}

	respondJSON(w, http.StatusOK, trigger)
}

// handleCancelEventTrigger cancels a waiting event trigger by key.
func (s *Server) handleCancelEventTrigger(w http.ResponseWriter, r *http.Request) {
	eventKey := chi.URLParam(r, "eventKey")
	if errMsg := validateEventKey(eventKey); errMsg != "" {
		respondError(w, r, http.StatusBadRequest, errMsg)
		return
	}

	trigger, err := s.store.GetEventTriggerByEventKey(r.Context(), eventKey)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get event trigger")
		return
	}
	if trigger == nil {
		respondError(w, r, http.StatusNotFound, "event trigger not found")
		return
	}

	if projectID := projectIDFromContext(r.Context()); projectID != "" && trigger.ProjectID != projectID {
		respondError(w, r, http.StatusForbidden, "event trigger does not belong to this project")
		return
	}

	if trigger.Status != domain.EventTriggerStatusWaiting {
		respondError(w, r, http.StatusConflict, "event trigger is not in waiting state")
		return
	}

	now := time.Now()
	if err := s.store.UpdateEventTriggerStatus(
		r.Context(), trigger.ID, domain.EventTriggerStatusCanceled, nil, nil, "canceled by user",
	); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to cancel event trigger")
		return
	}

	trigger.Status = domain.EventTriggerStatusCanceled
	trigger.Error = "canceled by user"

	// Direct publish for sub-millisecond SSE delivery.
	s.publishTriggerStatusChange(r.Context(), trigger)

	// Drive step/run progression for the cancellation.
	switch trigger.SourceType {
	case domain.EventSourceWorkflowStep:
		if trigger.WorkflowStepRunID != "" {
			if stepErr := s.store.UpdateStepRunStatus(r.Context(), trigger.WorkflowStepRunID, domain.StepFailed, map[string]any{
				"finished_at": now,
				"error":       "event trigger canceled by user",
			}); stepErr != nil {
				slog.Error("failed to fail step after trigger cancel", "step_run_id", trigger.WorkflowStepRunID, "error", stepErr)
			} else if trigger.WorkflowRunID != "" && s.workflowCallback != nil {
				s.workflowCallback.OnStepFailed(r.Context(), trigger.WorkflowRunID, trigger.WorkflowStepRunID)
			}
		}
	case domain.EventSourceJobRun:
		if trigger.JobRunID != "" {
			if runErr := s.store.UpdateRunStatus(r.Context(), trigger.JobRunID, domain.StatusWaiting, domain.StatusCanceled, nil); runErr != nil {
				slog.Error("failed to cancel job run after trigger cancel", "job_run_id", trigger.JobRunID, "error", runErr)
			}
		}
	}

	if s.metrics != nil {
		attrs := metric.WithAttributes(
			attribute.String("source_type", trigger.SourceType),
			attribute.String("project_id", trigger.ProjectID),
		)
		s.metrics.EventTriggersTimedOut.Add(r.Context(), 1, attrs)
	}

	respondJSON(w, http.StatusOK, trigger)
}

// handleListEventTriggers lists event triggers for the authenticated project.
func (s *Server) handleListEventTriggers(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project context is required — authenticate with an API key")
		return
	}

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	status := r.URL.Query().Get("status")
	workflowRunID := r.URL.Query().Get("workflow_run_id")
	sourceType := r.URL.Query().Get("source_type")
	triggers, err := s.store.ListEventTriggersByProject(r.Context(), projectID, status, workflowRunID, sourceType, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list event triggers")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(triggers, limit, func(t domain.EventTrigger) string {
		return t.RequestedAt.Format(time.RFC3339Nano)
	}))
}

// payloadsMatch compares two JSON payloads for semantic equality.
func payloadsMatch(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	// Fast path: byte-equal payloads are always semantically equal.
	if bytes.Equal(a, b) {
		return true
	}
	// Slow path: normalize via round-trip for semantic comparison.
	var va, vb any
	if err := json.Unmarshal(a, &va); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		return false
	}
	ea, err := json.Marshal(va)
	if err != nil {
		return false
	}
	eb, err := json.Marshal(vb)
	if err != nil {
		return false
	}
	return bytes.Equal(ea, eb)
}

// handleGetEventTriggerStats returns aggregate statistics for event triggers.
func (s *Server) handleGetEventTriggerStats(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project context is required — authenticate with an API key")
		return
	}

	stats, err := s.store.GetEventTriggerStats(r.Context(), projectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get event trigger stats")
		return
	}

	respondJSON(w, http.StatusOK, stats)
}

// senderIdentity returns a string identifying who is making the current request.
func senderIdentity(ctx context.Context) string {
	if pid := projectIDFromContext(ctx); pid != "" {
		return "api-key:" + pid
	}
	return "internal"
}

// handleSendEventByPrefix delivers an event to all waiting triggers matching a prefix.
func (s *Server) handleSendEventByPrefix(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	prefix := chi.URLParam(r, "prefix")
	if errMsg := validateEventKey(prefix); errMsg != "" {
		respondError(w, r, http.StatusBadRequest, errMsg)
		return
	}

	var req SendEventRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.decodeJSON(r, &req); err != nil {
			respondError(w, r, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	projectID := projectIDFromContext(ctx)

	triggers, err := s.store.ListEventTriggersByKeyPrefix(ctx, prefix, projectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list triggers by prefix")
		return
	}

	if len(triggers) == 0 {
		respondJSON(w, http.StatusOK, map[string]any{"resolved": 0, "triggers": []any{}})
		return
	}

	now := time.Now()
	sentBy := senderIdentity(ctx)

	// Collect trigger IDs for atomic batch update.
	triggerIDs := make([]string, len(triggers))
	triggerMap := make(map[string]*domain.EventTrigger, len(triggers))
	for i := range triggers {
		triggerIDs[i] = triggers[i].ID
		triggerMap[triggers[i].ID] = &triggers[i]
	}

	// Atomically mark all triggers as received in a single transaction.
	resolvedIDs, err := s.store.BatchReceiveEventTriggers(ctx, triggerIDs, req.Payload, now, sentBy)
	if err != nil {
		slog.Error("batch receive failed", "prefix", prefix, "error", err)
	}

	resolved := make([]domain.EventTrigger, 0, len(resolvedIDs))
	for _, id := range resolvedIDs {
		trigger := triggerMap[id]
		trigger.Status = domain.EventTriggerStatusReceived
		trigger.ReceivedAt = &now
		trigger.ResponsePayload = req.Payload
		trigger.SentBy = sentBy

		// Resume each source outside the transaction.
		if err := s.resumeEventSource(ctx, trigger); err != nil {
			slog.Error("failed to resume event source by prefix", "trigger_id", trigger.ID, "error", err)
		}

		// Direct publish for sub-millisecond SSE delivery.
		s.publishTriggerStatusChange(ctx, trigger)

		resolved = append(resolved, *trigger)
	}

	// Record metrics for each resolved trigger.
	if s.metrics != nil {
		for _, t := range resolved {
			attrs := metric.WithAttributes(
				attribute.String("source_type", t.SourceType),
				attribute.String("project_id", t.ProjectID),
			)
			s.metrics.EventTriggersReceived.Add(ctx, 1, attrs)
			waitDuration := now.Sub(t.RequestedAt).Seconds()
			s.metrics.EventTriggerWaitDuration.Record(ctx, waitDuration, attrs)
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"resolved": len(resolved),
		"triggers": resolved,
	})
}

// handlePurgeEventTriggers deletes terminal triggers older than N days.
func (s *Server) handlePurgeEventTriggers(w http.ResponseWriter, r *http.Request) {
	type purgeRequest struct {
		OlderThanDays int  `json:"older_than_days"`
		DryRun        bool `json:"dry_run"`
	}

	var req purgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.OlderThanDays < 1 {
		respondError(w, r, http.StatusBadRequest, "older_than_days must be >= 1")
		return
	}

	before := time.Now().Add(-time.Duration(req.OlderThanDays) * 24 * time.Hour)

	if req.DryRun {
		// For dry run, return a count estimate.
		count, err := s.store.CountEventTriggersFinishedBefore(r.Context(), before)
		if err != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to count triggers")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"dry_run": true, "would_delete": count})
		return
	}

	deleted, err := s.store.DeleteEventTriggersFinishedBefore(r.Context(), before, 10000)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to purge triggers")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}
