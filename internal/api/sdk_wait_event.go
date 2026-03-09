package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// SDKWaitForEventRequest is the payload for POST /sdk/v1/runs/{runID}/wait-for-event.
type SDKWaitForEventRequest struct {
	EventKey   string `json:"event_key" validate:"required"`
	TimeoutSec int    `json:"timeout_sec,omitempty"`
}

// handleSDKWaitForEvent pauses a run to wait for an external event.
// The run transitions to StatusWaiting and an event trigger row is created.
// When the event is received via POST /v1/events/{eventKey}/send, the run
// transitions back to StatusQueued and is re-dispatched with checkpoint data.
func (s *Server) handleSDKWaitForEvent(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)

	if !s.config.FFEventTriggers {
		respondError(w, r, http.StatusNotFound, "event triggers feature is not enabled")
		return
	}

	runID := chi.URLParam(r, "runID")

	var req SDKWaitForEventRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	if len(req.EventKey) > 512 {
		respondError(w, r, http.StatusBadRequest, "event_key must be at most 512 characters")
		return
	}

	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}
	if run == nil {
		respondError(w, r, http.StatusNotFound, "run not found")
		return
	}

	if run.Status != domain.StatusExecuting {
		respondError(w, r, http.StatusConflict, fmt.Sprintf("run must be executing to wait for event, current status: %s", run.Status))
		return
	}

	// Transition run to waiting.
	if err := s.store.UpdateRunStatus(r.Context(), run.ID, domain.StatusExecuting, domain.StatusWaiting, nil); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to update run status")
		return
	}

	timeoutSecs := req.TimeoutSec
	if timeoutSecs <= 0 {
		timeoutSecs = domain.DefaultEventTimeoutSecs
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(timeoutSecs) * time.Second)

	trigger := &domain.EventTrigger{
		ID:          uuid.Must(uuid.NewV7()).String(),
		EventKey:    req.EventKey,
		ProjectID:   run.ProjectID,
		SourceType:  domain.EventSourceJobRun,
		JobRunID:    run.ID,
		Status:      domain.EventTriggerStatusWaiting,
		TimeoutSecs: timeoutSecs,
		RequestedAt: now,
		ExpiresAt:   expiresAt,
	}

	if err := s.store.CreateEventTrigger(r.Context(), trigger); err != nil {
		// Rollback status. Note: there is a theoretical race window between the
		// executing→waiting transition and this rollback where the reaper could
		// act on the waiting run. The window is milliseconds vs the reaper's 10s+
		// interval, so the risk is negligible. Log failures rather than ignoring.
		if rbErr := s.store.UpdateRunStatus(r.Context(), run.ID, domain.StatusWaiting, domain.StatusExecuting, nil); rbErr != nil {
			slog.Warn("failed to rollback run status after trigger creation failure",
				"run_id", run.ID, "error", rbErr)
		}
		if errors.Is(err, store.ErrEventKeyConflict) {
			respondError(w, r, http.StatusConflict, "event key already in use")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to create event trigger")
		return
	}

	if s.metrics != nil {
		s.metrics.EventTriggersCreated.Add(r.Context(), 1)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":     "waiting",
		"event_key":  req.EventKey,
		"expires_at": expiresAt,
		"trigger_id": trigger.ID,
	})
}
