package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/dbscan"
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

func (q *Queries) ListDeadLetterRuns(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListDeadLetterRuns")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	// Static SQL with a nullable-aware cursor predicate so pgx's statement
	// cache sees a single plan per connection regardless of whether the
	// caller supplied a cursor. Avoids per-call fmt.Sprintf and []any
	// append churn.
	const query = `
		SELECT jr.id, jr.job_id, jr.project_id, COALESCE(s.status, jr.status), COALESCE(s.attempt, jr.attempt), jr.payload,
		       CASE WHEN terminal.fields ? 'result' THEN terminal.fields->'result' ELSE jr.result END,
		       COALESCE(jr.metadata, '{}'::jsonb) || COALESCE(metadata_delta.metadata, '{}'::jsonb),
		       CASE WHEN terminal.fields ? 'error' THEN terminal.fields->>'error' ELSE jr.error END,
		       CASE WHEN terminal.fields ? 'error_class' THEN terminal.fields->>'error_class' ELSE jr.error_class END,
		       jr.triggered_by, COALESCE(s.scheduled_at, jr.scheduled_at), COALESCE(s.started_at, jr.started_at), COALESCE(s.finished_at, jr.finished_at), COALESCE(h.heartbeat_at, s.heartbeat_at, jr.heartbeat_at),
		       COALESCE(s.next_retry_at, jr.next_retry_at), COALESCE(s.expires_at, jr.expires_at), jr.parent_run_id, COALESCE(s.priority, jr.priority), jr.idempotency_key, jr.job_version, jr.created_at, jr.workflow_step_run_id,
		       CASE WHEN terminal.fields ? 'execution_trace' THEN terminal.fields->'execution_trace' ELSE jr.execution_trace END,
		       jr.debug_mode, jr.continuation_of, jr.lineage_depth, jr.tags, jr.job_version_id, jr.created_by, jr.batch_id, COALESCE(NULLIF(s.concurrency_key, ''), jr.concurrency_key), COALESCE(NULLIF(s.execution_mode, ''), jr.execution_mode), jr.is_rollback, jr.replayed_run_id
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		LEFT JOIN LATERAL (
			SELECT heartbeat_at
			FROM job_run_heartbeats h
			WHERE h.run_id = jr.id
			  AND h.cleared = FALSE
			ORDER BY h.id DESC
			LIMIT 1
		) h ON true
		LEFT JOIN LATERAL (
			SELECT fields
			FROM job_run_lifecycle_events e
			WHERE e.run_id = jr.id
			  AND e.fields ?| ARRAY['result', 'error', 'error_class', 'execution_trace']
			ORDER BY e.created_at DESC, e.id DESC
			LIMIT 1
		) terminal ON true
		LEFT JOIN LATERAL (
			SELECT jsonb_object_agg(entry.key, entry.value ORDER BY e.created_at, e.id) AS metadata
			FROM job_run_lifecycle_events e
			CROSS JOIN LATERAL jsonb_each(COALESCE(e.fields->'metadata', '{}'::jsonb)) AS entry(key, value)
			WHERE e.run_id = jr.id
			  AND e.fields ? 'metadata'
		) metadata_delta ON true
		WHERE jr.project_id = $1
		  AND COALESCE(s.status, jr.status) = 'dead_letter'
		  AND ($2::timestamptz IS NULL OR jr.created_at < $2::timestamptz)
		ORDER BY jr.created_at DESC
		LIMIT $3`

	rows, err := q.db.Query(ctx, query, projectID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list dead letter runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, limit)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list dead letter runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list dead letter runs rows: %w", err)
	}

	return runs, nil
}

// ListDeadLetterRunsFiltered is the filtered counterpart to
// ListDeadLetterRuns. Both jobID and masked are optional; when masked is
// non-nil it selects masked (true) vs visible (false) rows via the
// latest visibility state (masked == visible_until IS NOT NULL). Pushing the
// filter into SQL keeps pagination honest — client-side filtering of a
// single page would under-report results that live on earlier pages.
func (q *Queries) ListDeadLetterRunsFiltered(ctx context.Context, projectID string, jobID *string, masked *bool, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListDeadLetterRunsFiltered")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	// Static SQL with nullable-aware predicates so all 2^3 filter
	// combinations (job/masked/cursor) share a single cached plan per
	// connection. The masked filter is expressed as
	// (latest visible_until IS NOT NULL) = $masked, which lets a NULL
	// parameter disable the predicate entirely while still using the same plan.
	const query = `
		SELECT jr.id, jr.job_id, jr.project_id, COALESCE(s.status, jr.status), COALESCE(s.attempt, jr.attempt), jr.payload,
		       CASE WHEN terminal.fields ? 'result' THEN terminal.fields->'result' ELSE jr.result END,
		       COALESCE(jr.metadata, '{}'::jsonb) || COALESCE(metadata_delta.metadata, '{}'::jsonb),
		       CASE WHEN terminal.fields ? 'error' THEN terminal.fields->>'error' ELSE jr.error END,
		       CASE WHEN terminal.fields ? 'error_class' THEN terminal.fields->>'error_class' ELSE jr.error_class END,
		       jr.triggered_by, COALESCE(s.scheduled_at, jr.scheduled_at), COALESCE(s.started_at, jr.started_at), COALESCE(s.finished_at, jr.finished_at), COALESCE(h.heartbeat_at, s.heartbeat_at, jr.heartbeat_at),
		       COALESCE(s.next_retry_at, jr.next_retry_at), COALESCE(s.expires_at, jr.expires_at), jr.parent_run_id, COALESCE(s.priority, jr.priority), jr.idempotency_key, jr.job_version, jr.created_at, jr.workflow_step_run_id,
		       CASE WHEN terminal.fields ? 'execution_trace' THEN terminal.fields->'execution_trace' ELSE jr.execution_trace END,
		       jr.debug_mode, jr.continuation_of, jr.lineage_depth, jr.tags, jr.job_version_id, jr.created_by, jr.batch_id, COALESCE(NULLIF(s.concurrency_key, ''), jr.concurrency_key), COALESCE(NULLIF(s.execution_mode, ''), jr.execution_mode), jr.is_rollback, jr.replayed_run_id
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		LEFT JOIN LATERAL (
			SELECT heartbeat_at
			FROM job_run_heartbeats h
			WHERE h.run_id = jr.id
			  AND h.cleared = FALSE
			ORDER BY h.id DESC
			LIMIT 1
		) h ON true
		LEFT JOIN LATERAL (
			SELECT fields
			FROM job_run_lifecycle_events e
			WHERE e.run_id = jr.id
			  AND e.fields ?| ARRAY['result', 'error', 'error_class', 'execution_trace']
			ORDER BY e.created_at DESC, e.id DESC
			LIMIT 1
		) terminal ON true
		LEFT JOIN LATERAL (
			SELECT jsonb_object_agg(entry.key, entry.value ORDER BY e.created_at, e.id) AS metadata
			FROM job_run_lifecycle_events e
			CROSS JOIN LATERAL jsonb_each(COALESCE(e.fields->'metadata', '{}'::jsonb)) AS entry(key, value)
			WHERE e.run_id = jr.id
			  AND e.fields ? 'metadata'
		) metadata_delta ON true
		LEFT JOIN LATERAL (
			SELECT e.visible_until, TRUE AS has_event
			FROM job_run_visibility_events e
			WHERE e.run_id = jr.id
			ORDER BY e.id DESC
			LIMIT 1
		) visibility ON TRUE
		WHERE jr.project_id = $1
		  AND COALESCE(s.status, jr.status) = 'dead_letter'
		  AND ($2::text IS NULL OR jr.job_id = $2::text)
		  AND ($3::bool IS NULL OR (
		       CASE WHEN COALESCE(visibility.has_event, FALSE)
		            THEN visibility.visible_until IS NOT NULL
		            ELSE jr.visible_until IS NOT NULL
		       END
		  ) = $3::bool)
		  AND ($4::timestamptz IS NULL OR jr.created_at < $4::timestamptz)
		ORDER BY jr.created_at DESC
		LIMIT $5`

	// Normalize the optional job filter: callers pass either nil or a
	// pointer to an empty string when the filter is absent. pgx marshals
	// *string to text or NULL directly, so no allocation is needed for
	// the default path.
	var jobArg any
	if jobID != nil && *jobID != "" {
		jobArg = *jobID
	}

	var maskedArg any
	if masked != nil {
		maskedArg = *masked
	}

	var cursorArg any
	if cursor != nil {
		cursorArg = *cursor
	}

	rows, err := q.db.Query(ctx, query, projectID, jobArg, maskedArg, cursorArg, limit)
	if err != nil {
		return nil, fmt.Errorf("list dead letter runs filtered: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, limit)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list dead letter runs filtered scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list dead letter runs filtered rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ReplayDeadLetterRun(ctx context.Context, runID string) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReplayDeadLetterRun")
	defer span.End()

	current, _, err := q.currentRunMutableState(ctx, runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("replay dead letter run: load status: %w", err)
	}
	if current != domain.StatusDeadLetter {
		return nil, fmt.Errorf("%w: run %s has status %s, expected dead_letter", ErrRunConflict, runID, current)
	}

	if err := q.UpdateRunStatus(ctx, runID, domain.StatusDeadLetter, domain.StatusQueued, map[string]any{
		"attempt":       1,
		"error":         "",
		"started_at":    nil,
		"finished_at":   nil,
		"heartbeat_at":  nil,
		"next_retry_at": nil,
	}); err == nil {
		return q.GetRun(ctx, runID)
	} else if !errors.Is(err, ErrRunConflict) {
		return nil, fmt.Errorf("replay dead letter run: %w", err)
	}

	var status domain.RunStatus
	loadErr := q.db.QueryRow(ctx, `
		SELECT COALESCE(s.status, jr.status)
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,
		runID,
	).Scan(&status)
	if loadErr != nil {
		if errors.Is(loadErr, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("replay dead letter run: disambiguate: %w", loadErr)
	}
	return nil, fmt.Errorf("%w: run %s has status %s, expected dead_letter", ErrRunConflict, runID, status)
}

func (q *Queries) BulkReplayDeadLetterRuns(ctx context.Context, runIDs []string, projectID string, limit int) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BulkReplayDeadLetterRuns")
	defer span.End()

	if len(runIDs) == 0 && projectID == "" {
		return nil, fmt.Errorf("at least one run id or project_id is required")
	}
	if len(runIDs) > 0 && projectID != "" {
		return nil, fmt.Errorf("provide either run_ids or project_id, not both")
	}

	idsToReplay := runIDs
	if len(idsToReplay) == 0 {
		if limit <= 0 {
			limit = 100
		}
		query := `
			SELECT jr.id
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE jr.project_id = $1 AND COALESCE(s.status, jr.status) = 'dead_letter'
			ORDER BY jr.created_at ASC
			LIMIT $2`
		rows, err := q.db.Query(ctx, query, projectID, limit)
		if err != nil {
			return nil, fmt.Errorf("select dead letter runs for bulk replay: %w", err)
		}
		defer rows.Close()

		idsToReplay = make([]string, 0, limit)
		for rows.Next() {
			var runID string
			if scanErr := rows.Scan(&runID); scanErr != nil {
				return nil, fmt.Errorf("scan dead letter run id for bulk replay: %w", scanErr)
			}
			idsToReplay = append(idsToReplay, runID)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate dead letter run ids for bulk replay: %w", err)
		}
	}

	replayed := make([]domain.JobRun, 0, len(idsToReplay))
	replayRuns := func(runQ *Queries) error {
		for _, runID := range idsToReplay {
			run, err := runQ.GetRun(ctx, runID)
			if err != nil {
				if errors.Is(err, ErrRunNotFound) {
					continue
				}
				return fmt.Errorf("get run %s for bulk replay: %w", runID, err)
			}
			if run.Status != domain.StatusDeadLetter {
				continue
			}

			if err := runQ.UpdateRunStatus(ctx, runID, domain.StatusDeadLetter, domain.StatusReplayStaged, nil); err != nil {
				return fmt.Errorf("stage run %s for replay: %w", runID, err)
			}

			if err := runQ.UpdateRunStatus(ctx, runID, domain.StatusReplayStaged, domain.StatusQueued, map[string]any{
				"attempt":       1,
				"error":         "",
				"started_at":    nil,
				"finished_at":   nil,
				"heartbeat_at":  nil,
				"next_retry_at": nil,
			}); err != nil {
				return fmt.Errorf("enqueue staged run %s: %w", runID, err)
			}

			updatedRun, err := runQ.GetRun(ctx, runID)
			if err != nil {
				return fmt.Errorf("get replayed run %s: %w", runID, err)
			}
			replayed = append(replayed, *updatedRun)
		}

		return nil
	}

	if _, ok := q.db.(TxBeginner); ok {
		if err := q.withTx(ctx, replayRuns); err != nil {
			return nil, fmt.Errorf("bulk replay dead letter runs transaction: %w", err)
		}
	} else {
		if err := replayRuns(q); err != nil {
			return nil, err
		}
	}

	if len(replayed) == 0 {
		return nil, fmt.Errorf("no dead_letter runs available for replay")
	}

	return replayed, nil
}

// DLQJobDepth represents the dead-letter queue depth for a single job.
type DLQJobDepth struct {
	JobID             string
	WebhookURL        string
	DLQCount          int
	DLQAlertThreshold int
}

func (q *Queries) ListDLQDepthByJob(ctx context.Context) ([]DLQJobDepth, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListDLQDepthByJob")
	defer span.End()

	query := `
		SELECT jr.job_id, COALESCE(j.webhook_url, ''), COUNT(*) AS dlq_count, j.dlq_alert_threshold
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		JOIN jobs j ON j.id = jr.job_id
		WHERE COALESCE(s.status, jr.status) = 'dead_letter'
		  AND j.dlq_alert_threshold IS NOT NULL
		GROUP BY jr.job_id, j.webhook_url, j.dlq_alert_threshold
		HAVING COUNT(*) >= j.dlq_alert_threshold`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list dlq depth by job: %w", err)
	}
	defer rows.Close()

	var results []DLQJobDepth
	for rows.Next() {
		var d DLQJobDepth
		if err := rows.Scan(&d.JobID, &d.WebhookURL, &d.DLQCount, &d.DLQAlertThreshold); err != nil {
			return nil, fmt.Errorf("scan dlq depth: %w", err)
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

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
		deleted_retries AS (
			DELETE FROM job_retries
			WHERE run_id IN (SELECT id FROM victim)
		),
		deleted_priority_events AS (
			DELETE FROM job_run_priority_events
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
		deleted_cache_versions AS (
			DELETE FROM job_run_cache_versions
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
