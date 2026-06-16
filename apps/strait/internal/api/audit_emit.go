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
const auditAsyncBufferSize = 16384

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
//
// drainCtx is a fresh cancellable context derived from Background. Per-event
// DB timeouts are derived from drainCtx so stopAuditAsyncDrain can cancel any
// straggling write after the queued backlog has been processed — bounding
// total shutdown time even when the per-event 10s timeout would otherwise
// dominate.
//
// The channel and done sentinel are assigned under auditAsyncMu so concurrent
// readers (e.g. the AuditDrainerQueueDepth observable gauge callback) cannot
// observe a torn pointer write.
func (s *Server) startAuditAsyncDrain() {
	bufSize := s.auditAsyncBufferSize
	if bufSize <= 0 {
		bufSize = auditAsyncBufferSize
	}
	ch := make(chan *domain.AuditEvent, bufSize)
	done := make(chan struct{})
	drainCtx, drainCancel := context.WithCancel(context.Background())
	s.auditAsyncMu.Lock()
	if s.drainCancel != nil {
		s.drainCancel()
	}
	s.auditAsyncCh = ch
	s.auditAsyncDone = done
	s.drainCtx = drainCtx
	s.drainCancel = drainCancel
	s.auditAsyncMu.Unlock()
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
	s.auditAsyncMu.RLock()
	ch := s.auditAsyncCh
	done := s.auditAsyncDone
	s.auditAsyncMu.RUnlock()
	defer close(done)
	for ev := range ch {
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
	//
	// Per-event ctx is derived from the long-lived s.drainCtx so a shutdown
	// cancellation propagates into any in-flight DB call. Without this the
	// 10s per-event timeout could outlast the 5s shutdown budget and force
	// stopAuditAsyncDrain to abandon work that is still pending in the DB.
	parent := s.drainContext()
	var lastErr error
	attempts := len(auditRetryDelays) + 1
	for attempt := range attempts {
		if attempt > 0 {
			t := time.NewTimer(auditRetryDelays[attempt-1])
			select {
			case <-parent.Done():
				t.Stop()
				return
			case <-t.C:
			}
		}
		ctx, cancel := context.WithTimeout(parent, 10*time.Second)
		err := s.store.CreateAuditEvent(ctx, ev)
		cancel()
		if err == nil {
			if attempt > 0 && s.metrics != nil && s.metrics.AuditRetryAttempts != nil {
				s.metrics.AuditRetryAttempts.Add(context.Background(), 1,
					metric.WithAttributes(
						attribute.Int("attempt", attempt),
						attribute.String("outcome", "success")))
			}
			if s.metrics != nil && s.metrics.AuditEventsEmitted != nil {
				s.metrics.AuditEventsEmitted.Add(context.Background(), 1,
					metric.WithAttributes(attribute.String("mode", "async")))
			}
			if s.siemDrain != nil {
				s.siemDrain.Enqueue(*ev)
			}
			return
		}
		lastErr = err
		if attempt > 0 && s.metrics != nil && s.metrics.AuditRetryAttempts != nil {
			s.metrics.AuditRetryAttempts.Add(context.Background(), 1,
				metric.WithAttributes(
					attribute.Int("attempt", attempt),
					attribute.String("outcome", "failed")))
		}
		slog.Warn("audit event write attempt failed",
			"action", ev.Action,
			"resource_type", ev.ResourceType,
			"resource_id", ev.ResourceID,
			"attempt", attempt+1,
			"error", err)
	}

	// All retries exhausted — spill to deadletter. The DLQ insert also
	// derives from the drain's parent context so a shutdown cancellation
	// reaches it even after the primary chain insert is abandoned.
	dlqCtx, dlqCancel := context.WithTimeout(parent, 10*time.Second)
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
			s.metrics.AuditEventsDropped.Add(context.Background(), 1,
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
		s.metrics.AuditEventsDeadlettered.Add(context.Background(), 1,
			metric.WithAttributes(attribute.String("reason", "write_failed")))
	}
}

// drainContext returns the long-lived parent context for per-event drainer
// operations. Falls back to context.Background() when startAuditAsyncDrain
// has not been invoked (test paths that bypass NewServer).
func (s *Server) drainContext() context.Context {
	s.auditAsyncMu.RLock()
	ctx := s.drainCtx
	s.auditAsyncMu.RUnlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// AuditDrainerQueueDepth returns the current number of buffered audit
// events pending drain. Satisfies telemetry.AuditDrainerStatsProvider so
// the observable gauge callback can report saturation without exposing
// channel internals to callers outside this package.
func (s *Server) AuditDrainerQueueDepth() int64 {
	s.auditAsyncMu.RLock()
	ch := s.auditAsyncCh
	s.auditAsyncMu.RUnlock()
	if ch == nil {
		return 0
	}
	return int64(len(ch))
}

// AuditDrainerQueueCapacity returns the buffer capacity of the async
// drainer channel (0 before startAuditAsyncDrain has been called).
func (s *Server) AuditDrainerQueueCapacity() int64 {
	s.auditAsyncMu.RLock()
	ch := s.auditAsyncCh
	s.auditAsyncMu.RUnlock()
	if ch == nil {
		return 0
	}
	return int64(cap(ch))
}

// stopAuditAsyncDrain shuts the async drainer down in three phases:
//  1. Close the input channel so no new events can be enqueued and the
//     drainer goroutine sees the close after consuming what is already
//     buffered.
//  2. Wait up to auditAsyncShutdownTimeout for the drainer to finish
//     processing the queued backlog.
//  3. Cancel drainCtx — this aborts any in-flight DB call (including the
//     10s per-event timeout). The drainer goroutine sees the cancelled
//     parent context, returns, and closes auditAsyncDone.
//
// The cancel happens regardless of whether the wait timed out; without it a
// stalled DB call could otherwise outlast Server.Close by up to 10s.
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
	s.auditAsyncMu.RLock()
	done := s.auditAsyncDone
	s.auditAsyncMu.RUnlock()
	if done == nil {
		return
	}
	timedOut := false
	shutdownTimer := time.NewTimer(auditAsyncShutdownTimeout)
	select {
	case <-done:
		shutdownTimer.Stop()
	case <-shutdownTimer.C:
		timedOut = true
		slog.Warn("audit async drain did not finish within shutdown timeout",
			"timeout", auditAsyncShutdownTimeout)
	}
	s.auditAsyncMu.RLock()
	cancel := s.drainCancel
	s.auditAsyncMu.RUnlock()
	if cancel != nil {
		cancel()
	}
	if timedOut {
		graceTimer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-done:
			graceTimer.Stop()
		case <-graceTimer.C:
			slog.Warn("audit drainer still alive after grace period, waiting for exit")
			<-done
		}
	}
}

// marshalAndCapDetails marshals details to JSON and replaces the payload
// with a truncation marker if it exceeds auditMaxDetailsBytes. This keeps
// the HMAC chain input bounded and prevents a misbehaving handler from
// ballooning the DB row.
func (s *Server) marshalAndCapDetails(ctx context.Context, action string, details map[string]any) (json.RawMessage, error) {
	// Scan for known secret shapes in value positions and redact before
	// marshalling. A handler that accidentally stuffs a Stripe key, JWT,
	// AWS key, bearer token, or PEM block into details would otherwise
	// land the raw value in audit_events.details (GIN-indexed) and be
	// forwarded verbatim to SIEM. Redacted values are replaced with
	// "[redacted:<shape>]" and a "_redacted" key lists the shapes so
	// auditors know redaction happened without ever seeing the original.
	if redacted, shapes := scanAndRedact(details); len(shapes) > 0 {
		if m, ok := redacted.(map[string]any); ok {
			details = m
		}
		details["_redacted"] = shapes
		if s.metrics != nil && s.metrics.AuditDetailsRedacted != nil {
			for _, shape := range shapes {
				s.metrics.AuditDetailsRedacted.Add(ctx, 1,
					metric.WithAttributes(attribute.String("shape", shape)))
			}
		}
		slog.Warn("audit event details had secret-shaped substrings redacted",
			"action", action,
			"shapes", shapes)
	}

	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return nil, err
	}
	if len(detailsJSON) <= auditMaxDetailsBytes {
		return detailsJSON, nil
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
		return nil, fmt.Errorf("marshal truncation marker: %w", mErr)
	}
	return out, nil
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

func (s *Server) auditEventFromContext(
	ctx context.Context,
	actorID string,
	actorType string,
	action string,
	resourceType string,
	resourceID string,
	details json.RawMessage,
) *domain.AuditEvent {
	return &domain.AuditEvent{
		ProjectID:     projectIDFromContext(ctx),
		ActorID:       actorID,
		ActorType:     actorType,
		Action:        action,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		Details:       details,
		RemoteIP:      remoteIPFromContext(ctx),
		UserAgent:     userAgentFromContext(ctx),
		RequestID:     requestIDFromContext(ctx),
		TraceID:       traceIDFromContext(ctx),
		SchemaVersion: domain.AuditEventSchemaVersionCurrent,
	}
}

func (s *Server) enqueueAuditEventAsync(
	ctx context.Context,
	ch chan *domain.AuditEvent,
	stopped bool,
	ev *domain.AuditEvent,
) {
	if stopped {
		if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
			s.metrics.AuditEventsDropped.Add(ctx, 1,
				metric.WithAttributes(attribute.String("reason", "stopped")))
		}
		return
	}

	if len(ch) > cap(ch)*3/4 {
		if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
			s.metrics.AuditEventsDropped.Add(ctx, 1,
				metric.WithAttributes(attribute.String("reason", "backpressure_degraded")))
		}
		syncCtx, syncCancel := context.WithTimeout(context.Background(), 5*time.Second)
		syncErr := s.store.CreateAuditEvent(syncCtx, ev)
		syncCancel()
		outcome := "success"
		if syncErr != nil {
			outcome = "failure"
			slog.Warn("backpressure sync audit write failed", "action", ev.Action, "error", syncErr)
		} else if s.metrics != nil && s.metrics.AuditEventsEmitted != nil {
			s.metrics.AuditEventsEmitted.Add(ctx, 1,
				metric.WithAttributes(attribute.String("mode", "sync_fallback")))
		}
		if s.metrics != nil && s.metrics.AuditEventsSyncFallback != nil {
			s.metrics.AuditEventsSyncFallback.Add(ctx, 1,
				metric.WithAttributes(attribute.String("outcome", outcome)))
		}
		return
	}

	// The channel may be closed between the snapshot above and the send
	// below if stopAuditAsyncDrain runs concurrently. Recover from the
	// resulting panic rather than adding a second lock acquisition on the
	// hot path.
	defer func() {
		if r := recover(); r != nil {
			if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
				s.metrics.AuditEventsDropped.Add(ctx, 1,
					metric.WithAttributes(attribute.String("reason", "channel_closed")))
			}
			slog.Warn("audit async channel closed during send, dropping event",
				"action", ev.Action,
				"resource_type", ev.ResourceType,
				"resource_id", ev.ResourceID)
		}
	}()
	select {
	case ch <- ev:
	default:
		if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
			s.metrics.AuditEventsDropped.Add(ctx, 1,
				metric.WithAttributes(attribute.String("reason", "buffer_full")))
		}
		slog.Warn("audit async buffer full, dropping event",
			"action", ev.Action,
			"resource_type", ev.ResourceType,
			"resource_id", ev.ResourceID)
	}
}

// emitAuditEventRawAsync is a narrow fast path for callers that can prove
// their details JSON is already valid, bounded, and free of user-supplied
// secret-bearing values. Generic handlers must use emitAuditEventAsync so
// marshalAndCapDetails can run redaction and size enforcement.
func (s *Server) emitAuditEventRawAsync(
	ctx context.Context,
	action string,
	resourceType string,
	resourceID string,
	detailsJSON json.RawMessage,
) bool {
	if len(detailsJSON) > auditMaxDetailsBytes {
		return false
	}
	if !domain.IsKnownAuditAction(action) {
		slog.Error("emitAuditEventRawAsync: unknown action rejected",
			"action", action, "resource_type", resourceType, "resource_id", resourceID)
		if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
			s.metrics.AuditEventsDropped.Add(ctx, 1,
				metric.WithAttributes(attribute.String("reason", "unknown_action")))
		}
		return true
	}
	s.auditAsyncMu.RLock()
	ch := s.auditAsyncCh
	stopped := s.auditAsyncStopped
	s.auditAsyncMu.RUnlock()

	if ch == nil {
		return false
	}
	actorID, actorType, ok := s.validateActorForEmit(ctx, action)
	if !ok {
		return true
	}
	ev := s.auditEventFromContext(ctx, actorID, actorType, action, resourceType, resourceID, detailsJSON)
	s.enqueueAuditEventAsync(ctx, ch, stopped, ev)
	return true
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
	s.auditAsyncMu.RLock()
	ch := s.auditAsyncCh
	stopped := s.auditAsyncStopped
	s.auditAsyncMu.RUnlock()

	if ch == nil {
		// Async drain not started (e.g. in tests that bypass NewServer).
		// Fall back to the synchronous path so the event is not lost.
		s.emitAuditEvent(ctx, action, resourceType, resourceID, details)
		return
	}
	actorID, actorType, ok := s.validateActorForEmit(ctx, action)
	if !ok {
		return
	}
	detailsJSON, err := s.marshalAndCapDetails(ctx, action, details)
	if err != nil {
		slog.Warn("failed to marshal async audit event details",
			"action", action, "error", err)
		return
	}
	ev := s.auditEventFromContext(ctx, actorID, actorType, action, resourceType, resourceID, detailsJSON)
	s.enqueueAuditEventAsync(ctx, ch, stopped, ev)
}
