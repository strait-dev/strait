package scheduler

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
)

// WithAuditRetention enables audit event retention enforcement.
// defaultDays is the server-wide default; a value <= 0 disables the task.
func (r *Reaper) WithAuditRetention(defaultDays int) *Reaper {
	r.auditRetentionDefaultDays = defaultDays
	return r
}

// reapAuditEvents deletes audit events older than the configured retention window.
// The projectID is intentionally left empty so the store deletes across all projects
// using the server-wide default.
func (r *Reaper) reapAuditEvents(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.reapAuditEvents")
	defer span.End()

	if r.auditRetentionDefaultDays <= 0 {
		return
	}

	cutoff := time.Now().Add(-time.Duration(r.auditRetentionDefaultDays) * 24 * time.Hour)
	deleted, err := r.store.DeleteAuditEventsBefore(ctx, "", cutoff)
	if err != nil {
		r.logger.Error("failed to reap audit events", "error", err)
		r.recordOperation(ctx, "audit_retention", "error")
		return
	}
	if deleted > 0 {
		r.logger.Info("reaped old audit events", "deleted", deleted, "cutoff", cutoff)
		r.recordDeleted(ctx, "audit_events", deleted)
	}
	r.recordOperation(ctx, "audit_retention", "ok")
}

// reclaimAuditDeadletter replays deadlettered audit events back into the primary
// audit_events table. Events that fail to reclaim are left for the next cycle.
func (r *Reaper) reclaimAuditDeadletter(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.reclaimAuditDeadletter")
	defer span.End()

	events, ids, err := r.store.ListAuditEventsDeadletter(ctx, 100)
	if err != nil {
		r.logger.Error("failed to list audit deadletter", "error", err)
		r.recordOperation(ctx, "audit_dlq_reclaim", "error")
		return
	}
	if len(events) == 0 {
		return
	}

	reclaimed := 0
	for i, ev := range events {
		evCopy := ev
		if writeErr := r.store.CreateAuditEvent(ctx, &evCopy); writeErr != nil {
			r.logger.Warn("audit deadletter reclaim failed",
				"event_id", ids[i], "action", ev.Action, "error", writeErr)
			continue
		}
		if delErr := r.store.DeleteAuditEventDeadletter(ctx, ids[i]); delErr != nil {
			r.logger.Error("audit deadletter delete failed after reclaim",
				"event_id", ids[i], "error", delErr)
			continue
		}
		reclaimed++
	}

	if reclaimed > 0 {
		r.logger.Info("reclaimed audit deadletter events",
			"reclaimed", reclaimed, "total", len(events))
		r.recordDeleted(ctx, "audit_deadletter_reclaimed", int64(reclaimed))
	}
	r.recordOperation(ctx, "audit_dlq_reclaim", "ok")
}
