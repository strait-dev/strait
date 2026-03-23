package api

import (
	"context"
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

type SDKWaitForEventRequest struct {
	EventKey   string `json:"event_key" validate:"required"`
	TimeoutSec int    `json:"timeout_secs,omitempty"`
	NotifyURL  string `json:"notify_url,omitempty"`
}
type quotaExceededError struct{ max int }

func (e *quotaExceededError) Error() string {
	return fmt.Sprintf("quota exceeded: max %d active event triggers", e.max)
}

type SDKWaitForEventInput struct {
	RunID string `path:"runID"`
	Body  SDKWaitForEventRequest
}
type SDKWaitForEventOutput struct{ Body any }

func (s *Server) handleSDKWaitForEvent(ctx context.Context, input *SDKWaitForEventInput) (*SDKWaitForEventOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if errMsg := validateEventKey(req.EventKey); errMsg != "" {
		return nil, huma.Error400BadRequest(errMsg)
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get run")
	}
	if run == nil {
		return nil, huma.Error404NotFound("run not found")
	}
	if run.Status != domain.StatusExecuting {
		return nil, huma.Error409Conflict(fmt.Sprintf("run must be executing to wait for event, current status: %s", run.Status))
	}
	timeoutSecs := req.TimeoutSec
	if timeoutSecs <= 0 {
		timeoutSecs = domain.DefaultEventTimeoutSecs
	}
	now := time.Now()
	expiresAt := now.Add(time.Duration(timeoutSecs) * time.Second)
	trigger := &domain.EventTrigger{ID: uuid.Must(uuid.NewV7()).String(), EventKey: req.EventKey, ProjectID: run.ProjectID, SourceType: domain.EventSourceJobRun, JobRunID: run.ID, Status: domain.EventTriggerStatusWaiting, TimeoutSecs: timeoutSecs, RequestedAt: now, ExpiresAt: expiresAt, NotifyURL: req.NotifyURL}
	var quotaErr *quotaExceededError
	if err := s.runInTx(ctx, func(txStore APIStore) error {
		if err := txStore.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusWaiting, nil); err != nil {
			return fmt.Errorf("update run status: %w", err)
		}
		quota, qErr := txStore.GetProjectQuota(ctx, run.ProjectID)
		if qErr == nil && quota != nil && quota.MaxActiveEventTriggers > 0 {
			active, cErr := txStore.CountActiveEventTriggersByProject(ctx, run.ProjectID)
			if cErr != nil {
				slog.Warn("failed to count active triggers for quota check", "project_id", run.ProjectID, "error", cErr)
			} else if active >= quota.MaxActiveEventTriggers {
				return &quotaExceededError{max: quota.MaxActiveEventTriggers}
			}
		}
		if err := txStore.CreateEventTrigger(ctx, trigger); err != nil {
			return fmt.Errorf("create event trigger: %w", err)
		}
		return nil
	}); err != nil {
		if errors.As(err, &quotaErr) {
			return nil, huma.Error429TooManyRequests(fmt.Sprintf("project has reached maximum active event triggers (%d)", quotaErr.max))
		}
		if errors.Is(err, store.ErrEventKeyConflict) {
			return nil, huma.Error409Conflict("event key already in use")
		}
		return nil, huma.Error500InternalServerError("failed to create event trigger")
	}
	if s.metrics != nil {
		s.metrics.EventTriggersCreated.Add(ctx, 1, metric.WithAttributes(attribute.String("source_type", trigger.SourceType), attribute.String("project_id", trigger.ProjectID)))
	}
	return &SDKWaitForEventOutput{Body: map[string]any{"status": "waiting", "event_key": req.EventKey, "expires_at": expiresAt, "trigger_id": trigger.ID}}, nil
}
