package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const maxEventTriggerPurgeDays = 36500

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

type eventTriggerMutation string

const (
	eventTriggerMutationSend   eventTriggerMutation = "send"
	eventTriggerMutationCancel eventTriggerMutation = "cancel"
)

func eventTriggerMutationScope(trigger *domain.EventTrigger, mutation eventTriggerMutation) string {
	if trigger.SourceType == domain.EventSourceWorkflowStep {
		if mutation == eventTriggerMutationCancel {
			return domain.ScopeWorkflowsWrite
		}
		return domain.ScopeWorkflowsTrigger
	}
	if mutation == eventTriggerMutationCancel {
		return domain.ScopeJobsWrite
	}
	return domain.ScopeJobsTrigger
}

func (s *Server) requireEventTriggerMutationPermission(ctx context.Context, trigger *domain.EventTrigger, mutation eventTriggerMutation) error {
	required := eventTriggerMutationScope(trigger, mutation)
	if s.hasProjectPermission(ctx, required) {
		return nil
	}
	return huma.Error403Forbidden("insufficient permissions: requires " + required)
}

type SendEventInput struct {
	EventKey string `path:"eventKey"`
	Body     SendEventRequest
}
type SendEventOutput struct {
	Body *domain.EventTrigger
}

// resolveEventTriggerByKey resolves an event trigger from a user-supplied event
// key, scoping the lookup to the caller's project whenever a project context is
// present. Event keys are unique per project (migration 000284 replaced the
// global UNIQUE(event_key) with UNIQUE(project_id, event_key)), so an unscoped
// lookup can non-deterministically resolve another tenant's row when two
// projects share a key. The unscoped lookup is reserved for the explicit
// projectless internal-caller path (internal management secret).
func (s *Server) resolveEventTriggerByKey(ctx context.Context, eventKey, projectID string) (*domain.EventTrigger, error) {
	if projectID != "" {
		return s.store.GetEventTriggerByEventKeyForProject(ctx, eventKey, projectID)
	}
	return s.store.GetEventTriggerByEventKey(ctx, eventKey)
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
	trigger, err := s.resolveEventTriggerByKey(ctx, eventKey, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get event trigger")
	}
	if trigger == nil {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if err := requireProjectMatch(ctx, trigger.ProjectID); err != nil {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if projectID == "" && isInternalCaller(ctx) {
		s.emitInternalSecretBypassAudit(ctx, "send_event.project_match", "handleSendEvent", "event_trigger", trigger.ID)
	}
	if err := requireEnvironmentMatch(ctx, trigger.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if err := s.requireEventTriggerMutationPermission(ctx, trigger, eventTriggerMutationSend); err != nil {
		return nil, err
	}
	if trigger.Status != domain.EventTriggerStatusWaiting {
		if trigger.Status == domain.EventTriggerStatusReceived && payloadsMatch(trigger.ResponsePayload, req.Payload) {
			return &SendEventOutput{Body: trigger}, nil
		}
		return nil, huma.Error409Conflict("event trigger is not in waiting state")
	}

	now := time.Now()
	if err := s.receiveEventTrigger(ctx, trigger, req.Payload, now, eventKey); err != nil {
		return nil, err
	}

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

func (s *Server) receiveEventTrigger(
	ctx context.Context,
	trigger *domain.EventTrigger,
	payload json.RawMessage,
	now time.Time,
	eventKey string,
) error {
	switch {
	case trigger.SourceType == domain.EventSourceJobRun && trigger.JobRunID != "":
		return s.receiveJobRunEventTrigger(ctx, trigger, payload, now)
	case trigger.SourceType == domain.EventSourceWorkflowStep && trigger.WorkflowStepRunID != "":
		return s.receiveWorkflowStepEventTrigger(ctx, trigger, payload, now, eventKey)
	default:
		return s.receiveStandaloneEventTrigger(ctx, trigger, payload, now, eventKey)
	}
}

func (s *Server) receiveJobRunEventTrigger(
	ctx context.Context,
	trigger *domain.EventTrigger,
	payload json.RawMessage,
	now time.Time,
) error {
	if err := s.store.ReceiveEventAndRequeueRun(ctx, trigger.ID, payload, now, trigger.JobRunID); err != nil {
		return receiveEventAPIError(err, "failed to receive event")
	}
	if err := s.enqueueExistingReadyRun(ctx, trigger.JobRunID); err != nil {
		return receiveEventAPIError(err, "failed to enqueue resumed run")
	}
	markEventTriggerReceived(trigger, payload, now)
	return nil
}

func (s *Server) enqueueExistingReadyRun(ctx context.Context, runID string) error {
	if runID == "" || s.queue == nil {
		return nil
	}
	_, ok := s.queue.(existingRunEnqueuer)
	if !ok {
		return nil
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("load resumed run: %w", err)
	}
	if run == nil {
		return nil
	}
	return s.enqueueExistingRunIfSupported(ctx, run)
}

func (s *Server) receiveWorkflowStepEventTrigger(
	ctx context.Context,
	trigger *domain.EventTrigger,
	payload json.RawMessage,
	now time.Time,
	eventKey string,
) error {
	err := s.runInTx(ctx, func(txStore APIStore) error {
		if err := txStore.UpdateEventTriggerStatusFrom(
			ctx,
			trigger.ID,
			domain.EventTriggerStatusWaiting,
			domain.EventTriggerStatusReceived,
			payload,
			&now,
			"",
		); err != nil {
			return fmt.Errorf("update event trigger status: %w", err)
		}
		return txStore.UpdateStepRunStatus(ctx, trigger.WorkflowStepRunID, domain.StepCompleted, map[string]any{
			"output":      payload,
			"finished_at": now,
		})
	})
	if err != nil {
		return receiveEventAPIError(err, "failed to receive event")
	}

	markEventTriggerReceived(trigger, payload, now)
	if trigger.WorkflowRunID == "" || s.workflowCallback == nil {
		return nil
	}
	if err := s.workflowCallback.OnEventReceived(ctx, trigger); err != nil {
		slog.Error("event received but failed to resume workflow", "event_key", eventKey, "trigger_id", trigger.ID, "error", err)
	}
	return nil
}

func (s *Server) receiveStandaloneEventTrigger(
	ctx context.Context,
	trigger *domain.EventTrigger,
	payload json.RawMessage,
	now time.Time,
	eventKey string,
) error {
	if err := s.store.UpdateEventTriggerStatusFrom(
		ctx,
		trigger.ID,
		domain.EventTriggerStatusWaiting,
		domain.EventTriggerStatusReceived,
		payload,
		&now,
		"",
	); err != nil {
		return receiveEventAPIError(err, "failed to update event trigger")
	}

	markEventTriggerReceived(trigger, payload, now)
	if err := s.resumeEventSource(ctx, trigger); err != nil {
		slog.Error("event received but failed to resume execution", "event_key", eventKey, "trigger_id", trigger.ID, "error", err)
	}
	return nil
}

func markEventTriggerReceived(trigger *domain.EventTrigger, payload json.RawMessage, now time.Time) {
	trigger.Status = domain.EventTriggerStatusReceived
	trigger.ResponsePayload = payload
	trigger.ReceivedAt = &now
}

func receiveEventAPIError(err error, fallback string) error {
	if errors.Is(err, store.ErrEventTriggerConflict) {
		return huma.Error409Conflict("event trigger is not in waiting state")
	}
	return huma.Error500InternalServerError(fallback)
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
	trigger, err := s.resolveEventTriggerByKey(ctx, input.EventKey, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get event trigger")
	}
	if trigger == nil {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if err := requireProjectMatch(ctx, trigger.ProjectID); err != nil {
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
	trigger, err := s.resolveEventTriggerByKey(ctx, input.EventKey, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get event trigger")
	}
	if trigger == nil {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if err := requireProjectMatch(ctx, trigger.ProjectID); err != nil {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if projectID == "" && isInternalCaller(ctx) {
		s.emitInternalSecretBypassAudit(ctx, "cancel_event_trigger.project_match", "handleCancelEventTrigger", "event_trigger", trigger.ID)
	}
	if err := requireEnvironmentMatch(ctx, trigger.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("event trigger not found")
	}
	if err := s.requireEventTriggerMutationPermission(ctx, trigger, eventTriggerMutationCancel); err != nil {
		return nil, err
	}
	if trigger.Status != domain.EventTriggerStatusWaiting {
		return nil, huma.Error409Conflict("event trigger is not in waiting state")
	}
	now := time.Now()
	if err := s.store.UpdateEventTriggerStatusFrom(ctx, trigger.ID, domain.EventTriggerStatusWaiting, domain.EventTriggerStatusCanceled, nil, nil, "canceled by user"); err != nil {
		if errors.Is(err, store.ErrEventTriggerConflict) {
			return nil, huma.Error409Conflict("event trigger is not in waiting state")
		}
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
	triggers, err := s.store.ListEventTriggersByProject(ctx, projectID, environmentIDFromContext(ctx), input.Status, input.WorkflowRunID, input.SourceType, limit+1, cursor)
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
	a = bytes.TrimSpace(a)
	b = bytes.TrimSpace(b)
	if bytes.Equal(a, b) {
		return true
	}
	if payloadHasJSONWhitespace(a) || payloadHasJSONWhitespace(b) {
		var compactA, compactB bytes.Buffer
		if err := json.Compact(&compactA, a); err == nil {
			if err := json.Compact(&compactB, b); err == nil && bytes.Equal(compactA.Bytes(), compactB.Bytes()) {
				return true
			}
		}
	}
	var va, vb any
	if err := json.Unmarshal(a, &va); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		return false
	}
	return jsonValuesEqual(va, vb)
}

func jsonValuesEqual(a, b any) bool {
	switch av := a.(type) {
	case nil:
		return b == nil
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonValuesEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for key, aValue := range av {
			bValue, ok := bv[key]
			if !ok || !jsonValuesEqual(aValue, bValue) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func payloadHasJSONWhitespace(payload []byte) bool {
	for _, c := range payload {
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			return true
		}
	}
	return false
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
	stats, err := s.store.GetEventTriggerStats(ctx, projectID, environmentIDFromContext(ctx))
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
		if err := s.requireEventTriggerMutationPermission(ctx, &triggers[i], eventTriggerMutationSend); err != nil {
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
	if req.OlderThanDays > maxEventTriggerPurgeDays {
		return nil, huma.Error400BadRequest(fmt.Sprintf("older_than_days must be <= %d", maxEventTriggerPurgeDays))
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required -- authenticate with an API key")
	}
	environmentID := environmentIDFromContext(ctx)
	before := time.Now().Add(-time.Duration(req.OlderThanDays) * 24 * time.Hour)
	if req.DryRun {
		count, err := s.store.CountEventTriggersFinishedBeforeForProject(ctx, projectID, environmentID, before)
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
	deleted, err := s.store.DeleteEventTriggersFinishedBeforeForProject(ctx, projectID, environmentID, before, 10000)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to purge triggers")
	}
	s.emitAuditEvent(ctx, domain.AuditActionEventTriggerPurged, "event_trigger", "", map[string]any{
		"deleted":         deleted,
		"older_than_days": req.OlderThanDays,
	})
	return &PurgeEventTriggersOutput{Body: map[string]any{"deleted": deleted}}, nil
}
