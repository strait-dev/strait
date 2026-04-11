package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"strait/internal/domain"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// auditAsyncBufferSize is the capacity of the buffered audit-event channel.
// Large enough to absorb bursts on the job trigger hot path; small enough to
// avoid unbounded memory growth if the DB stalls.
const auditAsyncBufferSize = 4096

// auditAsyncShutdownTimeout bounds how long Close() waits for the drainer
// to flush pending events before abandoning them.
const auditAsyncShutdownTimeout = 5 * time.Second

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
func (s *Server) drainAuditAsync() {
	defer close(s.auditAsyncDone)
	for ev := range s.auditAsyncCh {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := s.store.CreateAuditEvent(ctx, ev); err != nil {
			slog.Warn("failed to create async audit event",
				"action", ev.Action,
				"resource_type", ev.ResourceType,
				"resource_id", ev.ResourceID,
				"error", err)
			if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
				s.metrics.AuditEventsDropped.Add(ctx, 1,
					metric.WithAttributes(attribute.String("reason", "write_failed")))
			}
		} else if s.metrics != nil && s.metrics.AuditEventsEmitted != nil {
			s.metrics.AuditEventsEmitted.Add(ctx, 1,
				metric.WithAttributes(attribute.String("mode", "async")))
		}
		cancel()
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
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		slog.Warn("failed to marshal async audit event details",
			"action", action, "error", err)
		return
	}
	actorType, _ := ctx.Value(ctxActorTypeKey).(string)
	ev := &domain.AuditEvent{
		ProjectID:    projectIDFromContext(ctx),
		ActorID:      actorFromContext(ctx),
		ActorType:    actorType,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      detailsJSON,
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

