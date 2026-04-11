package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"strait/internal/domain"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Alerting rules for this subsystem live in strait-dev/infra RUNBOOK.md
// §audit-emit-health. The in-code health probe that feeds those alerts
// is in internal/health/audit_probe.go.

// auditAsyncBufferSize is the capacity of the buffered audit-event channel.
// Large enough to absorb bursts on the job trigger hot path; small enough to
// avoid unbounded memory growth if the DB stalls.
const auditAsyncBufferSize = 4096

// auditAsyncShutdownTimeout bounds how long Close() waits for the drainer
// to flush pending events before abandoning them.
const auditAsyncShutdownTimeout = 5 * time.Second

// auditRetryDelays is the in-memory retry schedule for transient
// CreateAuditEvent failures. The total budget is ~1.25 seconds per event,
// which blocks the drainer long enough to absorb a brief DB blip without
// indefinitely stalling the channel. After all retries fail the event is
// spilled to the audit_events_deadletter table via the store.
//
// Overridable in tests via setAuditRetryDelaysForTest to avoid real-time
// sleeps in unit tests. Not exposed for production use.
var auditRetryDelays = []time.Duration{
	50 * time.Millisecond,
	200 * time.Millisecond,
	1 * time.Second,
}

// auditMaxDetailsBytes caps the marshaled details payload. Oversize
// payloads are replaced with a truncation marker before being handed to
// the chain writer — the HMAC canonical form stays bounded and the DB
// row size is predictable.
const auditMaxDetailsBytes = 16 * 1024

// startAuditAsyncDrain initializes the async audit-emit infrastructure:
// a buffered channel and a single background goroutine that writes events
// sequentially via store.CreateAuditEvent. Sequential drain preserves the
// HMAC chain ordering even though requests are processed concurrently.
func (s *Server) startAuditAsyncDrain() {
	s.auditAsyncCh = make(chan *domain.AuditEvent, auditAsyncBufferSize)
	s.auditAsyncDone = make(chan struct{})
	go s.drainAuditAsync()
}

// drainAuditAsync runs in a dedicated goroutine. It reads from auditAsyncCh
// and calls store.CreateAuditEvent for each event. It exits once the channel
// is closed and fully drained.
//
// Every event is processed inside a deferred recover so a panic in one
// event cannot kill the drainer and silently stop the audit log. Panics
// are logged with a stack trace and metered as dropped events.
func (s *Server) drainAuditAsync() {
	defer close(s.auditAsyncDone)
	for ev := range s.auditAsyncCh {
		s.processAuditAsyncEvent(ev)
	}
}

func (s *Server) processAuditAsyncEvent(ev *domain.AuditEvent) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("audit drainer panic recovered",
				"action", ev.Action,
				"resource_type", ev.ResourceType,
				"resource_id", ev.ResourceID,
				"panic", r,
				"stack", string(debug.Stack()))
			if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
				s.metrics.AuditEventsDropped.Add(context.Background(), 1,
					metric.WithAttributes(attribute.String("reason", "panic")))
			}
		}
	}()

	// Retry loop: N+1 attempts total (one initial + len(auditRetryDelays)
	// retries). Between attempts, sleep by the configured backoff. If all
	// attempts fail, spill to the deadletter table; if THAT also fails,
	// log the full event as a last-resort forensic record.
	var lastErr error
	attempts := len(auditRetryDelays) + 1
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			time.Sleep(auditRetryDelays[attempt-1])
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := s.store.CreateAuditEvent(ctx, ev)
		cancel()
		if err == nil {
			if s.metrics != nil && s.metrics.AuditEventsEmitted != nil {
				s.metrics.AuditEventsEmitted.Add(context.Background(), 1,
					metric.WithAttributes(attribute.String("mode", "async")))
			}
			return
		}
		lastErr = err
		slog.Warn("audit event write attempt failed",
			"action", ev.Action,
			"resource_type", ev.ResourceType,
			"resource_id", ev.ResourceID,
			"attempt", attempt+1,
			"error", err)
	}

	// All retries exhausted — spill to deadletter.
	dlqCtx, dlqCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dlqCancel()
	lastErrStr := ""
	if lastErr != nil {
		lastErrStr = lastErr.Error()
	}
	if dlqErr := s.store.CreateAuditEventDeadletter(dlqCtx, ev, lastErrStr, len(auditRetryDelays)); dlqErr != nil {
		slog.Error("audit event deadletter write ALSO failed — event lost",
			"action", ev.Action,
			"resource_type", ev.ResourceType,
			"resource_id", ev.ResourceID,
			"project_id", ev.ProjectID,
			"actor_id", ev.ActorID,
			"details", string(ev.Details),
			"primary_error", lastErrStr,
			"deadletter_error", dlqErr)
		if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
			s.metrics.AuditEventsDropped.Add(dlqCtx, 1,
				metric.WithAttributes(attribute.String("reason", "deadletter_failed")))
		}
		return
	}
	slog.Warn("audit event spilled to deadletter table",
		"action", ev.Action,
		"resource_type", ev.ResourceType,
		"resource_id", ev.ResourceID,
		"retry_count", len(auditRetryDelays),
		"primary_error", lastErrStr)
	if s.metrics != nil && s.metrics.AuditEventsDeadlettered != nil {
		s.metrics.AuditEventsDeadlettered.Add(dlqCtx, 1,
			metric.WithAttributes(attribute.String("reason", "write_failed")))
	}
}

// stopAuditAsyncDrain closes the channel and waits for the drainer to finish
// flushing pending events. Bounded by auditAsyncShutdownTimeout so a stalled
// DB cannot indefinitely block server shutdown.
func (s *Server) stopAuditAsyncDrain() {
	s.auditAsyncStopOnce.Do(func() {
		s.auditAsyncMu.Lock()
		s.auditAsyncStopped = true
		ch := s.auditAsyncCh
		s.auditAsyncMu.Unlock()
		if ch == nil {
			return
		}
		close(ch)
	})
	if s.auditAsyncDone == nil {
		return
	}
	select {
	case <-s.auditAsyncDone:
	case <-time.After(auditAsyncShutdownTimeout):
		slog.Warn("audit async drain did not finish within shutdown timeout",
			"timeout", auditAsyncShutdownTimeout)
	}
}

// marshalAndCapDetails marshals details to JSON and replaces the payload
// with a truncation marker if it exceeds auditMaxDetailsBytes. This keeps
// the HMAC chain input bounded and prevents a misbehaving handler from
// ballooning the DB row.
func (s *Server) marshalAndCapDetails(ctx context.Context, action string, details map[string]any) (json.RawMessage, bool, error) {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return nil, false, err
	}
	if len(detailsJSON) <= auditMaxDetailsBytes {
		return detailsJSON, false, nil
	}
	slog.Warn("audit event details exceeded size cap; truncating",
		"action", action,
		"original_bytes", len(detailsJSON),
		"cap_bytes", auditMaxDetailsBytes)
	if s.metrics != nil && s.metrics.AuditEventsTruncated != nil {
		s.metrics.AuditEventsTruncated.Add(ctx, 1,
			metric.WithAttributes(attribute.String("action", action)))
	}
	marker := map[string]any{
		"_truncated":     true,
		"original_bytes": len(detailsJSON),
		"cap_bytes":      auditMaxDetailsBytes,
		"action":         action,
	}
	out, mErr := json.Marshal(marker)
	if mErr != nil {
		return nil, true, fmt.Errorf("marshal truncation marker: %w", mErr)
	}
	return out, true, nil
}

// validateActorForEmit rejects events with no actor when the context
// asserts the caller is a user or api_key. A missing actor on a
// user/api_key request is almost always a middleware bug and ships
// unattributable events — for compliance, refuse them.
//
// An empty actor_type is treated as "internal / unknown" and is allowed
// through (internal-secret callers, schedulers, and background workers
// fall into this category and legitimately have no logical actor).
func (s *Server) validateActorForEmit(ctx context.Context, action string) (actorID, actorType string, ok bool) {
	actorID = actorFromContext(ctx)
	actorType, _ = ctx.Value(ctxActorTypeKey).(string)
	if actorID != "" {
		return actorID, actorType, true
	}
	// Empty actor is only acceptable when the caller is a trusted
	// internal path (empty type, "internal", "sse_token"). Explicit
	// "user" or "api_key" with empty actor ID is a middleware bug.
	switch actorType {
	case "", "internal", "sse_token":
		return actorID, actorType, true
	}
	slog.Error("audit emit rejected: missing actor on authenticated request",
		"action", action,
		"actor_type", actorType)
	if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
		s.metrics.AuditEventsDropped.Add(ctx, 1,
			metric.WithAttributes(attribute.String("reason", "missing_actor")))
	}
	return "", "", false
}

// emitAuditEventAsync builds an audit event from the request context and
// hands it off to the background drainer. Safe to call on hot paths: the
// caller never blocks on the DB. If the buffer is full the event is dropped
// and a metric is incremented. Actor and project are snapshotted from ctx
// synchronously before returning so the detached worker does not observe a
// cancelled request context.
func (s *Server) emitAuditEventAsync(ctx context.Context, action, resourceType, resourceID string, details map[string]any) {
	if !domain.IsKnownAuditAction(action) {
		slog.Error("emitAuditEventAsync: unknown action rejected",
			"action", action, "resource_type", resourceType, "resource_id", resourceID)
		if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
			s.metrics.AuditEventsDropped.Add(ctx, 1,
				metric.WithAttributes(attribute.String("reason", "unknown_action")))
		}
		return
	}
	if s.auditAsyncCh == nil {
		// Async drain not started (e.g. in tests that bypass NewServer).
		// Fall back to the synchronous path so the event is not lost.
		s.emitAuditEvent(ctx, action, resourceType, resourceID, details)
		return
	}
	actorID, actorType, ok := s.validateActorForEmit(ctx, action)
	if !ok {
		return
	}
	detailsJSON, _, err := s.marshalAndCapDetails(ctx, action, details)
	if err != nil {
		slog.Warn("failed to marshal async audit event details",
			"action", action, "error", err)
		return
	}
	ev := &domain.AuditEvent{
		ProjectID:     projectIDFromContext(ctx),
		ActorID:       actorID,
		ActorType:     actorType,
		Action:        action,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		Details:       detailsJSON,
		RemoteIP:      remoteIPFromContext(ctx),
		UserAgent:     userAgentFromContext(ctx),
		RequestID:     requestIDFromContext(ctx),
		TraceID:       traceIDFromContext(ctx),
		SchemaVersion: domain.AuditEventSchemaVersionCurrent,
	}

	s.auditAsyncMu.RLock()
	stopped := s.auditAsyncStopped
	ch := s.auditAsyncCh
	s.auditAsyncMu.RUnlock()

	if stopped || ch == nil {
		if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
			s.metrics.AuditEventsDropped.Add(ctx, 1,
				metric.WithAttributes(attribute.String("reason", "stopped")))
		}
		return
	}

	select {
	case ch <- ev:
	default:
		if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
			s.metrics.AuditEventsDropped.Add(ctx, 1,
				metric.WithAttributes(attribute.String("reason", "buffer_full")))
		}
		slog.Warn("audit async buffer full, dropping event",
			"action", action,
			"resource_type", resourceType,
			"resource_id", resourceID)
	}
}
