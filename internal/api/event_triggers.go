package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

// SendEventRequest is the payload for POST /v1/events/{eventKey}/send.
type SendEventRequest struct {
	Payload json.RawMessage `json:"payload,omitempty"`
}

// handleSendEvent delivers an event to a waiting event trigger.
func (s *Server) handleSendEvent(w http.ResponseWriter, r *http.Request) {
	eventKey := chi.URLParam(r, "eventKey")
	if eventKey == "" {
		respondError(w, r, http.StatusBadRequest, "event key is required")
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
	if err := s.store.UpdateEventTriggerStatus(
		r.Context(),
		trigger.ID,
		domain.EventTriggerStatusReceived,
		req.Payload,
		&now,
		"",
	); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to update event trigger")
		return
	}

	// Update the in-memory trigger before passing to resumeEventSource,
	// so the callback sees the correct payload and status.
	trigger.Status = domain.EventTriggerStatusReceived
	trigger.ResponsePayload = req.Payload
	trigger.ReceivedAt = &now

	// Resume the workflow step or job run that was waiting.
	if err := s.resumeEventSource(r.Context(), trigger); err != nil {
		respondError(w, r, http.StatusInternalServerError, "event received but failed to resume execution")
		return
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
	if eventKey == "" {
		respondError(w, r, http.StatusBadRequest, "event key is required")
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

	respondJSON(w, http.StatusOK, trigger)
}

// handleListEventTriggers lists event triggers for the authenticated project.
func (s *Server) handleListEventTriggers(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	status := r.URL.Query().Get("status")
	triggers, err := s.store.ListEventTriggersByProject(r.Context(), projectID, status, limit+1, cursor)
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
	var va, vb any
	if err := json.Unmarshal(a, &va); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		return false
	}
	ea, _ := json.Marshal(va)
	eb, _ := json.Marshal(vb)
	return string(ea) == string(eb)
}

// SendEventByPrefixRequest is the payload for POST /v1/events/prefix/{prefix}/send.
type SendEventByPrefixRequest struct {
	Payload json.RawMessage `json:"payload,omitempty"`
}

// handleSendEventByPrefix delivers an event to all waiting triggers matching a prefix.
func (s *Server) handleSendEventByPrefix(w http.ResponseWriter, r *http.Request) {
	prefix := chi.URLParam(r, "prefix")
	if prefix == "" {
		respondError(w, r, http.StatusBadRequest, "prefix is required")
		return
	}

	var req SendEventByPrefixRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, r, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	triggers, err := s.store.ListEventTriggersByKeyPrefix(r.Context(), prefix)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list triggers by prefix")
		return
	}

	if len(triggers) == 0 {
		respondJSON(w, http.StatusOK, map[string]any{"resolved": 0, "triggers": []any{}})
		return
	}

	now := time.Now()
	resolved := make([]domain.EventTrigger, 0, len(triggers))

	for _, trigger := range triggers {
		if err := s.store.UpdateEventTriggerStatus(r.Context(), trigger.ID, domain.EventTriggerStatusReceived, req.Payload, &now, ""); err != nil {
			slog.Error("failed to resolve trigger by prefix", "trigger_id", trigger.ID, "error", err)
			continue
		}
		trigger.Status = domain.EventTriggerStatusReceived
		trigger.ReceivedAt = &now

		// Resume the source.
		trigger.ResponsePayload = req.Payload
		if err := s.resumeEventSource(r.Context(), &trigger); err != nil {
			slog.Error("failed to resume event source by prefix", "trigger_id", trigger.ID, "error", err)
		}

		resolved = append(resolved, trigger)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"resolved": len(resolved),
		"triggers": resolved,
	})
}
