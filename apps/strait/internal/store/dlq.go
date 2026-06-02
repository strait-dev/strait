package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// ReplayDeadLetterRunWithAudit atomically (a) CASes a dead-letter run
// back to queued, (b) marks the original row's replayed_run_id for
// lineage, and (c) writes the supplied audit event. All three writes
// share a single transaction so a crash midway cannot leave half-applied
// state (the prior sequential path could leave the run queued without an
// audit row when the final CreateAuditEvent failed).
//
// The caller pre-builds the audit row (actor, action, resource, project)
// before entering the transaction; this helper fills in the before/after
// details payload using the CAS result. Returns ErrRunNotFound when the
// row does not exist and ErrRunConflict when the row exists but is not in
// dead_letter status or already carries a replayed_run_id.
func (q *Queries) ReplayDeadLetterRunWithAudit(ctx context.Context, runID string, audit *domain.AuditEvent) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReplayDeadLetterRunWithAudit")
	defer span.End()

	if audit == nil {
		return nil, fmt.Errorf("replay dlq with audit: audit event is required")
	}

	_, ok := q.db.(TxBeginner)
	if !ok {
		return nil, fmt.Errorf("replay dlq with audit: db does not support transactions")
	}

	var result *domain.JobRun
	err := q.withTx(ctx, func(txQ *Queries) error {
		run, err := txQ.ReplayDeadLetterRun(ctx, runID)
		if err != nil {
			return err
		}

		if err := txQ.MarkRunReplayed(ctx, runID, run.ID); err != nil {
			return fmt.Errorf("mark run replayed: %w", err)
		}

		details := map[string]any{
			"run_id": runID,
			"before": map[string]any{"status": domain.StatusDeadLetter},
			"after":  map[string]any{"status": run.Status, "replayed_run_id": run.ID},
		}
		raw, mErr := json.Marshal(details)
		if mErr != nil {
			return fmt.Errorf("marshal audit details: %w", mErr)
		}
		audit.Details = raw
		if audit.ResourceID == "" {
			audit.ResourceID = runID
		}
		if audit.ResourceType == "" {
			audit.ResourceType = "job_run"
		}
		// Derive project id from the CAS result so callers don't have to
		// pre-load the run just to populate the audit envelope.
		if audit.ProjectID == "" {
			audit.ProjectID = run.ProjectID
		}

		if err := txQ.CreateAuditEvent(ctx, audit); err != nil {
			return fmt.Errorf("create audit event: %w", err)
		}

		result = run
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("replay dlq with audit: %w", err)
	}
	return result, nil
}

// DLQ admin helpers power the /v1/admin/dlq HTTP endpoints. Listing and
// replay reuse the existing ListDeadLetterRuns / ReplayDeadLetterRun
// helpers on Queries; this file adds the mutations that did not previously
// have dedicated entry points (unmask + purge).

// UnmaskDLQRun clears visible_until on a dead-letter run so it is no
// longer hidden by the DLQ age-out process. The run must already be in
// dead_letter status; otherwise ErrRunConflict is returned. Returns
// ErrRunNotFound if the run does not exist.
func (q *Queries) UnmaskDLQRun(ctx context.Context, runID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UnmaskDLQRun")
	defer span.End()

	// Single-statement mutation with guard: avoids a check-then-act race
	// where two concurrent callers both observe status=dead_letter before
	// either lands its UPDATE. The loser's RETURNING comes back empty; we
	// then do a follow-up SELECT to disambiguate ErrRunNotFound (row is
	// gone) from ErrRunConflict (row exists but is no longer dead_letter).
	var id string
	err := q.db.QueryRow(ctx, `
		WITH selected AS (
			SELECT
				jr.id,
				jr.project_id,
				jr.job_id,
				jr.status AS ledger_status,
				CASE WHEN COALESCE(visibility.has_event, FALSE)
				     THEN (visibility.visible_until IS NULL OR visibility.visible_until > NOW())
				     ELSE (jr.visible_until IS NULL OR jr.visible_until > NOW())
				END AS was_visible
			FROM job_runs jr
			LEFT JOIN LATERAL (
				SELECT e.visible_until, TRUE AS has_event
				FROM job_run_visibility_events e
				WHERE e.run_id = jr.id
				ORDER BY e.id DESC
				LIMIT 1
			) visibility ON TRUE
			WHERE jr.id = $1
			  AND COALESCE((
				SELECT s.status
				FROM job_run_read_state s
				WHERE s.run_id = jr.id
			  ), jr.status) = 'dead_letter'
			FOR UPDATE OF jr
			),
			updated AS (
				INSERT INTO job_run_visibility_events (run_id, visible_until)
				SELECT id, NULL
				FROM selected
				RETURNING run_id
			),
		incremented AS (
			INSERT INTO dlq_counts (project_id, job_id, count)
			SELECT project_id, job_id, 1
			FROM selected
			WHERE NOT was_visible
			ON CONFLICT (project_id, job_id)
			DO UPDATE SET count = dlq_counts.count + 1, updated_at = NOW()
			RETURNING 1
		)
		SELECT run_id FROM updated`, runID).Scan(&id)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("unmask dlq run: %w", err)
	}

	status, _, loadErr := q.currentRunMutableState(ctx, runID)
	if loadErr != nil {
		if errors.Is(loadErr, pgx.ErrNoRows) {
			return ErrRunNotFound
		}
		return fmt.Errorf("unmask dlq run: disambiguate: %w", loadErr)
	}
	return fmt.Errorf("%w: run %s has status %s, expected dead_letter", ErrRunConflict, runID, status)
}

// PurgeDLQRun hard-deletes a dead-letter run. Requires the caller to
// already hold the dlq:purge scope. Returns ErrRunNotFound if the run
// does not exist or is not in dead_letter status.
func (q *Queries) PurgeDLQRun(ctx context.Context, runID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.PurgeDLQRun")
	defer span.End()

	// Single-statement DELETE with status guard; see UnmaskDLQRun for the
	// rationale. Side tables owned by the split run-state model are removed
	// in the same statement so hard purge does not leave cold-state or
	// lifecycle rows behind.
	var id string
	err := q.db.QueryRow(ctx, `
		WITH victim AS (
			SELECT
				jr.id,
				jr.project_id,
				jr.job_id,
				jr.status AS ledger_status,
				(jr.visible_until IS NULL OR jr.visible_until > NOW()) AS ledger_visible,
				CASE WHEN COALESCE(visibility.has_event, FALSE)
				     THEN (visibility.visible_until IS NULL OR visibility.visible_until > NOW())
				     ELSE (jr.visible_until IS NULL OR jr.visible_until > NOW())
				END AS was_visible
			FROM job_runs jr
			LEFT JOIN LATERAL (
				SELECT e.visible_until, TRUE AS has_event
				FROM job_run_visibility_events e
				WHERE e.run_id = jr.id
				ORDER BY e.id DESC
				LIMIT 1
			) visibility ON TRUE
			WHERE jr.id = $1
			  AND COALESCE((
				SELECT s.status
				FROM job_run_read_state s
				WHERE s.run_id = jr.id
			  ), jr.status) = 'dead_letter'
			FOR UPDATE OF jr
		),
		deleted_active_claims AS (
			DELETE FROM job_run_active_claims
			WHERE run_id IN (SELECT id FROM victim)
		),
		deleted_lifecycle_events AS (
			DELETE FROM job_run_lifecycle_events
			WHERE run_id IN (SELECT id FROM victim)
		),
		deleted_ready_events AS (
			DELETE FROM job_run_ready_events
			WHERE run_id IN (SELECT id FROM victim)
		),
		deleted_terminal_state AS (
			DELETE FROM job_run_terminal_state
			WHERE run_id IN (SELECT id FROM victim)
		),
		deleted_heartbeats AS (
			DELETE FROM job_run_heartbeats
			WHERE run_id IN (SELECT id FROM victim)
		),
		deleted_visibility_events AS (
			DELETE FROM job_run_visibility_events
			WHERE run_id IN (SELECT id FROM victim)
		),
		decremented AS (
			UPDATE dlq_counts c
			SET count = GREATEST(c.count - 1, 0),
			    updated_at = NOW()
			FROM victim v
			WHERE v.ledger_status <> 'dead_letter'
			  AND v.was_visible
			  AND c.project_id = v.project_id
			  AND c.job_id = v.job_id
			RETURNING 1
		),
		deleted_run AS (
			DELETE FROM job_runs
			WHERE id IN (SELECT id FROM victim)
			RETURNING id
		),
		restored_trigger_decrement AS (
			INSERT INTO dlq_counts (project_id, job_id, count)
			SELECT v.project_id, v.job_id, 1
			FROM victim v
			JOIN deleted_run d ON d.id = v.id
			WHERE v.ledger_status = 'dead_letter'
			  AND v.ledger_visible
			  AND NOT v.was_visible
			ON CONFLICT (project_id, job_id)
			DO UPDATE SET count = dlq_counts.count + 1, updated_at = NOW()
			RETURNING 1
		)
		SELECT id FROM deleted_run`, runID).Scan(&id)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("purge dlq run: %w", err)
	}

	status, _, loadErr := q.currentRunMutableState(ctx, runID)
	if loadErr != nil {
		if errors.Is(loadErr, pgx.ErrNoRows) {
			return ErrRunNotFound
		}
		return fmt.Errorf("purge dlq run: disambiguate: %w", loadErr)
	}
	return fmt.Errorf("%w: run %s has status %s, expected dead_letter", ErrRunConflict, runID, status)
}

// MarkRunReplayed sets replayed_run_id on an original dead-letter run to
// record the lineage of a replay. Called by the admin DLQ replay handler
// after a successful re-enqueue. replayedByRunID may equal originalRunID
// when the existing ReplayDeadLetterRun behavior reuses the same run ID.
//
// Stamp-once semantics: the WHERE clause guards on replayed_run_id IS
// NULL so a second call cannot overwrite the first replay's lineage.
// Status is not part of the guard because ReplayDeadLetterRun has
// already CAS'd the row out of dead_letter by the time this runs.
//
// Returns ErrRunNotFound when the row does not exist, and
// ErrRunConflict when the row exists but already carries a
// replayed_run_id (out-of-order or duplicate call).
func (q *Queries) MarkRunReplayed(ctx context.Context, originalRunID, replayedByRunID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkRunReplayed")
	defer span.End()

	var id string
	err := q.db.QueryRow(ctx,
		`UPDATE job_runs SET replayed_run_id = $1 WHERE id = $2 AND replayed_run_id IS NULL RETURNING id`,
		replayedByRunID, originalRunID,
	).Scan(&id)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("mark run replayed: %w", err)
	}

	// Disambiguate: row absent vs already stamped.
	var existing *string
	loadErr := q.db.QueryRow(ctx, `SELECT replayed_run_id FROM job_runs WHERE id = $1`, originalRunID).Scan(&existing)
	if loadErr != nil {
		if errors.Is(loadErr, pgx.ErrNoRows) {
			return ErrRunNotFound
		}
		return fmt.Errorf("mark run replayed: disambiguate: %w", loadErr)
	}
	return fmt.Errorf("%w: run %s already has replayed_run_id set", ErrRunConflict, originalRunID)
}
