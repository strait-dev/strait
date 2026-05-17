package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// SendEventRequest is the payload for POST /v1/events/{eventKey}/send.
type SendEventRequest struct {
	Payload json.RawMessage `json:"payload,omitempty"`
}

func validateEventKey(key string) string {
	if len(key) == 0 {
		return "event key is required"
	}
	if len(key) > 512 {
		return "event key must be at most 512 characters"
	}
	for i := range len(key) {
		if key[i] < 0x20 {
			return "event key contains invalid characters (control characters not allowed)"
		}
	}
	return ""
}

type SendEventInput struct {
	EventKey string `path:"eventKey"`
	Body     SendEventRequest
}
type SendEventOutput struct {
	Body *domain.EventTrigger
}

func (s *Server) handleSendEvent(ctx context.Context, input *SendEventInput) (*SendEventOutput, error) {
	eventKey := input.EventKey
	if errMsg := validateEventKey(eventKey); errMsg != "" {
		return nil, huma.Error400BadRequest(errMsg)
	}
	req := input.Body
	projectID := projectIDFromContext(ctx)
	if projectID == "" && !isInternalCaller(ctx) {
		return nil, huma.Error400BadRequest("project context is required -- authenticate with an API key")
	}
	trigger, err := s.store.GetEventTriggerByEventKey(ctx, eventKey)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get event trigger")
	}
	if trigger == nil {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if projectID != "" && trigger.ProjectID != projectID {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if projectID == "" && isInternalCaller(ctx) {
		s.emitInternalSecretBypassAudit(ctx, "send_event.project_match", "handleSendEvent", "event_trigger", trigger.ID)
	}
	if err := requireEnvironmentMatch(ctx, trigger.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if trigger.Status != domain.EventTriggerStatusWaiting {
		if trigger.Status == domain.EventTriggerStatusReceived && payloadsMatch(trigger.ResponsePayload, req.Payload) {
			return &SendEventOutput{Body: trigger}, nil
		}
		return nil, huma.Error409Conflict("event trigger is not in waiting state")
	}
	now := time.Now()
	if trigger.SourceType == domain.EventSourceJobRun && trigger.JobRunID != "" {
		if err := s.store.ReceiveEventAndRequeueRun(ctx, trigger.ID, req.Payload, now, trigger.JobRunID); err != nil {
			return nil, huma.Error500InternalServerError("failed to receive event")
		}
	} else if trigger.SourceType == domain.EventSourceWorkflowStep && trigger.WorkflowStepRunID != "" {
		if err := s.runInTx(ctx, func(txStore APIStore) error {
			if err := txStore.UpdateEventTriggerStatus(ctx, trigger.ID, domain.EventTriggerStatusReceived, req.Payload, &now, ""); err != nil {
				return fmt.Errorf("update event trigger status: %w", err)
			}
			return txStore.UpdateStepRunStatus(ctx, trigger.WorkflowStepRunID, domain.StepCompleted, map[string]any{
				"output": req.Payload, "finished_at": now,
			})
		}); err != nil {
			return nil, huma.Error500InternalServerError("failed to receive event")
		}
		trigger.Status = domain.EventTriggerStatusReceived
		trigger.ResponsePayload = req.Payload
		trigger.ReceivedAt = &now
		if trigger.WorkflowRunID != "" && s.workflowCallback != nil {
			if err := s.workflowCallback.OnEventReceived(ctx, trigger); err != nil {
				slog.Error("event received but failed to resume workflow", "event_key", eventKey, "trigger_id", trigger.ID, "error", err)
			}
		}
	} else {
		if err := s.store.UpdateEventTriggerStatus(ctx, trigger.ID, domain.EventTriggerStatusReceived, req.Payload, &now, ""); err != nil {
			return nil, huma.Error500InternalServerError("failed to update event trigger")
		}
		trigger.Status = domain.EventTriggerStatusReceived
		trigger.ResponsePayload = req.Payload
		trigger.ReceivedAt = &now
		if err := s.resumeEventSource(ctx, trigger); err != nil {
			slog.Error("event received but failed to resume execution", "event_key", eventKey, "trigger_id", trigger.ID, "error", err)
		}
	}
	trigger.Status = domain.EventTriggerStatusReceived
	trigger.ResponsePayload = req.Payload
	trigger.ReceivedAt = &now
	sentBy := senderIdentity(ctx)
	if err := s.store.SetEventTriggerSentBy(ctx, trigger.ID, sentBy); err != nil {
		slog.Warn("failed to set sent_by", "trigger_id", trigger.ID, "error", err)
	}
	trigger.SentBy = sentBy
	s.publishTriggerStatusChange(ctx, trigger)
	s.enqueueEventTriggerRecord(trigger, domain.EventTriggerStatusReceived)
	if s.metrics != nil {
		attrs := metric.WithAttributes(attribute.String("source_type", trigger.SourceType), attribute.String("project_id", trigger.ProjectID))
		s.metrics.EventTriggersReceived.Add(ctx, 1, attrs)
		s.metrics.EventTriggerWaitDuration.Record(ctx, now.Sub(trigger.RequestedAt).Seconds(), attrs)
	}
	s.emitAuditEvent(ctx, domain.AuditActionEventSent, "event_trigger", trigger.ID, map[string]any{
		"event_key":    eventKey,
		"source_type":  trigger.SourceType,
		"payload_size": len(req.Payload),
	})
	return &SendEventOutput{Body: trigger}, nil
}

func (s *Server) resumeEventSource(ctx context.Context, trigger *domain.EventTrigger) error {
	switch trigger.SourceType {
	case domain.EventSourceWorkflowStep:
		if trigger.WorkflowStepRunID == "" {
			return nil
		}
		if s.workflowCallback != nil {
			return s.workflowCallback.OnEventReceived(ctx, trigger)
		}
		return nil
	case domain.EventSourceJobRun:
		if trigger.JobRunID == "" {
			return nil
		}
		return s.store.UpdateRunStatus(ctx, trigger.JobRunID, domain.StatusWaiting, domain.StatusQueued, map[string]any{"checkpoint_data": trigger.ResponsePayload})
	}
	return nil
}

type GetEventTriggerInput struct {
	EventKey string `path:"eventKey"`
}
type GetEventTriggerOutput struct {
	Body *domain.EventTrigger
}

func (s *Server) handleGetEventTrigger(ctx context.Context, input *GetEventTriggerInput) (*GetEventTriggerOutput, error) {
	if errMsg := validateEventKey(input.EventKey); errMsg != "" {
		return nil, huma.Error400BadRequest(errMsg)
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" && !isInternalCaller(ctx) {
		return nil, huma.Error400BadRequest("project context is required -- authenticate with an API key")
	}
	trigger, err := s.store.GetEventTriggerByEventKey(ctx, input.EventKey)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get event trigger")
	}
	if trigger == nil {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if projectID != "" && trigger.ProjectID != projectID {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if projectID == "" && isInternalCaller(ctx) {
		s.emitInternalSecretBypassAudit(ctx, "get_event_trigger.project_match", "handleGetEventTrigger", "event_trigger", trigger.ID)
	}
	if err := requireEnvironmentMatch(ctx, trigger.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	return &GetEventTriggerOutput{Body: trigger}, nil
}

type CancelEventTriggerInput struct {
	EventKey string `path:"eventKey"`
}
type CancelEventTriggerOutput struct {
	Body *domain.EventTrigger
}

func (s *Server) handleCancelEventTrigger(ctx context.Context, input *CancelEventTriggerInput) (*CancelEventTriggerOutput, error) {
	if errMsg := validateEventKey(input.EventKey); errMsg != "" {
		return nil, huma.Error400BadRequest(errMsg)
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" && !isInternalCaller(ctx) {
		return nil, huma.Error400BadRequest("project context is required -- authenticate with an API key")
	}
	trigger, err := s.store.GetEventTriggerByEventKey(ctx, input.EventKey)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get event trigger")
	}
	if trigger == nil {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if projectID != "" && trigger.ProjectID != projectID {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if projectID == "" && isInternalCaller(ctx) {
		s.emitInternalSecretBypassAudit(ctx, "cancel_event_trigger.project_match", "handleCancelEventTrigger", "event_trigger", trigger.ID)
	}
	if err := requireEnvironmentMatch(ctx, trigger.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if trigger.Status != domain.EventTriggerStatusWaiting {
		return nil, huma.Error409Conflict("event trigger is not in waiting state")
	}
	now := time.Now()
	if err := s.store.UpdateEventTriggerStatus(ctx, trigger.ID, domain.EventTriggerStatusCanceled, nil, nil, "canceled by user"); err != nil {
		return nil, huma.Error500InternalServerError("failed to cancel event trigger")
	}
	trigger.Status = domain.EventTriggerStatusCanceled
	trigger.Error = "canceled by user"
	s.publishTriggerStatusChange(ctx, trigger)
	switch trigger.SourceType {
	case domain.EventSourceWorkflowStep:
		if trigger.WorkflowStepRunID != "" {
			if stepErr := s.store.UpdateStepRunStatus(ctx, trigger.WorkflowStepRunID, domain.StepFailed, map[string]any{"finished_at": now, "error": "event trigger canceled by user"}); stepErr != nil {
				slog.Error("failed to fail step after trigger cancel", "step_run_id", trigger.WorkflowStepRunID, "error", stepErr)
			} else if trigger.WorkflowRunID != "" && s.workflowCallback != nil {
				s.workflowCallback.OnStepFailed(ctx, trigger.WorkflowRunID, trigger.WorkflowStepRunID)
			}
		}
	case domain.EventSourceJobRun:
		if trigger.JobRunID != "" {
			if runErr := s.store.UpdateRunStatus(ctx, trigger.JobRunID, domain.StatusWaiting, domain.StatusCanceled, nil); runErr != nil {
				slog.Error("failed to cancel job run after trigger cancel", "job_run_id", trigger.JobRunID, "error", runErr)
			}
		}
	}
	if s.metrics != nil {
		attrs := metric.WithAttributes(attribute.String("source_type", trigger.SourceType), attribute.String("project_id", trigger.ProjectID))
		s.metrics.EventTriggersTimedOut.Add(ctx, 1, attrs)
	}
	s.emitAuditEvent(ctx, domain.AuditActionEventTriggerCancelled, "event_trigger", trigger.ID, map[string]any{
		"event_key":   input.EventKey,
		"source_type": trigger.SourceType,
	})
	return &CancelEventTriggerOutput{Body: trigger}, nil
}

type ListEventTriggersInput struct {
	Status        string `query:"status"`
	WorkflowRunID string `query:"workflow_run_id"`
	SourceType    string `query:"source_type"`
	Limit         string `query:"limit"`
	Cursor        string `query:"cursor"`
}
type ListEventTriggersOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListEventTriggers(ctx context.Context, input *ListEventTriggersInput) (*ListEventTriggersOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required -- authenticate with an API key")
	}
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	triggers, err := s.store.ListEventTriggersByProject(ctx, projectID, input.Status, input.WorkflowRunID, input.SourceType, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list event triggers")
	}
	return &ListEventTriggersOutput{Body: paginatedResult(triggers, limit, func(t domain.EventTrigger) string {
		return t.RequestedAt.Format(time.RFC3339Nano)
	})}, nil
}

func payloadsMatch(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	if bytes.Equal(a, b) {
		return true
	}
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

type GetEventTriggerStatsInput struct{}
type GetEventTriggerStatsOutput struct {
	Body any
}

func (s *Server) handleGetEventTriggerStats(ctx context.Context, _ *GetEventTriggerStatsInput) (*GetEventTriggerStatsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required -- authenticate with an API key")
	}
	stats, err := s.store.GetEventTriggerStats(ctx, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get event trigger stats")
	}
	return &GetEventTriggerStatsOutput{Body: stats}, nil
}

func (s *Server) enqueueEventTriggerRecord(trigger *domain.EventTrigger, status string) {
	if s.chExporter == nil || trigger == nil {
		return
	}
	var waitDurationMs uint64
	var receivedAt *time.Time
	if trigger.ReceivedAt != nil {
		t := *trigger.ReceivedAt
		receivedAt = &t
		waitDurationMs = uint64(max(trigger.ReceivedAt.Sub(trigger.RequestedAt).Milliseconds(), 0))
	}
	s.chExporter.Enqueue(clickhouse.EventTriggerEventRecord{
		TriggerID: trigger.ID, EventKey: trigger.EventKey, ProjectID: trigger.ProjectID,
		SourceType: trigger.SourceType, Status: status,
		TimeoutSecs:    uint32(max(trigger.TimeoutSecs, 0)), //nolint:gosec // timeout is always non-negative
		WaitDurationMs: waitDurationMs, CreatedAt: trigger.RequestedAt, ReceivedAt: receivedAt,
	})
}

func senderIdentity(ctx context.Context) string {
	if pid := projectIDFromContext(ctx); pid != "" {
		return "api-key:" + pid
	}
	return "internal"
}

type SendEventByPrefixInput struct {
	Prefix string `path:"prefix"`
	Body   SendEventRequest
}
type SendEventByPrefixOutput struct {
	Body map[string]any
}

func (s *Server) handleSendEventByPrefix(ctx context.Context, input *SendEventByPrefixInput) (*SendEventByPrefixOutput, error) {
	prefix := input.Prefix
	if errMsg := validateEventKey(prefix); errMsg != "" {
		return nil, huma.Error400BadRequest(errMsg)
	}
	req := input.Body
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required -- authenticate with an API key")
	}
	triggers, err := s.store.ListEventTriggersByKeyPrefix(ctx, prefix, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list triggers by prefix")
	}
	filtered := triggers[:0]
	for i := range triggers {
		if envErr := requireEnvironmentMatch(ctx, triggers[i].EnvironmentID); envErr != nil {
			continue
		}
		filtered = append(filtered, triggers[i])
	}
	triggers = filtered
	if len(triggers) == 0 {
		return &SendEventByPrefixOutput{Body: map[string]any{"resolved": 0, "triggers": []any{}}}, nil
	}
	now := time.Now()
	sentBy := senderIdentity(ctx)
	triggerIDs := make([]string, len(triggers))
	triggerMap := make(map[string]*domain.EventTrigger, len(triggers))
	for i := range triggers {
		triggerIDs[i] = triggers[i].ID
		triggerMap[triggers[i].ID] = &triggers[i]
	}
	resolvedIDs, err := s.store.BatchReceiveEventTriggers(ctx, triggerIDs, req.Payload, now, sentBy)
	if err != nil {
		slog.Error("batch receive failed", "prefix", prefix, "project_id", projectID, "trigger_count", len(triggerIDs), "error", err)
	}
	resolved := make([]domain.EventTrigger, 0, len(resolvedIDs))
	for _, id := range resolvedIDs {
		trigger := triggerMap[id]
		trigger.Status = domain.EventTriggerStatusReceived
		trigger.ReceivedAt = &now
		trigger.ResponsePayload = req.Payload
		trigger.SentBy = sentBy
		if err := s.resumeEventSource(ctx, trigger); err != nil {
			slog.Error("failed to resume event source by prefix", "trigger_id", trigger.ID, "project_id", trigger.ProjectID, "event_key", trigger.EventKey, "error", err)
		}
		s.publishTriggerStatusChange(ctx, trigger)
		resolved = append(resolved, *trigger)
	}
	for i := range resolved {
		s.enqueueEventTriggerRecord(&resolved[i], domain.EventTriggerStatusReceived)
	}
	if s.metrics != nil {
		for _, t := range resolved {
			attrs := metric.WithAttributes(attribute.String("source_type", t.SourceType), attribute.String("project_id", t.ProjectID))
			s.metrics.EventTriggersReceived.Add(ctx, 1, attrs)
			s.metrics.EventTriggerWaitDuration.Record(ctx, now.Sub(t.RequestedAt).Seconds(), attrs)
		}
	}
	s.emitAuditEvent(ctx, domain.AuditActionEventSentByPrefix, "event_trigger", "", map[string]any{
		"prefix":        prefix,
		"trigger_count": len(resolved),
		"payload_size":  len(req.Payload),
	})
	return &SendEventByPrefixOutput{Body: map[string]any{"resolved": len(resolved), "triggers": resolved}}, nil
}

type purgeEventTriggersRequest struct {
	OlderThanDays int  `json:"older_than_days"`
	DryRun        bool `json:"dry_run"`
}
type PurgeEventTriggersInput struct {
	Body purgeEventTriggersRequest
}
type PurgeEventTriggersOutput struct {
	Body map[string]any
}

func (s *Server) handlePurgeEventTriggers(ctx context.Context, input *PurgeEventTriggersInput) (*PurgeEventTriggersOutput, error) {
	req := input.Body
	if req.OlderThanDays < 1 {
		return nil, huma.Error400BadRequest("older_than_days must be >= 1")
	}
	before := time.Now().Add(-time.Duration(req.OlderThanDays) * 24 * time.Hour)
	if req.DryRun {
		count, err := s.store.CountEventTriggersFinishedBefore(ctx, before)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to count triggers")
		}
		s.emitAuditEvent(ctx, domain.AuditActionEventTriggerPurged, "event_trigger", "", map[string]any{
			"dry_run":         true,
			"would_delete":    count,
			"older_than_days": req.OlderThanDays,
		})
		return &PurgeEventTriggersOutput{Body: map[string]any{"dry_run": true, "would_delete": count}}, nil
	}
	deleted, err := s.store.DeleteEventTriggersFinishedBefore(ctx, before, 10000)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to purge triggers")
	}
	s.emitAuditEvent(ctx, domain.AuditActionEventTriggerPurged, "event_trigger", "", map[string]any{
		"deleted":         deleted,
		"older_than_days": req.OlderThanDays,
	})
	return &PurgeEventTriggersOutput{Body: map[string]any{"deleted": deleted}}, nil
}
