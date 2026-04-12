package scheduler

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// defaultAuditDLQReclaimBatch bounds a single reclaimer tick when no
// explicit batch is configured via WithAuditDLQReclaimBatch.
const defaultAuditDLQReclaimBatch = 200

// WithAuditRetention enables audit event retention enforcement.
// defaultDays is the server-wide default; a value <= 0 disables the task.
func (r *Reaper) WithAuditRetention(defaultDays int) *Reaper {
	r.auditRetentionDefaultDays = defaultDays
	return r
}

// WithAuditDLQReclaimBatch sets the maximum number of deadletter events
// the reclaimer processes per tick. Values <= 0 are ignored so the default
// remains in effect.
func (r *Reaper) WithAuditDLQReclaimBatch(n int) *Reaper {
	if n > 0 {
		r.auditDLQReclaimBatch = n
	}
	return r
}

// reapAuditEvents deletes audit events older than the configured retention
// window. Per-project overrides in project_quotas.audit_retention_days take
// precedence over the server-wide default. A project-level override of 0
// disables retention trimming for that project entirely.
//
// For projects without an override, a single sweep applies the default,
// skipping the projects that have an override row (whether positive or 0)
// so the per-project decision is final.
func (r *Reaper) reapAuditEvents(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.reapAuditEvents")
	defer span.End()

	if r.auditRetentionDefaultDays <= 0 {
		return
	}

	overrides, err := r.store.ListAuditRetentionOverrides(ctx)
	if err != nil {
		r.logger.Error("failed to list audit retention overrides", "error", err)
		r.recordOperation(ctx, "audit_retention", "error")
		return
	}

	now := time.Now()
	excluded := make([]string, 0, len(overrides))
	for _, ov := range overrides {
		excluded = append(excluded, ov.ProjectID)
		if ov.Days <= 0 {
			// Disabled — skip trimming for this project.
			continue
		}
		cutoff := now.Add(-time.Duration(ov.Days) * 24 * time.Hour)
		deleted, err := r.store.DeleteAuditEventsBefore(ctx, ov.ProjectID, cutoff)
		if err != nil {
			r.logger.Error("failed to reap audit events for project",
				"project_id", ov.ProjectID, "error", err)
			r.recordOperation(ctx, "audit_retention", "error")
			continue
		}
		if deleted > 0 {
			r.logger.Info("reaped old audit events (override)",
				"project_id", ov.ProjectID, "deleted", deleted, "cutoff", cutoff)
			r.recordDeleted(ctx, "audit_events", deleted)
			r.recordAuditRetentionDeleted(ctx, ov.ProjectID, deleted)
		}
	}

	// Default sweep across every project without an override.
	defaultCutoff := now.Add(-time.Duration(r.auditRetentionDefaultDays) * 24 * time.Hour)
	deleted, err := r.store.DeleteAuditEventsBeforeExcluding(ctx, defaultCutoff, excluded)
	if err != nil {
		r.logger.Error("failed to reap audit events (default)", "error", err)
		r.recordOperation(ctx, "audit_retention", "error")
		return
	}
	if deleted > 0 {
		r.logger.Info("reaped old audit events", "deleted", deleted, "cutoff", defaultCutoff)
		r.recordDeleted(ctx, "audit_events", deleted)
		r.recordAuditRetentionDeleted(ctx, "", deleted)
	}
	r.recordOperation(ctx, "audit_retention", "ok")
}

// recordAuditRetentionDeleted increments the retention deletion counter with
// a project_id attribute (empty string identifies the default global sweep).
func (r *Reaper) recordAuditRetentionDeleted(ctx context.Context, projectID string, n int64) {
	if r.metrics == nil || n <= 0 {
		return
	}
	r.metrics.AuditRetentionDeleted.Add(ctx, n, metric.WithAttributes(
		attribute.String("project_id", projectID),
	))
}

// recordReclaimerSuccess increments the reclaimer success counter.
func (r *Reaper) recordReclaimerSuccess(ctx context.Context, n int) {
	if r.metrics == nil || n <= 0 {
		return
	}
	r.metrics.AuditReclaimerSuccess.Add(ctx, int64(n))
}

// recordReclaimerFailed increments the reclaimer failure counter with a reason label.
func (r *Reaper) recordReclaimerFailed(ctx context.Context, n int, reason string) {
	if r.metrics == nil || n <= 0 {
		return
	}
	r.metrics.AuditReclaimerFailed.Add(ctx, int64(n), metric.WithAttributes(
		attribute.String("reason", reason),
	))
}

// reclaimAuditDeadletter replays deadlettered audit events back into the primary
// audit_events table. The batch is bounded by auditDLQReclaimBatch so a large
// backlog is drained across multiple ticks rather than in one long transaction.
// Events that fail to reclaim are left for the next cycle.
func (r *Reaper) reclaimAuditDeadletter(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.reclaimAuditDeadletter")
	defer span.End()

	batch := r.auditDLQReclaimBatch
	if batch <= 0 {
		batch = defaultAuditDLQReclaimBatch
	}
	span.SetAttributes(attribute.Int("reclaim.batch_size", batch))

	events, ids, err := r.store.ListAuditEventsDeadletter(ctx, batch)
	if err != nil {
		r.logger.Error("failed to list audit deadletter", "error", err)
		r.recordOperation(ctx, "audit_dlq_reclaim", "error")
		r.recordReclaimerFailed(ctx, 1, "list_error")
		return
	}
	if len(events) == 0 {
		span.SetAttributes(
			attribute.Int("reclaim.succeeded", 0),
			attribute.Int("reclaim.failed", 0),
		)
		return
	}

	reclaimed := 0
	insertFailed := 0
	deleteFailed := 0
	for i, ev := range events {
		evCopy := ev
		if writeErr := r.store.CreateAuditEvent(ctx, &evCopy); writeErr != nil {
			r.logger.Warn("audit deadletter reclaim failed",
				"event_id", ids[i], "action", ev.Action, "error", writeErr)
			insertFailed++
			continue
		}
		if delErr := r.store.DeleteAuditEventDeadletter(ctx, ids[i]); delErr != nil {
			r.logger.Error("audit deadletter delete failed after reclaim",
				"event_id", ids[i], "error", delErr)
			deleteFailed++
			continue
		}
		reclaimed++
	}

	span.SetAttributes(
		attribute.Int("reclaim.succeeded", reclaimed),
		attribute.Int("reclaim.failed", insertFailed+deleteFailed),
	)

	if reclaimed > 0 {
		r.logger.Info("reclaimed audit deadletter events",
			"reclaimed", reclaimed, "total", len(events))
		r.recordDeleted(ctx, "audit_deadletter_reclaimed", int64(reclaimed))
		r.recordReclaimerSuccess(ctx, reclaimed)
	}
	if insertFailed > 0 {
		r.recordReclaimerFailed(ctx, insertFailed, "chain_insert_failed")
	}
	if deleteFailed > 0 {
		r.recordReclaimerFailed(ctx, deleteFailed, "dlq_delete_failed")
	}
	r.recordOperation(ctx, "audit_dlq_reclaim", "ok")
}
