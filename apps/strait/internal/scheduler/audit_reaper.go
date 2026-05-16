package scheduler

import (
	"context"
	"encoding/json"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// defaultAuditDLQReclaimBatch bounds a single reclaimer tick when no
// explicit batch is configured via WithAuditDLQReclaimBatch.
const defaultAuditDLQReclaimBatch = 200

// defaultAuditDLQMaxReclaimAttempts is the per-row reclaim attempt cap
// applied when WithAuditDLQMaxReclaimAttempts has not configured one.
// Beyond this many failed attempts, a row is skipped each tick and
// surfaced via the strait_audit_reclaimer_abandoned_total metric.
const defaultAuditDLQMaxReclaimAttempts = 10

// auditDLQPerEventReclaimTimeout bounds a single reclaim attempt so a
// wedged CreateAuditEvent (pgx connection hang, contended advisory
// lock, etc.) cannot stall the entire tick. The deadline is derived
// from the tick context so a shutdown cancellation still propagates
// through each per-event child context.
var auditDLQPerEventReclaimTimeout = 10 * time.Second

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

// WithAuditDLQMaxAgeDays enables the DLQ retention reaper. Rows older than
// the configured window (by original event created_at) are dropped each
// tick and surfaced via audit.deadletter_aged events. A value of 0 keeps
// the sweep disabled — the legacy behavior.
func (r *Reaper) WithAuditDLQMaxAgeDays(days int) *Reaper {
	if days > 0 {
		r.auditDLQMaxAgeDays = days
	}
	return r
}

// WithAuditDLQMaxReclaimAttempts caps how many times the reclaimer will
// retry a single DLQ row's chain insert. After this many failures the row
// is skipped (it stays in the DLQ for operator triage) and the
// AuditReclaimerAbandoned counter is incremented. Pass 0 to disable the
// cap entirely (legacy behavior — every row is retried forever).
func (r *Reaper) WithAuditDLQMaxReclaimAttempts(n int) *Reaper {
	if n >= 0 {
		r.auditDLQMaxReclaimAttempts = n
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

// recordReclaimerAbandoned increments the abandoned counter when a DLQ row
// hits the max-attempts cap and is skipped this tick. It stays in the DLQ
// so an operator can manually inspect or drop it.
func (r *Reaper) recordReclaimerAbandoned(ctx context.Context, n int) {
	if r.metrics == nil || n <= 0 {
		return
	}
	r.metrics.AuditReclaimerAbandoned.Add(ctx, int64(n))
}

// recordDeadletterAged increments the DLQ retention drop counter, labeled
// by project_id so a single noisy project does not hide drops elsewhere.
func (r *Reaper) recordDeadletterAged(ctx context.Context, projectID string, n int64) {
	if r.metrics == nil || n <= 0 {
		return
	}
	r.metrics.AuditDeadletterAged.Add(ctx, n, metric.WithAttributes(
		attribute.String("project_id", projectID),
	))
}

// reapDeadletter drops audit_events_deadletter rows older than the
// configured AUDIT_DLQ_MAX_AGE_DAYS window. Disabled when the value is 0.
//
// For every project that lost rows, the reaper emits an
// audit.deadletter_aged event into the chain so the drop is itself
// audited. The event lives outside the original DLQ row's project chain
// only when CreateAuditEvent fails — in that case the metric still
// records the drop count so operators can detect silent loss.
func (r *Reaper) reapDeadletter(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.reapDeadletter")
	defer span.End()

	if r.auditDLQMaxAgeDays <= 0 {
		return
	}
	cutoff := time.Now().UTC().Add(-time.Duration(r.auditDLQMaxAgeDays) * 24 * time.Hour)
	dropped, err := r.store.DeleteAuditDeadletterOlderThan(ctx, cutoff)
	if err != nil {
		r.logger.Error("failed to drop aged audit deadletter rows", "error", err)
		r.recordOperation(ctx, "audit_dlq_retention", "error")
		return
	}

	for projectID, n := range dropped {
		if n <= 0 {
			continue
		}
		r.logger.Info("aged out audit deadletter rows",
			"project_id", projectID, "dropped", n,
			"max_age_days", r.auditDLQMaxAgeDays, "cutoff", cutoff)
		r.recordDeadletterAged(ctx, projectID, n)
		r.emitDeadletterAgedAudit(ctx, projectID, n, cutoff)
	}
	r.recordOperation(ctx, "audit_dlq_retention", "ok")
}

// emitDeadletterAgedAudit writes an audit.deadletter_aged event for a
// project whose DLQ rows were just dropped by the retention reaper. The
// event carries dropped_count and the trigger reason. Failure to write
// the event is logged but not fatal — the drop already happened and the
// metric records it.
func (r *Reaper) emitDeadletterAgedAudit(ctx context.Context, projectID string, dropped int64, cutoff time.Time) {
	details, err := json.Marshal(map[string]any{
		"dropped_count":  dropped,
		"reason":         "max_age_exceeded",
		"max_age_cutoff": cutoff.Format(time.RFC3339),
		"max_age_days":   r.auditDLQMaxAgeDays,
	})
	if err != nil {
		r.logger.Warn("marshal deadletter_aged details failed", "error", err)
		return
	}
	ev := &domain.AuditEvent{
		ID:           uuid.Must(uuid.NewV7()).String(),
		ProjectID:    projectID,
		ActorID:      "system",
		ActorType:    "system",
		Action:       domain.AuditActionDeadletterAged,
		ResourceType: "audit_events_deadletter",
		ResourceID:   "retention",
		Details:      json.RawMessage(details),
	}
	if writeErr := r.store.CreateAuditEvent(ctx, ev); writeErr != nil {
		r.logger.Warn("failed to write deadletter_aged audit event",
			"project_id", projectID, "dropped", dropped, "error", writeErr)
	}
}

// reclaimAuditDeadletter replays deadlettered audit events back into the
// primary audit_events table. The batch is bounded by auditDLQReclaimBatch
// so a large backlog is drained across multiple ticks. Three policy gates
// shape the per-row decision:
//
//  1. Idempotency: if reclaimed_event_id is already set, the chain insert
//     was previously successful and only the DLQ delete is missing —
//     skip the chain insert and just delete.
//  2. Max-attempts: if attempt_count >= auditDLQMaxReclaimAttempts (and
//     the cap is enabled), the row is permanently broken; skip it this
//     tick and bump the abandoned counter.
//  3. Successful insert: assign a fresh chain ID, write to chain, mark
//     reclaimed_event_id on the DLQ row (so a retry skips re-insert),
//     then delete the DLQ row.
//
// On insert failure the attempt_count is incremented so subsequent ticks
// converge on the abandoned state instead of looping forever.
func (r *Reaper) reclaimAuditDeadletter(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.reclaimAuditDeadletter")
	defer span.End()

	batch := r.auditDLQReclaimBatch
	if batch <= 0 {
		batch = defaultAuditDLQReclaimBatch
	}
	span.SetAttributes(attribute.Int("reclaim.batch_size", batch))

	maxAttempts := r.auditDLQMaxReclaimAttempts
	if maxAttempts < 0 {
		maxAttempts = defaultAuditDLQMaxReclaimAttempts
	}

	events, ids, attempts, err := r.store.ListAuditEventsDeadletterWithAttempts(ctx, batch)
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
	abandoned := 0
	for i, ev := range events {
		dlqID := ids[i]
		info := attempts[i]

		// (1) Idempotent retry path: chain insert previously succeeded,
		// only the DLQ cleanup remains.
		if info.ReclaimedEventID != nil && *info.ReclaimedEventID != "" {
			if delErr := r.store.DeleteAuditEventDeadletter(ctx, dlqID, ev.ProjectID); delErr != nil {
				r.logger.Error("audit deadletter delete failed for previously-reclaimed row",
					"deadletter_id", dlqID, "new_event_id", *info.ReclaimedEventID, "error", delErr)
				deleteFailed++
				continue
			}
			reclaimed++
			continue
		}

		// (2) Max-attempts gate: skip the row this tick.
		if maxAttempts > 0 && info.AttemptCount >= maxAttempts {
			r.logger.Warn("audit deadletter row hit max reclaim attempts; skipping",
				"deadletter_id", dlqID, "attempt_count", info.AttemptCount,
				"max", maxAttempts, "action", ev.Action)
			abandoned++
			continue
		}

		// (3) Fresh attempt: atomically lock the DLQ row, insert into the
		// signed chain, mark it reclaimed, and delete the DLQ row in one
		// transaction. The store method is the single replay primitive used
		// by both admin and scheduler paths.
		newEventID := uuid.Must(uuid.NewV7()).String()
		insertCtx, insertCancel := context.WithTimeout(ctx, auditDLQPerEventReclaimTimeout)
		_, replayed, writeErr := r.store.ReplayAuditEventDeadletter(insertCtx, dlqID, ev.ProjectID, newEventID)
		insertCancel()
		if writeErr != nil {
			r.logger.Warn("audit deadletter replay failed",
				"deadletter_id", dlqID, "action", ev.Action, "error", writeErr)
			insertFailed++
			if incErr := r.store.IncrementAuditDeadletterAttempt(ctx, dlqID); incErr != nil {
				r.logger.Error("failed to increment dlq attempt count",
					"deadletter_id", dlqID, "error", incErr)
			}
			continue
		}
		if !replayed {
			continue
		}
		reclaimed++
	}

	span.SetAttributes(
		attribute.Int("reclaim.succeeded", reclaimed),
		attribute.Int("reclaim.failed", insertFailed+deleteFailed),
		attribute.Int("reclaim.abandoned", abandoned),
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
	if abandoned > 0 {
		r.recordReclaimerAbandoned(ctx, abandoned)
	}
	r.recordOperation(ctx, "audit_dlq_reclaim", "ok")
}
