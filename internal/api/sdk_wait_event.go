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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// SDKWaitForEventRequest is the payload for POST /sdk/v1/runs/{runID}/wait-for-event.
type SDKWaitForEventRequest struct {
	EventKey   string `json:"event_key" validate:"required"`
	TimeoutSec int    `json:"timeout_secs,omitempty"`
	NotifyURL  string `json:"notify_url,omitempty"`
}

// quotaExceededError signals that a per-project quota limit was hit inside a
// transaction, allowing the caller to distinguish quota failures from other errors.
type quotaExceededError struct{ max int }

func (e *quotaExceededError) Error() string {
	return fmt.Sprintf("quota exceeded: max %d active event triggers", e.max)
}

// handleSDKWaitForEvent pauses a run to wait for an external event.
// The run transitions to StatusWaiting and an event trigger row is created.
// When the event is received via POST /v1/events/{eventKey}/send, the run
// transitions back to StatusQueued and is re-dispatched with checkpoint data.
func (s *Server) handleSDKWaitForEvent(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)

	runID := chi.URLParam(r, "runID")

	var req SDKWaitForEventRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	if errMsg := validateEventKey(req.EventKey); errMsg != "" {
		respondError(w, r, http.StatusBadRequest, errMsg)
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
		NotifyURL:   req.NotifyURL,
	}

	var quotaErr *quotaExceededError
	if err := s.runInTx(r.Context(), func(txStore APIStore) error {
		if err := txStore.UpdateRunStatus(r.Context(), run.ID, domain.StatusExecuting, domain.StatusWaiting, nil); err != nil {
			return fmt.Errorf("update run status: %w", err)
		}

		quota, qErr := txStore.GetProjectQuota(r.Context(), run.ProjectID)
		if qErr == nil && quota != nil && quota.MaxActiveEventTriggers > 0 {
			active, cErr := txStore.CountActiveEventTriggersByProject(r.Context(), run.ProjectID)
			if cErr != nil {
				slog.Warn("failed to count active triggers for quota check", "project_id", run.ProjectID, "error", cErr)
			} else if active >= quota.MaxActiveEventTriggers {
				return &quotaExceededError{max: quota.MaxActiveEventTriggers}
			}
		}

		if err := txStore.CreateEventTrigger(r.Context(), trigger); err != nil {
			return fmt.Errorf("create event trigger: %w", err)
		}
		return nil
	}); err != nil {
		if errors.As(err, &quotaErr) {
			respondError(w, r, http.StatusTooManyRequests, fmt.Sprintf("project has reached maximum active event triggers (%d)", quotaErr.max))
			return
		}
		if errors.Is(err, store.ErrEventKeyConflict) {
			respondError(w, r, http.StatusConflict, "event key already in use")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to create event trigger")
		return
	}

	if s.metrics != nil {
		attrs := metric.WithAttributes(
			attribute.String("source_type", trigger.SourceType),
			attribute.String("project_id", trigger.ProjectID),
		)
		s.metrics.EventTriggersCreated.Add(r.Context(), 1, attrs)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":     "waiting",
		"event_key":  req.EventKey,
		"expires_at": expiresAt,
		"trigger_id": trigger.ID,
	})
}
