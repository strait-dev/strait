package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/samber/lo"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateRun(ctx context.Context, run *domain.JobRun) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateRun")
	defer span.End()

	if run.ID == "" {
		run.ID = uuid.Must(uuid.NewV7()).String()
	}

	if run.Attempt == 0 {
		run.Attempt = 1
	}

	if run.TriggeredBy == "" {
		run.TriggeredBy = domain.TriggerManual
	}

	if run.Status == "" || run.Status == domain.StatusQueued {
		run.Status = domain.StatusQueued
		if run.ScheduledAt != nil && run.ScheduledAt.After(time.Now()) {
			run.Status = domain.StatusDelayed
		}
	}

	metadataJSON := []byte("{}")
	if len(run.Metadata) > 0 {
		var marshalErr error
		metadataJSON, marshalErr = json.Marshal(run.Metadata)
		if marshalErr != nil {
			return fmt.Errorf("create run: marshal metadata: %w", marshalErr)
		}
	}

	query := `
		WITH idempotency_check AS (
			SELECT 1
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE jr.job_id = $2
			  AND jr.idempotency_key = $18
			  AND jr.idempotency_key IS NOT NULL
			  AND COALESCE(s.status, jr.status) IN ('delayed', 'queued', 'dequeued', 'executing', 'waiting')
			LIMIT 1
		)
		INSERT INTO job_runs (
			id, job_id, project_id, status, attempt, payload, result, error,
			triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, workflow_step_run_id,
			debug_mode, continuation_of, lineage_depth,
			tags, job_version_id, created_by, concurrency_key, batch_id,
				execution_mode, queue_name, metadata,
				is_rollback
			)
			SELECT
				$1, $2, $3, $4, $5, $6, $7, $8,
				$9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
				$21, $22, $23,
				$24::jsonb, $25, $26, $27, $28,
				$29, $30, $31::jsonb, $32
			WHERE NOT EXISTS (SELECT 1 FROM idempotency_check)
			RETURNING created_at`

	execMode := run.ExecutionMode
	if execMode == "" {
		execMode = domain.ExecutionModeHTTP
	}
	queueName := run.QueueName
	if queueName == "" {
		queueName = defaultJobQueueName
	}

	err := q.db.QueryRow(
		ctx,
		query,
		run.ID,
		run.JobID,
		run.ProjectID,
		run.Status,
		run.Attempt,
		dbscan.NilIfEmptyRawMessage(run.Payload),
		dbscan.NilIfEmptyRawMessage(run.Result),
		dbscan.NilIfEmptyString(run.Error),
		run.TriggeredBy,
		run.ScheduledAt,
		run.StartedAt,
		run.FinishedAt,
		run.HeartbeatAt,
		run.NextRetryAt,
		run.ExpiresAt,
		dbscan.NilIfEmptyString(run.ParentRunID),
		run.Priority,
		dbscan.NilIfEmptyString(run.IdempotencyKey),
		run.JobVersion,
		dbscan.NilIfEmptyString(run.WorkflowStepRunID),
		run.DebugMode,
		dbscan.NilIfEmptyString(run.ContinuationOf),
		run.LineageDepth,
		marshalTagsForRun(run.Tags),
		dbscan.NilIfEmptyString(run.JobVersionID),
		dbscan.NilIfEmptyString(run.CreatedBy),
		dbscan.NilIfEmptyString(run.ConcurrencyKey),
		dbscan.NilIfEmptyString(run.BatchID),
		string(execMode),
		queueName,
		metadataJSON,
		run.IsRollback,
	).Scan(&run.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) && run.IdempotencyKey != "" {
			return domain.ErrIdempotencyConflict
		}
		return fmt.Errorf("create run: %w", err)
	}

	return nil
}

func (q *Queries) GetRunStatus(ctx context.Context, id string) (domain.RunStatus, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRunStatus")
	defer span.End()

	var status domain.RunStatus
	err := q.db.QueryRow(ctx, `
		SELECT COALESCE(s.status, jr.status)
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,
		id,
	).Scan(&status)
	if err == nil {
		return status, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("get run status: %w", err)
	}
	if err := q.db.QueryRow(ctx, `SELECT status FROM job_runs_history WHERE id = $1`, id).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrRunNotFound
		}
		return "", fmt.Errorf("get run status: history fallback: %w", err)
	}
	return status, nil
}

func (q *Queries) GetRunTokenState(ctx context.Context, id string) (domain.RunStatus, int, string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRunTokenState")
	defer span.End()

	var status domain.RunStatus
	var attempt int
	var projectID string
	err := q.db.QueryRow(ctx, `
		SELECT COALESCE(s.status, jr.status), COALESCE(s.attempt, jr.attempt), jr.project_id
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,
		id,
	).Scan(&status, &attempt, &projectID)
	if err == nil {
		return status, attempt, projectID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", 0, "", fmt.Errorf("get run token state: %w", err)
	}
	err = q.db.QueryRow(ctx, `SELECT status, attempt, project_id FROM job_runs_history WHERE id = $1`, id).Scan(&status, &attempt, &projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, "", ErrRunNotFound
		}
		return "", 0, "", fmt.Errorf("get run token state: history fallback: %w", err)
	}
	return status, attempt, projectID, nil
}

func (q *Queries) EnsureRunActiveForAttempt(ctx context.Context, id string, attempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.EnsureRunActiveForAttempt")
	defer span.End()

	var exists bool
	err := q.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE jr.id = $1
			  AND COALESCE(s.attempt, jr.attempt) = $2
			  AND COALESCE(s.status, jr.status) IN ('executing', 'waiting')
		 )`, id, attempt).Scan(&exists)
	if err != nil {
		return fmt.Errorf("ensure active run: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, id, attempt)
	}
	return nil
}

func (q *Queries) GetRun(ctx context.Context, id string) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRun")
	defer span.End()

	query := `
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
		WHERE jr.id = $1`

	run, err := dbscan.ScanRun(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			historyRun, histErr := q.GetRunFromHistory(ctx, id)
			if histErr != nil {
				return nil, fmt.Errorf("get run: history fallback: %w", histErr)
			}
			if historyRun != nil {
				return historyRun, nil
			}
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("get run: %w", err)
	}

	return run, nil
}

func (q *Queries) GetRunWithCacheVersion(ctx context.Context, id string) (*domain.JobRun, int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRunWithCacheVersion")
	defer span.End()

	query := `
		SELECT jr.id, jr.job_id, jr.project_id, COALESCE(s.status, jr.status), COALESCE(s.attempt, jr.attempt), jr.payload,
		       CASE WHEN terminal.fields ? 'result' THEN terminal.fields->'result' ELSE jr.result END,
		       COALESCE(jr.metadata, '{}'::jsonb) || COALESCE(metadata_delta.metadata, '{}'::jsonb),
		       CASE WHEN terminal.fields ? 'error' THEN terminal.fields->>'error' ELSE jr.error END,
		       CASE WHEN terminal.fields ? 'error_class' THEN terminal.fields->>'error_class' ELSE jr.error_class END,
		       jr.triggered_by, COALESCE(s.scheduled_at, jr.scheduled_at), COALESCE(s.started_at, jr.started_at), COALESCE(s.finished_at, jr.finished_at), COALESCE(h.heartbeat_at, s.heartbeat_at, jr.heartbeat_at),
		       COALESCE(s.next_retry_at, jr.next_retry_at), COALESCE(s.expires_at, jr.expires_at), jr.parent_run_id, COALESCE(s.priority, jr.priority), jr.idempotency_key, jr.job_version, jr.created_at, jr.workflow_step_run_id,
		       CASE WHEN terminal.fields ? 'execution_trace' THEN terminal.fields->'execution_trace' ELSE jr.execution_trace END,
		       jr.debug_mode, jr.continuation_of, jr.lineage_depth, jr.tags, jr.job_version_id, jr.created_by, jr.batch_id, COALESCE(NULLIF(s.concurrency_key, ''), jr.concurrency_key), COALESCE(NULLIF(s.execution_mode, ''), jr.execution_mode), jr.is_rollback, jr.replayed_run_id, COALESCE(v.cache_version, jr.cache_version)
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
			SELECT cache_version
			FROM job_run_cache_versions v
			WHERE v.run_id = jr.id
			ORDER BY v.id DESC
			LIMIT 1
		) v ON true
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
		WHERE jr.id = $1`

	run, err := dbscan.ScanRunWithCacheVersion(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			historyRun, histVersion, histErr := q.GetRunFromHistoryWithCacheVersion(ctx, id)
			if histErr != nil {
				return nil, 0, fmt.Errorf("get run with cache version: history fallback: %w", histErr)
			}
			if historyRun != nil {
				return historyRun, histVersion, nil
			}
			return nil, 0, ErrRunNotFound
		}
		return nil, 0, fmt.Errorf("get run with cache version: %w", err)
	}

	return run, run.CacheVersion, nil
}

func (q *Queries) GetRunByIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRunByIdempotencyKey")
	defer span.End()

	// Backed by idx_runs_idempotency on (job_id, idempotency_key) WHERE
	// idempotency_key IS NOT NULL. Non-partial w.r.t. status so that
	// terminal flips do not trigger index writes. The finished_at filter
	// is satisfied by a row fetch after the index narrows to <=few rows.
	query := `
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
		WHERE jr.job_id = $1
		  AND jr.idempotency_key = $2
		  AND (
		    COALESCE(s.status, jr.status) IN ('delayed', 'queued', 'dequeued', 'executing', 'waiting')
		    OR (COALESCE(s.status, jr.status) IN ('completed', 'failed', 'timed_out', 'crashed', 'system_failed', 'canceled', 'dead_letter') AND COALESCE(s.finished_at, jr.finished_at) > NOW() - INTERVAL '24 hours')
		  )
		ORDER BY jr.created_at DESC
		LIMIT 1`

	run, err := dbscan.ScanRun(q.db.QueryRow(ctx, query, jobID, idempotencyKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get run by idempotency key: %w", err)
	}

	return run, nil
}

// Queries hot table only; archived runs are not included.
func (q *Queries) FindRecentRunByPayload(ctx context.Context, jobID string, payload json.RawMessage, since time.Time) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.FindRecentRunByPayload")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, is_rollback, replayed_run_id
		FROM job_runs
		WHERE job_id = $1
		  AND payload = $2::jsonb
		  AND created_at >= $3
		ORDER BY created_at DESC
		LIMIT 1`

	run, err := dbscan.ScanRun(q.db.QueryRow(ctx, query, jobID, dbscan.NilIfEmptyRawMessage(payload), since))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find recent run by payload: %w", err)
	}

	return run, nil
}

// Queries hot table only; archived runs are not included.
func (q *Queries) CountRunsForJobSince(ctx context.Context, jobID string, since time.Time) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountRunsForJobSince")
	defer span.End()

	query := `
		SELECT COUNT(*)
		FROM job_runs
		WHERE job_id = $1
		  AND created_at >= $2`

	var count int
	if err := q.db.QueryRow(ctx, query, jobID, since).Scan(&count); err != nil {
		return 0, fmt.Errorf("count runs for job since: %w", err)
	}

	return count, nil
}

func (q *Queries) AreAllDescendantsTerminal(ctx context.Context, parentRunID string) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.AreAllDescendantsTerminal")
	defer span.End()

	query := `
		WITH RECURSIVE descendants AS (
			SELECT jr.id, COALESCE(s.status, jr.status) AS status, 1 AS depth
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE jr.parent_run_id = $1
			UNION ALL
			SELECT jr.id, COALESCE(s.status, jr.status), d.depth + 1
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			JOIN descendants d ON jr.parent_run_id = d.id
			WHERE d.depth < 100
		)
		SELECT COUNT(*)
		FROM descendants
		WHERE status NOT IN ('completed', 'failed', 'timed_out', 'crashed', 'system_failed', 'canceled', 'expired')`

	var nonTerminalCount int
	if err := q.db.QueryRow(ctx, query, parentRunID).Scan(&nonTerminalCount); err != nil {
		return false, fmt.Errorf("check descendants terminal: %w", err)
	}

	return nonTerminalCount == 0, nil
}

// SumRunCostMicrousd returns the recorded launch compute cost for a single run.
func (q *Queries) SumRunCostMicrousd(ctx context.Context, runID string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SumRunCostMicrousd")
	defer span.End()

	query := `
		SELECT COALESCE(SUM(compute_cost_microusd), 0)
		FROM billing_cost_events
		WHERE idempotency_key = 'strait:cost_recorded:' || $1`
	var total int64
	if err := q.db.QueryRow(ctx, query, runID).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum run cost: %w", err)
	}
	return total, nil
}

// SumProjectDailyCostMicrousd returns today's launch compute cost for a project.
func (q *Queries) SumProjectDailyCostMicrousd(ctx context.Context, projectID string, timezone string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SumProjectDailyCostMicrousd")
	defer span.End()

	tz := timezone
	if tz == "" {
		tz = "UTC"
	}

	query := `
		SELECT COALESCE(SUM(compute_cost_microusd), 0)
		FROM billing_cost_events
		WHERE project_id = $1
		  AND created_at >= (date_trunc('day', NOW() AT TIME ZONE $2) AT TIME ZONE $2)
		  AND created_at < ((date_trunc('day', NOW() AT TIME ZONE $2) + INTERVAL '1 day') AT TIME ZONE $2)
	`
	var total int64
	if err := q.db.QueryRow(ctx, query, projectID, tz).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum project daily cost: %w", err)
	}
	return total, nil
}

func (q *Queries) ListRunsByJob(ctx context.Context, jobID string, limit, offset int) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunsByJob")
	defer span.End()

	query := `
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
		WHERE jr.job_id = $1
		ORDER BY jr.created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := q.db.Query(ctx, query, jobID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list runs by job: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, limit)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list runs by job scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runs by job rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue, triggeredBy, batchID *string, payloadContains json.RawMessage, executionMode *domain.ExecutionMode, errorClass *string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunsByProject")
	defer span.End()

	baseQuery := `
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
		WHERE jr.project_id = $1`

	args := []any{projectID}
	param := 2

	if status != nil {
		baseQuery += fmt.Sprintf(" AND COALESCE(s.status, jr.status) = $%d", param)
		args = append(args, *status)
		param++
	}

	if metadataKey != nil {
		if metadataValue == nil {
			baseQuery += fmt.Sprintf(" AND (COALESCE(jr.metadata, '{}'::jsonb) || COALESCE(metadata_delta.metadata, '{}'::jsonb)) ? $%d", param)
			args = append(args, *metadataKey)
			param++
		} else {
			baseQuery += fmt.Sprintf(" AND (COALESCE(jr.metadata, '{}'::jsonb) || COALESCE(metadata_delta.metadata, '{}'::jsonb)) ->> $%d = $%d", param, param+1)
			args = append(args, *metadataKey, *metadataValue)
			param += 2
		}
	}

	if triggeredBy != nil {
		baseQuery += fmt.Sprintf(" AND jr.triggered_by = $%d", param)
		args = append(args, *triggeredBy)
		param++
	}

	if batchID != nil {
		baseQuery += fmt.Sprintf(" AND jr.batch_id = $%d", param)
		args = append(args, *batchID)
		param++
	}

	if len(payloadContains) > 0 {
		baseQuery += fmt.Sprintf(" AND jr.payload @> $%d::jsonb", param)
		args = append(args, payloadContains)
		param++
	}

	if executionMode != nil {
		baseQuery += fmt.Sprintf(" AND COALESCE(NULLIF(s.execution_mode, ''), jr.execution_mode) = $%d", param)
		args = append(args, string(*executionMode))
		param++
	}

	if errorClass != nil {
		baseQuery += fmt.Sprintf(" AND CASE WHEN terminal.fields ? 'error_class' THEN terminal.fields->>'error_class' ELSE jr.error_class END = $%d", param)
		args = append(args, *errorClass)
		param++
	}

	if cursor != nil {
		baseQuery += fmt.Sprintf(" AND jr.created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	baseQuery += fmt.Sprintf(" ORDER BY jr.created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("list runs by project: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, limit)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list runs by project scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runs by project rows: %w", err)
	}

	return runs, nil
}

// ListRunsByProjectFiltered applies the full /runs filter set in SQL. This
// avoids page-by-page post-filtering and per-job environment lookups in the API
// layer for real stores.
func (q *Queries) ListRunsByProjectFiltered(ctx context.Context, projectID string, status *domain.RunStatus, statuses []domain.RunStatus, tagKey, tagValue string, environmentID *string, metadataKey, metadataValue, triggeredBy, batchID *string, payloadContains json.RawMessage, executionMode *domain.ExecutionMode, errorClass *string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunsByProjectFiltered")
	defer span.End()

	baseQuery := `
		SELECT jr.id, jr.job_id, jr.project_id, COALESCE(s.status, jr.status), COALESCE(s.attempt, jr.attempt), jr.payload,
		       CASE WHEN terminal.fields ? 'result' THEN terminal.fields->'result' ELSE jr.result END,
		       COALESCE(jr.metadata, '{}'::jsonb) || COALESCE(metadata_delta.metadata, '{}'::jsonb),
		       CASE WHEN terminal.fields ? 'error' THEN terminal.fields->>'error' ELSE jr.error END,
		       CASE WHEN terminal.fields ? 'error_class' THEN terminal.fields->>'error_class' ELSE jr.error_class END,
		       jr.triggered_by, COALESCE(s.scheduled_at, jr.scheduled_at), COALESCE(s.started_at, jr.started_at), COALESCE(s.finished_at, jr.finished_at), COALESCE(h.heartbeat_at, s.heartbeat_at, jr.heartbeat_at),
		       COALESCE(s.next_retry_at, jr.next_retry_at), COALESCE(s.expires_at, jr.expires_at), jr.parent_run_id, COALESCE(s.priority, jr.priority), jr.idempotency_key, jr.job_version, jr.created_at, jr.workflow_step_run_id,
		       CASE WHEN terminal.fields ? 'execution_trace' THEN terminal.fields->'execution_trace' ELSE jr.execution_trace END,
		       jr.debug_mode, jr.continuation_of, jr.lineage_depth, jr.tags, jr.job_version_id, jr.created_by, jr.batch_id, COALESCE(NULLIF(s.concurrency_key, ''), jr.concurrency_key), COALESCE(NULLIF(s.execution_mode, ''), jr.execution_mode), jr.is_rollback, jr.replayed_run_id
		FROM job_runs jr`

	args := []any{projectID}
	param := 2

	if environmentID != nil && *environmentID != "" {
		baseQuery += " JOIN jobs j ON j.id = jr.job_id"
	}

	baseQuery += `
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
		) metadata_delta ON true`

	baseQuery += " WHERE jr.project_id = $1"

	if status != nil {
		baseQuery += fmt.Sprintf(" AND COALESCE(s.status, jr.status) = $%d", param)
		args = append(args, string(*status))
		param++
	} else if len(statuses) > 0 {
		statusVals := make([]string, 0, len(statuses))
		for _, s := range statuses {
			statusVals = append(statusVals, string(s))
		}
		baseQuery += fmt.Sprintf(" AND COALESCE(s.status, jr.status) = ANY($%d)", param)
		args = append(args, statusVals)
		param++
	}

	if tagKey != "" {
		if tagValue == "" {
			baseQuery += fmt.Sprintf(" AND jr.tags ? $%d", param)
			args = append(args, tagKey)
			param++
		} else {
			baseQuery += fmt.Sprintf(" AND jr.tags ->> $%d = $%d", param, param+1)
			args = append(args, tagKey, tagValue)
			param += 2
		}
	}

	if environmentID != nil && *environmentID != "" {
		baseQuery += fmt.Sprintf(" AND j.environment_id = $%d", param)
		args = append(args, *environmentID)
		param++
	}

	if metadataKey != nil {
		if metadataValue == nil {
			baseQuery += fmt.Sprintf(" AND (COALESCE(jr.metadata, '{}'::jsonb) || COALESCE(metadata_delta.metadata, '{}'::jsonb)) ? $%d", param)
			args = append(args, *metadataKey)
			param++
		} else {
			baseQuery += fmt.Sprintf(" AND (COALESCE(jr.metadata, '{}'::jsonb) || COALESCE(metadata_delta.metadata, '{}'::jsonb)) ->> $%d = $%d", param, param+1)
			args = append(args, *metadataKey, *metadataValue)
			param += 2
		}
	}

	if triggeredBy != nil {
		baseQuery += fmt.Sprintf(" AND jr.triggered_by = $%d", param)
		args = append(args, *triggeredBy)
		param++
	}

	if batchID != nil {
		baseQuery += fmt.Sprintf(" AND jr.batch_id = $%d", param)
		args = append(args, *batchID)
		param++
	}

	if len(payloadContains) > 0 {
		baseQuery += fmt.Sprintf(" AND jr.payload @> $%d::jsonb", param)
		args = append(args, payloadContains)
		param++
	}

	if executionMode != nil {
		baseQuery += fmt.Sprintf(" AND COALESCE(NULLIF(s.execution_mode, ''), jr.execution_mode) = $%d", param)
		args = append(args, string(*executionMode))
		param++
	}

	if errorClass != nil {
		baseQuery += fmt.Sprintf(" AND CASE WHEN terminal.fields ? 'error_class' THEN terminal.fields->>'error_class' ELSE jr.error_class END = $%d", param)
		args = append(args, *errorClass)
		param++
	}

	if cursor != nil {
		baseQuery += fmt.Sprintf(" AND jr.created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	baseQuery += fmt.Sprintf(" ORDER BY jr.created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("list runs by project filtered: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, limit)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list runs by project filtered scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runs by project filtered rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListFinishedRunsSince(ctx context.Context, projectID string, since time.Time, sinceRunID string, limit int) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListFinishedRunsSince")
	defer span.End()

	if limit <= 0 {
		limit = 100
	}

	query := `
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
		  AND COALESCE(s.status, jr.status) IN ('completed', 'failed', 'timed_out', 'crashed', 'system_failed', 'canceled', 'expired')
		  AND (COALESCE(s.finished_at, jr.finished_at) > $2 OR (COALESCE(s.finished_at, jr.finished_at) = $2 AND jr.id > $3))
		ORDER BY COALESCE(s.finished_at, jr.finished_at) ASC, jr.id ASC
		LIMIT $4
	`

	rows, err := q.db.Query(ctx, query, projectID, since, sinceRunID, limit)
	if err != nil {
		return nil, fmt.Errorf("list finished runs since: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, limit)
	for rows.Next() {
		run, scanErr := dbscan.ScanRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list finished runs since scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}
	return runs, rows.Err()
}

// SnoozeRunWithLock transitions a run from `from` to `to` while holding a
// pessimistic row lock obtained via SELECT ... FOR UPDATE SKIP LOCKED. If
// another transaction currently holds the row (reaper, completion, sibling
// worker), the function returns ErrRunLocked so callers can no-op cleanly
// instead of dueling for the status update.
//
// The caller observes one of:
//   - nil: row was locked and successfully transitioned.
//   - ErrRunLocked: row exists in `from` but is held by another tx.
//   - ErrRunConflict: row exists but is no longer in `from`.
//   - ErrRunNotFound: row does not exist.
//   - any other error: genuine DB failure.
func (q *Queries) SnoozeRunWithLock(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SnoozeRunWithLock")
	defer span.End()

	_, ok := q.db.(TxBeginner)
	if !ok {
		return fmt.Errorf("snooze run with lock: underlying db does not support transactions")
	}

	return q.withTx(ctx, func(txQ *Queries) error {
		var locked string
		err := txQ.db.QueryRow(ctx, `
			SELECT run_id FROM job_run_state
			WHERE run_id = $1 AND status = $2
			FOR UPDATE SKIP LOCKED`, id, from).Scan(&locked)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				var actual domain.RunStatus
				statusErr := txQ.db.QueryRow(ctx, `
					SELECT COALESCE(s.status, jr.status)
					FROM job_runs jr
					LEFT JOIN job_run_read_state s ON s.run_id = jr.id
					WHERE jr.id = $1`,
					id,
				).Scan(&actual)
				if statusErr != nil {
					if errors.Is(statusErr, pgx.ErrNoRows) {
						return ErrRunNotFound
					}
					return fmt.Errorf("snooze run with lock: disambiguate: %w", statusErr)
				}
				if actual != from {
					return fmt.Errorf("%w: id %s from %s actual %s", ErrRunConflict, id, from, actual)
				}
				return ErrRunLocked
			}
			return fmt.Errorf("snooze run with lock: select: %w", err)
		}
		return txQ.UpdateRunStatus(ctx, id, from, to, fields)
	})
}

func (q *Queries) UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateRunStatus")
	defer span.End()

	if err := domain.ValidateTransition(from, to); err != nil {
		return fmt.Errorf("invalid status transition: %w", err)
	}

	if err := validateRunStatusFields(fields); err != nil {
		return err
	}

	updatedState, err := q.tryUpdateRunStateStatus(ctx, id, from, to, fields, nil)
	if err != nil {
		return err
	}
	if updatedState {
		return nil
	}

	currentStatus, _, err := q.currentRunMutableState(ctx, id)
	if err != nil {
		return fmt.Errorf("checking current status: %w", err)
	}
	if currentStatus == to {
		return nil // idempotent: already in target state
	}
	return fmt.Errorf("%w: id %s from %s", ErrRunConflict, id, from)
}

func (q *Queries) tryUpdateRunStateStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any, attempt *int) (bool, error) {
	if terminalRunStateShouldReactivate(from, to) {
		if _, ok := q.db.(TxBeginner); ok {
			moved := false
			err := q.withTx(ctx, func(txQ *Queries) error {
				var eventAttempt int
				var reactivateErr error
				moved, eventAttempt, reactivateErr = txQ.reactivateRunTerminalState(ctx, id, from, to, fields, attempt)
				if reactivateErr != nil || !moved {
					return reactivateErr
				}
				if from == domain.StatusDeadLetter {
					if err := txQ.decrementVisibleDLQCountForRun(ctx, id); err != nil {
						return err
					}
				}
				if err := txQ.bumpRunCacheVersion(ctx, id); err != nil {
					return err
				}
				if err := txQ.appendRunLifecycleEvent(ctx, id, from, to, fields, &eventAttempt); err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				return false, err
			}
			if moved {
				return true, nil
			}
		}
	}

	if terminalRunStateShouldMove(to) {
		if _, ok := q.db.(TxBeginner); ok {
			moved := false
			err := q.withTx(ctx, func(txQ *Queries) error {
				var eventAttempt int
				var appendErr error
				moved, eventAttempt, appendErr = txQ.appendRunTerminalState(ctx, id, from, to, fields, attempt)
				if appendErr != nil || !moved {
					return appendErr
				}
				if to == domain.StatusDeadLetter {
					if err := txQ.incrementVisibleDLQCountForRun(ctx, id); err != nil {
						return err
					}
				}
				if err := txQ.bumpRunCacheVersion(ctx, id); err != nil {
					return err
				}
				if err := txQ.appendRunLifecycleEvent(ctx, id, from, to, fields, &eventAttempt); err != nil {
					return err
				}
				ledgerFields := runLedgerFields(fields)
				if len(ledgerFields) > 0 {
					if err := txQ.updateRunLedgerFields(ctx, id, ledgerFields); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return false, err
			}
			if moved {
				return true, nil
			}
		}
	}

	if activeClaimRunStateShouldRequeue(from, to) {
		moved, err := q.tryRequeueActiveClaimRunState(ctx, id, from, to, fields, attempt)
		if err != nil {
			return false, err
		}
		if moved {
			return true, nil
		}
	}

	stateColumns := map[string]struct{}{
		"attempt":         {},
		"scheduled_at":    {},
		"started_at":      {},
		"finished_at":     {},
		"heartbeat_at":    {},
		"next_retry_at":   {},
		"expires_at":      {},
		"priority":        {},
		"concurrency_key": {},
		"execution_mode":  {},
	}
	ledgerColumns := map[string]struct{}{
		"payload":              {},
		"triggered_by":         {},
		"workflow_step_run_id": {},
		"debug_mode":           {},
		"continuation_of":      {},
		"lineage_depth":        {},
	}

	stateSet := []string{"status = $1", "updated_at = NOW()"}
	if from == domain.StatusWaiting && to == domain.StatusQueued {
		stateSet = append(stateSet, "ready_generation = ready_generation + 1")
	}
	if activeClaimRunStateShouldRequeue(from, to) {
		stateSet = append(stateSet, "ready_generation = ready_generation + 1")
	}
	stateArgs := []any{to, id, from}
	stateParam := 4
	ledgerFields := make(map[string]any)

	keys := lo.Keys(fields)
	sort.Strings(keys)
	for _, key := range keys {
		value := fields[key]
		if _, ok := stateColumns[key]; ok {
			if key == "concurrency_key" || key == "execution_mode" {
				if text, ok := value.(string); ok {
					value = dbscan.NilIfEmptyString(text)
				}
			}
			stateSet = append(stateSet, fmt.Sprintf("%s = $%d", key, stateParam))
			stateArgs = append(stateArgs, value)
			stateParam++
			continue
		}
		if _, ok := ledgerColumns[key]; ok {
			ledgerFields[key] = value
		}
	}

	var query string
	if attempt != nil {
		query = fmt.Sprintf("UPDATE job_run_state SET %s WHERE run_id = $2 AND status = $3 AND attempt = $%d", strings.Join(stateSet, ", "), stateParam)
		stateArgs = append(stateArgs, *attempt)
	} else {
		query = fmt.Sprintf("UPDATE job_run_state SET %s WHERE run_id = $2 AND status = $3", strings.Join(stateSet, ", "))
	}
	tag, err := q.db.Exec(ctx, query, stateArgs...)
	if err != nil {
		return false, fmt.Errorf("update run state status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}

	if err := q.bumpRunCacheVersion(ctx, id); err != nil {
		return false, err
	}
	if err := q.appendRunLifecycleEvent(ctx, id, from, to, fields, attempt); err != nil {
		return false, err
	}
	if len(ledgerFields) > 0 {
		if err := q.updateRunLedgerFields(ctx, id, ledgerFields); err != nil {
			return false, err
		}
	}
	return true, nil
}

func activeClaimRunStateShouldRequeue(from, to domain.RunStatus) bool {
	return to == domain.StatusQueued && (from == domain.StatusExecuting || from == domain.StatusDequeued)
}

func (q *Queries) tryRequeueActiveClaimRunState(
	ctx context.Context,
	id string,
	from, to domain.RunStatus,
	fields map[string]any,
	attempt *int,
) (bool, error) {
	if _, ok := q.db.(TxBeginner); !ok {
		return false, nil
	}

	moved := false
	err := q.withTx(ctx, func(txQ *Queries) error {
		stateColumns := map[string]struct{}{
			"attempt":         {},
			"scheduled_at":    {},
			"started_at":      {},
			"finished_at":     {},
			"heartbeat_at":    {},
			"next_retry_at":   {},
			"expires_at":      {},
			"priority":        {},
			"concurrency_key": {},
			"execution_mode":  {},
		}
		stateSet := []string{"status = $1", "updated_at = NOW()", "ready_generation = s.ready_generation + 1"}
		args := []any{to, id, nil}
		param := 4
		if attempt != nil {
			args[2] = *attempt
		}

		ledgerFields := make(map[string]any)
		keys := lo.Keys(fields)
		sort.Strings(keys)
		for _, key := range keys {
			value := fields[key]
			if _, ok := stateColumns[key]; ok {
				if key == "concurrency_key" || key == "execution_mode" {
					if text, ok := value.(string); ok {
						value = dbscan.NilIfEmptyString(text)
					}
				}
				stateSet = append(stateSet, fmt.Sprintf("%s = $%d", key, param))
				args = append(args, value)
				param++
				continue
			}
			if _, ok := runLedgerFields(fields)[key]; ok {
				ledgerFields[key] = value
			}
		}

		query := fmt.Sprintf(`
			WITH selected AS MATERIALIZED (
				SELECT
					s.run_id,
					COALESCE(c.attempt, s.attempt) AS event_attempt,
					s.ready_generation
				FROM job_run_state s
				JOIN job_run_active_claims c
				  ON c.run_id = s.run_id
				 AND c.ready_generation = s.ready_generation
				WHERE s.run_id = $2
					  AND s.status = 'queued'
					  AND NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
					  AND ($3::int IS NULL OR c.attempt = $3)
					FOR UPDATE OF s
			),
			updated AS (
				UPDATE job_run_state s
				SET %s
				FROM selected picked
				WHERE s.run_id = picked.run_id
				RETURNING picked.event_attempt
				)
				SELECT event_attempt FROM updated`,
			strings.Join(stateSet, ", "),
		)

		var eventAttempt int
		if scanErr := txQ.db.QueryRow(ctx, query, args...).Scan(&eventAttempt); scanErr != nil {
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("requeue active claim run state: %w", scanErr)
		}
		if err := txQ.bumpRunCacheVersion(ctx, id); err != nil {
			return err
		}
		if err := txQ.appendRunLifecycleEvent(ctx, id, from, to, fields, &eventAttempt); err != nil {
			return err
		}
		if len(ledgerFields) > 0 {
			if err := txQ.updateRunLedgerFields(ctx, id, ledgerFields); err != nil {
				return err
			}
		}
		moved = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return moved, nil
}

func (q *Queries) bumpRunCacheVersion(ctx context.Context, id string) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO job_run_cache_versions (run_id, cache_version)
		VALUES ($1, strait_next_run_cache_version($1))`,
		id,
	)
	if err != nil {
		return fmt.Errorf("bump run cache version: %w", err)
	}
	return nil
}

func terminalRunStateShouldMove(status domain.RunStatus) bool {
	return status.IsTerminal()
}

func terminalRunStateShouldReactivate(from, to domain.RunStatus) bool {
	return from == domain.StatusDeadLetter && !to.IsTerminal()
}

func runLedgerFields(fields map[string]any) map[string]any {
	ledgerColumns := map[string]struct{}{
		"payload":              {},
		"triggered_by":         {},
		"workflow_step_run_id": {},
		"debug_mode":           {},
		"continuation_of":      {},
		"lineage_depth":        {},
	}
	ledgerFields := make(map[string]any)
	for _, key := range lo.Keys(fields) {
		if _, ok := ledgerColumns[key]; ok {
			ledgerFields[key] = fields[key]
		}
	}
	return ledgerFields
}

func (q *Queries) appendRunTerminalState(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any, attempt *int) (bool, int, error) {
	query := appendRunTerminalStateQuery
	args := []any{
		id,
		from,
		to,
		fieldValue(fields, "priority"),
		fieldValue(fields, "scheduled_at"),
		fieldValue(fields, "started_at"),
		fieldValue(fields, "finished_at"),
		fieldValue(fields, "heartbeat_at"),
		fieldValue(fields, "next_retry_at"),
		fieldValue(fields, "expires_at"),
		normalizedTextField(fields, "concurrency_key"),
		normalizedTextField(fields, "execution_mode"),
	}
	if attempt != nil {
		args = append(args, *attempt)
		query = appendRunTerminalStateForAttemptQuery
	}

	var eventAttempt int
	err := q.db.QueryRow(ctx, query, args...).Scan(&eventAttempt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, 0, nil
		}
		return false, 0, fmt.Errorf("append run terminal state: %w", err)
	}
	return true, eventAttempt, nil
}

func (q *Queries) reactivateRunTerminalState(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any, attempt *int) (bool, int, error) {
	var eventAttempt int
	deleteQuery := `DELETE FROM job_run_terminal_state WHERE run_id = $1 AND status = $2 RETURNING attempt`
	args := []any{id, from}
	if attempt != nil {
		deleteQuery = `DELETE FROM job_run_terminal_state WHERE run_id = $1 AND status = $2 AND attempt = $3 RETURNING attempt`
		args = append(args, *attempt)
	}
	if err := q.db.QueryRow(ctx, deleteQuery, args...).Scan(&eventAttempt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, 0, nil
		}
		return false, 0, fmt.Errorf("reactivate terminal run state: %w", err)
	}

	stateSet := []string{"status = $1", "updated_at = NOW()", "ready_generation = ready_generation + 1"}
	stateArgs := []any{to, id}
	stateParam := 3
	stateColumns := map[string]struct{}{
		"attempt":         {},
		"scheduled_at":    {},
		"started_at":      {},
		"finished_at":     {},
		"heartbeat_at":    {},
		"next_retry_at":   {},
		"expires_at":      {},
		"priority":        {},
		"concurrency_key": {},
		"execution_mode":  {},
	}
	keys := lo.Keys(fields)
	sort.Strings(keys)
	for _, key := range keys {
		value := fields[key]
		if _, ok := stateColumns[key]; !ok {
			continue
		}
		if key == "concurrency_key" || key == "execution_mode" {
			if text, ok := value.(string); ok {
				value = dbscan.NilIfEmptyString(text)
			}
		}
		stateSet = append(stateSet, fmt.Sprintf("%s = $%d", key, stateParam))
		stateArgs = append(stateArgs, value)
		stateParam++
	}

	query := fmt.Sprintf("UPDATE job_run_state SET %s WHERE run_id = $2", strings.Join(stateSet, ", "))
	tag, err := q.db.Exec(ctx, query, stateArgs...)
	if err != nil {
		return false, 0, fmt.Errorf("reactivate run state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, 0, fmt.Errorf("%w: %s", ErrRunNotFound, id)
	}
	return true, eventAttempt, nil
}

const appendRunTerminalStateQuery = `
	WITH selected AS (
		SELECT
			s.run_id,
			s.project_id,
			s.job_id,
			CASE
				WHEN c.run_id IS NOT NULL AND s.status IN ('queued', 'delayed') THEN 'executing'
				WHEN c.run_id IS NOT NULL AND s.status = 'paused' AND ready.reason = 'paused_resume' THEN 'executing'
				WHEN ready.reason IN ('delayed_due', 'worker_recovered') AND s.status = 'delayed' THEN 'queued'
				ELSE s.status
			END AS previous_status,
			COALESCE(c.attempt, ready.attempt, s.attempt) AS attempt,
			s.priority,
			s.scheduled_at,
			COALESCE(c.started_at, s.started_at) AS started_at,
			CASE WHEN ready.reason = 'retry_ready' THEN NULL::TIMESTAMPTZ ELSE s.finished_at END AS finished_at,
			CASE WHEN ready.reason = 'retry_ready' THEN NULL::TIMESTAMPTZ ELSE s.heartbeat_at END AS heartbeat_at,
			CASE WHEN ready.reason = 'retry_ready' THEN NULL::TIMESTAMPTZ ELSE s.next_retry_at END AS next_retry_at,
			s.expires_at,
			s.concurrency_key,
			s.execution_mode,
			s.queue_name,
			s.job_enabled,
			s.job_paused,
			s.job_max_concurrency,
			s.job_max_concurrency_per_key,
			s.ready_generation,
			c.run_id IS NOT NULL AS uses_active_claim
		FROM job_run_state s
		LEFT JOIN job_run_active_claims c
		  ON c.run_id = s.run_id
		 AND c.ready_generation = s.ready_generation
		LEFT JOIN LATERAL (
			SELECT e.attempt, e.reason
			FROM job_run_ready_events e
			WHERE e.run_id = s.run_id
			  AND e.ready_generation = s.ready_generation
			ORDER BY e.id DESC
			LIMIT 1
		) ready ON true
		WHERE s.run_id = $1
		  AND (
		      s.status = $2
		      OR ($2 = 'executing' AND s.status IN ('queued', 'delayed') AND c.run_id IS NOT NULL)
		      OR ($2 = 'executing' AND s.status = 'paused' AND c.run_id IS NOT NULL AND ready.reason = 'paused_resume')
		      OR ($2 = 'queued' AND s.status = 'delayed' AND ready.reason IN ('delayed_due', 'worker_recovered'))
		  )
		  AND NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
		FOR UPDATE OF s
	),
	inserted AS (
		INSERT INTO job_run_terminal_state (
			run_id,
			project_id,
			job_id,
			status,
			attempt,
			priority,
			scheduled_at,
			started_at,
			finished_at,
			heartbeat_at,
			next_retry_at,
			expires_at,
			concurrency_key,
			execution_mode,
			queue_name,
			job_enabled,
			job_paused,
			job_max_concurrency,
			job_max_concurrency_per_key,
			ready_generation,
			updated_at
		)
		SELECT
			run_id,
			project_id,
			job_id,
			$3,
			attempt,
			COALESCE($4::INT, priority),
			COALESCE($5::TIMESTAMPTZ, scheduled_at),
			COALESCE($6::TIMESTAMPTZ, started_at),
			COALESCE($7::TIMESTAMPTZ, finished_at),
			COALESCE($8::TIMESTAMPTZ, heartbeat_at),
			COALESCE($9::TIMESTAMPTZ, next_retry_at),
			COALESCE($10::TIMESTAMPTZ, expires_at),
			COALESCE($11::TEXT, concurrency_key),
			COALESCE($12::TEXT, execution_mode),
			queue_name,
			job_enabled,
			job_paused,
			job_max_concurrency,
			job_max_concurrency_per_key,
			ready_generation,
			NOW()
		FROM selected
		ON CONFLICT (run_id) DO NOTHING
		RETURNING run_id, attempt
	),
	released AS (
		UPDATE job_active_counts c
		SET count = GREATEST(c.count - 1, 0),
		    updated_at = NOW()
		FROM selected s
		JOIN inserted i ON i.run_id = s.run_id
		WHERE s.previous_status IN ('dequeued', 'executing')
		  AND NOT s.uses_active_claim
		  AND (s.job_max_concurrency IS NOT NULL OR s.job_max_concurrency_per_key IS NOT NULL)
		  AND c.job_id = s.job_id
		  AND c.concurrency_key = COALESCE(s.concurrency_key, '')
		  AND c.count <> 0
		RETURNING 1
	)
	SELECT attempt FROM inserted`

const appendRunTerminalStateForAttemptQuery = `
	WITH selected AS (
		SELECT
			s.run_id,
			s.project_id,
			s.job_id,
			CASE
				WHEN c.run_id IS NOT NULL AND s.status IN ('queued', 'delayed') THEN 'executing'
				WHEN c.run_id IS NOT NULL AND s.status = 'paused' AND ready.reason = 'paused_resume' THEN 'executing'
				WHEN ready.reason IN ('delayed_due', 'worker_recovered') AND s.status = 'delayed' THEN 'queued'
				ELSE s.status
			END AS previous_status,
			COALESCE(c.attempt, ready.attempt, s.attempt) AS attempt,
			s.priority,
			s.scheduled_at,
			COALESCE(c.started_at, s.started_at) AS started_at,
			CASE WHEN ready.reason = 'retry_ready' THEN NULL::TIMESTAMPTZ ELSE s.finished_at END AS finished_at,
			CASE WHEN ready.reason = 'retry_ready' THEN NULL::TIMESTAMPTZ ELSE s.heartbeat_at END AS heartbeat_at,
			CASE WHEN ready.reason = 'retry_ready' THEN NULL::TIMESTAMPTZ ELSE s.next_retry_at END AS next_retry_at,
			s.expires_at,
			s.concurrency_key,
			s.execution_mode,
			s.queue_name,
			s.job_enabled,
			s.job_paused,
			s.job_max_concurrency,
			s.job_max_concurrency_per_key,
			s.ready_generation,
			c.run_id IS NOT NULL AS uses_active_claim
		FROM job_run_state s
		LEFT JOIN job_run_active_claims c
		  ON c.run_id = s.run_id
		 AND c.ready_generation = s.ready_generation
		LEFT JOIN LATERAL (
			SELECT e.attempt, e.reason
			FROM job_run_ready_events e
			WHERE e.run_id = s.run_id
			  AND e.ready_generation = s.ready_generation
			ORDER BY e.id DESC
			LIMIT 1
		) ready ON true
		WHERE s.run_id = $1
		  AND COALESCE(c.attempt, ready.attempt, s.attempt) = $13
		  AND (
		      s.status = $2
		      OR ($2 = 'executing' AND s.status IN ('queued', 'delayed') AND c.run_id IS NOT NULL)
		      OR ($2 = 'executing' AND s.status = 'paused' AND c.run_id IS NOT NULL AND ready.reason = 'paused_resume')
		      OR ($2 = 'queued' AND s.status = 'delayed' AND ready.reason IN ('delayed_due', 'worker_recovered'))
		  )
		  AND NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
		FOR UPDATE OF s
	),
	inserted AS (
		INSERT INTO job_run_terminal_state (
			run_id,
			project_id,
			job_id,
			status,
			attempt,
			priority,
			scheduled_at,
			started_at,
			finished_at,
			heartbeat_at,
			next_retry_at,
			expires_at,
			concurrency_key,
			execution_mode,
			queue_name,
			job_enabled,
			job_paused,
			job_max_concurrency,
			job_max_concurrency_per_key,
			ready_generation,
			updated_at
		)
		SELECT
			run_id,
			project_id,
			job_id,
			$3,
			attempt,
			COALESCE($4::INT, priority),
			COALESCE($5::TIMESTAMPTZ, scheduled_at),
			COALESCE($6::TIMESTAMPTZ, started_at),
			COALESCE($7::TIMESTAMPTZ, finished_at),
			COALESCE($8::TIMESTAMPTZ, heartbeat_at),
			COALESCE($9::TIMESTAMPTZ, next_retry_at),
			COALESCE($10::TIMESTAMPTZ, expires_at),
			COALESCE($11::TEXT, concurrency_key),
			COALESCE($12::TEXT, execution_mode),
			queue_name,
			job_enabled,
			job_paused,
			job_max_concurrency,
			job_max_concurrency_per_key,
			ready_generation,
			NOW()
		FROM selected
		ON CONFLICT (run_id) DO NOTHING
		RETURNING run_id, attempt
	),
	released AS (
		UPDATE job_active_counts c
		SET count = GREATEST(c.count - 1, 0),
		    updated_at = NOW()
		FROM selected s
		JOIN inserted i ON i.run_id = s.run_id
		WHERE s.previous_status IN ('dequeued', 'executing')
		  AND NOT s.uses_active_claim
		  AND (s.job_max_concurrency IS NOT NULL OR s.job_max_concurrency_per_key IS NOT NULL)
		  AND c.job_id = s.job_id
		  AND c.concurrency_key = COALESCE(s.concurrency_key, '')
		  AND c.count <> 0
		RETURNING 1
	)
	SELECT attempt FROM inserted`

func fieldValue(fields map[string]any, key string) any {
	if len(fields) == 0 {
		return nil
	}
	return fields[key]
}

func normalizedTextField(fields map[string]any, key string) any {
	value, ok := fields[key]
	if !ok {
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return value
	}
	return dbscan.NilIfEmptyString(text)
}

func validateRunStatusFields(fields map[string]any) error {
	allowedColumns := map[string]struct{}{
		"attempt":              {},
		"payload":              {},
		"result":               {},
		"error":                {},
		"error_class":          {},
		"triggered_by":         {},
		"scheduled_at":         {},
		"started_at":           {},
		"finished_at":          {},
		"heartbeat_at":         {},
		"next_retry_at":        {},
		"expires_at":           {},
		"execution_trace":      {},
		"workflow_step_run_id": {},
		"debug_mode":           {},
		"continuation_of":      {},
		"lineage_depth":        {},
		"priority":             {},
		"metadata":             {},
	}

	keys := lo.Keys(fields)
	sort.Strings(keys)
	for _, key := range keys {
		if _, ok := allowedColumns[key]; !ok {
			return &domain.FieldError{Field: key}
		}
	}
	return nil
}

func (q *Queries) currentRunMutableState(ctx context.Context, id string) (domain.RunStatus, int, error) {
	var status domain.RunStatus
	var attempt int
	err := q.db.QueryRow(ctx, `
		SELECT COALESCE(s.status, jr.status), COALESCE(s.attempt, jr.attempt)
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,
		id,
	).Scan(&status, &attempt)
	if err != nil {
		return "", 0, err
	}
	return status, attempt, nil
}

func (q *Queries) appendRunLifecycleEvent(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any, attempt *int) error {
	eventFields, err := json.Marshal(normalizeRunLifecycleFields(fields))
	if err != nil {
		return fmt.Errorf("marshal run lifecycle fields: %w", err)
	}
	attemptValue := 0
	if attempt != nil {
		attemptValue = *attempt
	} else {
		_ = q.db.QueryRow(ctx, `SELECT attempt FROM job_run_state WHERE run_id = $1`, id).Scan(&attemptValue)
	}
	if attemptValue <= 0 {
		attemptValue = 1
	}
	if _, err := q.db.Exec(ctx, `
		INSERT INTO job_run_lifecycle_events (run_id, from_status, to_status, attempt, fields)
		VALUES ($1, $2, $3, $4, $5::jsonb)`,
		id, from, to, attemptValue, eventFields,
	); err != nil {
		return fmt.Errorf("append run lifecycle event: %w", err)
	}
	return nil
}

func normalizeRunLifecycleFields(fields map[string]any) map[string]any {
	if len(fields) == 0 {
		return fields
	}
	normalized := make(map[string]any, len(fields))
	for key, value := range fields {
		if key == "result" {
			if raw, ok := value.([]byte); ok {
				value = json.RawMessage(raw)
			}
		}
		normalized[key] = value
	}
	return normalized
}

func (q *Queries) updateRunLedgerFields(ctx context.Context, id string, fields map[string]any) error {
	setClauses := make([]string, 0, len(fields))
	changePredicates := make([]string, 0, len(fields))
	args := []any{id}
	param := 2
	keys := lo.Keys(fields)
	sort.Strings(keys)
	for _, key := range keys {
		value := fields[key]
		if raw, ok := value.(json.RawMessage); ok {
			value = dbscan.NilIfEmptyRawMessage(raw)
		}
		if key == "metadata" {
			if m, ok := value.(map[string]string); ok {
				encoded, err := json.Marshal(m)
				if err != nil {
					return fmt.Errorf("marshal metadata: %w", err)
				}
				setClauses = append(setClauses, fmt.Sprintf("metadata = COALESCE(metadata, '{}'::jsonb) || $%d::jsonb", param))
				changePredicates = append(changePredicates, fmt.Sprintf("COALESCE(metadata, '{}'::jsonb) IS DISTINCT FROM COALESCE(metadata, '{}'::jsonb) || $%d::jsonb", param))
				args = append(args, encoded)
				param++
				continue
			}
		}
		if key == "execution_trace" {
			switch trace := value.(type) {
			case *domain.ExecutionTrace:
				if trace == nil {
					value = nil
				} else {
					encoded, err := json.Marshal(trace)
					if err != nil {
						return fmt.Errorf("marshal execution trace: %w", err)
					}
					value = dbscan.NilIfEmptyRawMessage(encoded)
				}
			case domain.ExecutionTrace:
				encoded, err := json.Marshal(trace)
				if err != nil {
					return fmt.Errorf("marshal execution trace: %w", err)
				}
				value = dbscan.NilIfEmptyRawMessage(encoded)
			}
		}
		if key == "error" {
			if text, ok := value.(string); ok {
				value = dbscan.NilIfEmptyString(text)
			}
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, param))
		changePredicates = append(changePredicates, fmt.Sprintf("%s IS DISTINCT FROM $%d", key, param))
		args = append(args, value)
		param++
	}
	if len(setClauses) == 0 {
		return nil
	}
	query := fmt.Sprintf("UPDATE job_runs SET %s WHERE id = $1", strings.Join(setClauses, ", "))
	query += " AND (" + strings.Join(changePredicates, " OR ") + ")"
	if _, err := q.db.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("update run ledger fields: %w", err)
	}
	return nil
}

// UpdateRunStatusForActiveRun mirrors UpdateRunStatus but additionally
// constrains the WHERE clause to the supplied attempt. SDK terminal handlers
// route through this so a stale token (run retried, attempt advanced) cannot
// flip a fresh run's status. When zero rows are affected because the attempt
// no longer matches, the call is translated to ErrRunConflict.
func (q *Queries) UpdateRunStatusForActiveRun(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any, attempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateRunStatusForActiveRun")
	defer span.End()

	if err := domain.ValidateTransition(from, to); err != nil {
		return fmt.Errorf("invalid status transition: %w", err)
	}

	if err := validateRunStatusFields(fields); err != nil {
		return err
	}

	updatedState, err := q.tryUpdateRunStateStatus(ctx, id, from, to, fields, &attempt)
	if err != nil {
		return err
	}
	if updatedState {
		return nil
	}

	currentStatus, currentAttempt, err := q.currentRunMutableState(ctx, id)
	if err != nil {
		return fmt.Errorf("checking current status: %w", err)
	}
	if currentAttempt != attempt {
		return fmt.Errorf("%w: run %s active attempt %d, requested %d", ErrRunConflict, id, currentAttempt, attempt)
	}
	if currentStatus == to {
		return nil
	}
	return fmt.Errorf("%w: id %s from %s", ErrRunConflict, id, from)
}

func (q *Queries) UpdateRunMetadata(ctx context.Context, id string, annotations map[string]string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateRunMetadata")
	defer span.End()

	encoded, err := json.Marshal(annotations)
	if err != nil {
		return fmt.Errorf("marshal annotations: %w", err)
	}

	var found bool
	var updated bool
	if err := q.db.QueryRow(ctx, `
		WITH target AS MATERIALIZED (
			SELECT id, COALESCE(metadata, '{}'::jsonb) AS current_metadata
			FROM job_runs
			WHERE id = $2
		),
		updated AS (
			UPDATE job_runs jr
			SET metadata = target.current_metadata || $1::jsonb
			FROM target
			WHERE jr.id = target.id
			  AND target.current_metadata IS DISTINCT FROM target.current_metadata || $1::jsonb
			RETURNING jr.id
		)
		SELECT EXISTS(SELECT 1 FROM target), EXISTS(SELECT 1 FROM updated)`,
		encoded,
		id,
	).Scan(&found, &updated); err != nil {
		return fmt.Errorf("update run metadata: %w", err)
	}
	if !found {
		return fmt.Errorf("%w: %s", ErrRunNotFound, id)
	}
	return nil
}

func (q *Queries) UpdateRunMetadataForActiveRun(ctx context.Context, id string, annotations map[string]string, attempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateRunMetadataForActiveRun")
	defer span.End()

	encoded, err := json.Marshal(annotations)
	if err != nil {
		return fmt.Errorf("marshal annotations: %w", err)
	}

	query := `
		WITH gate AS MATERIALIZED (
			SELECT
				COALESCE(s.status, jr.status) AS from_status,
				COALESCE(s.status, jr.status) AS to_status,
				COALESCE(s.attempt, jr.attempt) AS current_attempt,
				COALESCE(jr.metadata, '{}'::jsonb) || COALESCE(metadata_delta.metadata, '{}'::jsonb) AS current_metadata
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			LEFT JOIN LATERAL (
				SELECT jsonb_object_agg(entry.key, entry.value ORDER BY e.created_at, e.id) AS metadata
				FROM job_run_lifecycle_events e
				CROSS JOIN LATERAL jsonb_each(COALESCE(e.fields->'metadata', '{}'::jsonb)) AS entry(key, value)
				WHERE e.run_id = jr.id
				  AND e.fields ? 'metadata'
			) metadata_delta ON true
			WHERE jr.id = $2
			  AND COALESCE(s.attempt, jr.attempt) = $3
			  AND COALESCE(s.status, jr.status) IN ('executing', 'waiting')
		),
		lifecycle_event AS (
			INSERT INTO job_run_lifecycle_events (run_id, from_status, to_status, attempt, fields)
			SELECT $2, from_status, to_status, current_attempt, jsonb_build_object('metadata', $1::jsonb)
			FROM gate
			WHERE current_metadata IS DISTINCT FROM current_metadata || $1::jsonb
			RETURNING run_id
		),
		cache_versions AS (
			INSERT INTO job_run_cache_versions (run_id, cache_version)
			SELECT run_id, strait_next_run_cache_version(run_id)
			FROM lifecycle_event
			RETURNING 1
		)
		SELECT EXISTS(SELECT 1 FROM gate), EXISTS(SELECT 1 FROM lifecycle_event)`

	var active bool
	var updated bool
	err = q.db.QueryRow(ctx, query, encoded, id, attempt).Scan(&active, &updated)
	if err != nil {
		return fmt.Errorf("update active run metadata: %w", err)
	}
	if !active {
		return fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, id, attempt)
	}

	return nil
}

func (q *Queries) UpdateHeartbeat(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateHeartbeat")
	defer span.End()

	query := `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		SELECT id, NOW(), FALSE
		FROM job_runs
		WHERE id = $1
		RETURNING run_id`

	var runID string
	err := q.db.QueryRow(ctx, query, id).Scan(&runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrRunNotFound, id)
		}
		return fmt.Errorf("update heartbeat: %w", err)
	}

	return nil
}

func (q *Queries) UpdateHeartbeatForActiveRun(ctx context.Context, id string, attempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateHeartbeatForActiveRun")
	defer span.End()

	query := `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		SELECT jr.id, NOW(), FALSE
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.id = $1
		  AND COALESCE(s.attempt, jr.attempt) = $2
		  AND COALESCE(s.status, jr.status) IN ('executing', 'waiting')
		RETURNING run_id`

	var runID string
	err := q.db.QueryRow(ctx, query, id, attempt).Scan(&runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, id, attempt)
		}
		return fmt.Errorf("update active run heartbeat: %w", err)
	}

	return nil
}

func (q *Queries) ListStaleRuns(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListStaleRuns")
	defer span.End()

	// Heartbeat liveness is sourced from the job_run_heartbeats side table,
	// which the worker tick path upserts. The claim path no longer writes
	// job_runs.heartbeat_at so that job_runs UPDATEs stay HOT (the column
	// is covered by idx_runs_project_executing). When no side-table row
	// exists yet — the window between claim and the first worker tick —
	// we fall back to started_at to avoid flagging a fresh run as stale.
	query := "/* action=reaper */ " + fmt.Sprintf(`
		SELECT r.id, r.job_id, r.project_id, s.status, s.attempt, r.payload, r.result, r.metadata, r.error, r.error_class,
		       r.triggered_by, s.scheduled_at, s.started_at, s.finished_at, COALESCE(hb.last_hb, s.heartbeat_at),
		       s.next_retry_at, s.expires_at, r.parent_run_id, s.priority, r.idempotency_key, r.job_version, r.created_at, r.workflow_step_run_id, r.execution_trace, r.debug_mode, r.continuation_of, r.lineage_depth, r.tags, r.job_version_id, r.created_by, r.batch_id, s.concurrency_key, s.execution_mode, r.is_rollback, r.replayed_run_id
		FROM job_runs r
		JOIN job_run_state s ON s.run_id = r.id
		LEFT JOIN LATERAL (
			SELECT heartbeat_at AS last_hb
			FROM job_run_heartbeats h
			WHERE h.run_id = r.id
			  AND h.cleared = FALSE
			ORDER BY h.id DESC
			LIMIT 1
		) hb ON true
		WHERE s.status = '%s'
		  AND s.execution_mode != 'worker'
		  AND NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
		  AND COALESCE(hb.last_hb, s.heartbeat_at, s.started_at) < NOW() - $1::interval
		  AND s.finished_at IS NULL
		  AND s.started_at IS NOT NULL
		ORDER BY COALESCE(hb.last_hb, s.heartbeat_at, s.started_at) ASC
		LIMIT 1000`, domain.StatusExecuting)

	rows, err := q.db.Query(ctx, query, threshold.String())
	if err != nil {
		return nil, fmt.Errorf("list stale runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, 16)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list stale runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list stale runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListDueRuns(ctx context.Context) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListDueRuns")
	defer span.End()

	query := fmt.Sprintf(`
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, is_rollback, replayed_run_id
		FROM job_runs
		WHERE status = '%s' AND scheduled_at <= NOW()
		ORDER BY scheduled_at ASC
		LIMIT 1000`, domain.StatusDelayed)

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list due runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, 16)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list due runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list due runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListExpiredRuns(ctx context.Context) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListExpiredRuns")
	defer span.End()

	query := fmt.Sprintf(`
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, is_rollback, replayed_run_id
		FROM job_runs
		WHERE status IN ('%s', '%s')
		  AND expires_at IS NOT NULL
		  AND expires_at <= NOW()
		ORDER BY expires_at ASC
		LIMIT 1000`, domain.StatusDelayed, domain.StatusQueued)

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list expired runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, 16)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list expired runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list expired runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListChildRuns(ctx context.Context, parentRunID string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListChildRuns")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, is_rollback, replayed_run_id
		FROM job_runs
		WHERE parent_run_id = $1`

	args := []any{parentRunID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at ASC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list child runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, 16)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list child runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list child runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListStaleDequeued(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListStaleDequeued")
	defer span.End()

	query := "/* action=reaper */ " + fmt.Sprintf(`
		SELECT r.id, r.job_id, r.project_id, s.status, s.attempt, r.payload, r.result, r.metadata, r.error, r.error_class,
		       r.triggered_by, s.scheduled_at, s.started_at, s.finished_at, s.heartbeat_at,
		       s.next_retry_at, s.expires_at, r.parent_run_id, s.priority, r.idempotency_key, r.job_version, r.created_at, r.workflow_step_run_id, r.execution_trace, r.debug_mode, r.continuation_of, r.lineage_depth, r.tags, r.job_version_id, r.created_by, r.batch_id, s.concurrency_key, s.execution_mode, r.is_rollback, r.replayed_run_id
		FROM job_runs r
		JOIN job_run_state s ON s.run_id = r.id
		WHERE s.status = '%s'
		  AND NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
		  AND s.started_at < NOW() - $1::interval
		  AND s.finished_at IS NULL
		  AND s.started_at IS NOT NULL
		ORDER BY s.started_at ASC
		LIMIT 1000`, domain.StatusDequeued)

	rows, err := q.db.Query(ctx, query, threshold.String())
	if err != nil {
		return nil, fmt.Errorf("list stale dequeued runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, 16)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list stale dequeued runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list stale dequeued runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) DeleteTerminalRunsPastRetention(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteTerminalRunsPastRetention")
	defer span.End()

	shortCutoff := time.Now().Add(-shortRetention)
	longCutoff := time.Now().Add(-longRetention)

	// Exclude the current month's partition to avoid creating dead tuples
	// in the hot partition that the dequeue hot path scans. Runs in cold
	// partitions (older months) do not affect dequeue performance.
	hotBoundary := beginningOfMonth(time.Now())

	query := "/* action=reaper */ " + `
		WITH to_delete AS (
			SELECT jr.id
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE COALESCE(s.finished_at, jr.finished_at) IS NOT NULL
			  AND jr.created_at < $3
			  AND (
				(COALESCE(s.status, jr.status) IN ('completed', 'failed', 'canceled', 'expired') AND COALESCE(s.finished_at, jr.finished_at) <= $1)
				OR
				(COALESCE(s.status, jr.status) IN ('timed_out', 'crashed', 'system_failed') AND COALESCE(s.finished_at, jr.finished_at) <= $2)
			  )
			LIMIT 5000
			FOR UPDATE OF jr SKIP LOCKED
		),
		deleted_active_claims AS (
			DELETE FROM job_run_active_claims
			WHERE run_id IN (SELECT id FROM to_delete)
		),
		deleted_lifecycle_events AS (
			DELETE FROM job_run_lifecycle_events
			WHERE run_id IN (SELECT id FROM to_delete)
		),
		deleted_ready_events AS (
			DELETE FROM job_run_ready_events
			WHERE run_id IN (SELECT id FROM to_delete)
		),
		deleted_retries AS (
			DELETE FROM job_retries
			WHERE run_id IN (SELECT id FROM to_delete)
		),
		deleted_priority_events AS (
			DELETE FROM job_run_priority_events
			WHERE run_id IN (SELECT id FROM to_delete)
		),
		deleted_visibility_events AS (
			DELETE FROM job_run_visibility_events
			WHERE run_id IN (SELECT id FROM to_delete)
		),
		deleted_cache_versions AS (
			DELETE FROM job_run_cache_versions
			WHERE run_id IN (SELECT id FROM to_delete)
		),
		deleted_heartbeats AS (
			DELETE FROM job_run_heartbeats
			WHERE run_id IN (SELECT id FROM to_delete)
		),
		deleted_terminal_state AS (
			DELETE FROM job_run_terminal_state
			WHERE run_id IN (SELECT id FROM to_delete)
		)
		DELETE FROM job_runs WHERE id IN (SELECT id FROM to_delete)`

	tag, err := q.db.Exec(ctx, query, shortCutoff, longCutoff, hotBoundary)
	if err != nil {
		return 0, fmt.Errorf("delete terminal runs past retention: %w", err)
	}

	return tag.RowsAffected(), nil
}

// beginningOfMonth returns midnight on the first day of t's month in UTC.
// Converts t to UTC first so the calendar fields (Year, Month) are correct
// regardless of the host's local timezone.
func beginningOfMonth(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// QueueJobDepth represents the queue depth for a single job.
type QueueJobDepth struct {
	JobID                    string
	QueuedCount              int
	QueueDepthAlertThreshold int
}

func (q *Queries) ListQueueDepthByJob(ctx context.Context) ([]QueueJobDepth, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListQueueDepthByJob")
	defer span.End()

	query := `
		SELECT jr.job_id, COUNT(*) AS queued_count, j.queue_depth_alert_threshold
		FROM job_runs jr
		JOIN jobs j ON j.id = jr.job_id
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE COALESCE(s.status, jr.status) = 'queued'
		  AND j.queue_depth_alert_threshold IS NOT NULL
		GROUP BY jr.job_id, j.queue_depth_alert_threshold
		HAVING COUNT(*) >= j.queue_depth_alert_threshold`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list queue depth by job: %w", err)
	}
	defer rows.Close()

	var results []QueueJobDepth
	for rows.Next() {
		var d QueueJobDepth
		if err := rows.Scan(&d.JobID, &d.QueuedCount, &d.QueueDepthAlertThreshold); err != nil {
			return nil, fmt.Errorf("scan queue depth: %w", err)
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

func (q *Queries) GetDebugBundle(ctx context.Context, runID string) (*domain.DebugBundle, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetDebugBundle")
	defer span.End()

	// Refuse to materialize a debug bundle for runs that the DLQ
	// age-out / explicit mask flow has hidden. Masked rows carry rich
	// PII (raw payloads, prompts, tool outputs) that operators have
	// already chosen to take out of circulation; surfacing them via
	// the debug endpoint would defeat that decision. UnmaskDLQRun is
	// the supported path back to visibility.
	var visibleUntil *time.Time
	if err := q.db.QueryRow(ctx, `
		SELECT CASE WHEN COALESCE(visibility.has_event, FALSE)
		            THEN visibility.visible_until
		            ELSE jr.visible_until
		       END
		FROM job_runs jr
		LEFT JOIN LATERAL (
			SELECT e.visible_until, TRUE AS has_event
			FROM job_run_visibility_events e
			WHERE e.run_id = jr.id
			ORDER BY e.id DESC
			LIMIT 1
		) visibility ON TRUE
		WHERE jr.id = $1
	`, runID).Scan(&visibleUntil); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("get debug bundle visibility: %w", err)
	}
	if visibleUntil != nil && !visibleUntil.After(time.Now()) {
		return nil, ErrRunNotFound
	}

	run, err := q.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}

	events, err := q.ListEvents(ctx, runID, 10000, nil)
	if err != nil {
		return nil, fmt.Errorf("get debug bundle events: %w", err)
	}

	checkpoints, err := q.ListRunCheckpoints(ctx, runID, 1000, nil)
	if err != nil {
		return nil, fmt.Errorf("get debug bundle checkpoints: %w", err)
	}

	outputs, err := q.ListRunOutputs(ctx, runID, 10000, nil)
	if err != nil {
		return nil, fmt.Errorf("get debug bundle outputs: %w", err)
	}

	resourceSnapshots, err := q.ListRunResourceSnapshots(ctx, runID, nil, nil, 1000)
	if err != nil {
		return nil, fmt.Errorf("get debug bundle resource snapshots: %w", err)
	}

	return &domain.DebugBundle{
		Run:               run,
		Events:            events,
		Checkpoints:       checkpoints,
		Outputs:           outputs,
		ResourceSnapshots: resourceSnapshots,
	}, nil
}

func (q *Queries) UpdateRunDebugMode(ctx context.Context, runID string, debugMode bool) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateRunDebugMode")
	defer span.End()

	var found bool
	var updated bool
	if err := q.db.QueryRow(ctx, `
		WITH target AS MATERIALIZED (
			SELECT id
			FROM job_runs
			WHERE id = $2
		),
		updated AS (
			UPDATE job_runs
			SET debug_mode = $1
			WHERE id = $2
			  AND debug_mode IS DISTINCT FROM $1
			RETURNING id
		)
		SELECT EXISTS(SELECT 1 FROM target), EXISTS(SELECT 1 FROM updated)`,
		debugMode,
		runID,
	).Scan(&found, &updated); err != nil {
		return fmt.Errorf("update run debug mode: %w", err)
	}
	if !found {
		return fmt.Errorf("%w: %s", ErrRunNotFound, runID)
	}
	return nil
}

func (q *Queries) ListRunLineage(ctx context.Context, runID string, limit int, _ *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunLineage")
	defer span.End()

	// Walk backward to find the root of the lineage chain.
	rootID := runID
	for range 20 { // safety bound
		var continuationOf *string
		err := q.db.QueryRow(ctx, "SELECT continuation_of FROM job_runs WHERE id = $1", rootID).Scan(&continuationOf)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				break
			}
			return nil, fmt.Errorf("list run lineage walk: %w", err)
		}
		if continuationOf == nil || *continuationOf == "" {
			break
		}
		rootID = *continuationOf
	}

	// Walk forward from root via recursive CTE.
	query := `
		WITH RECURSIVE lineage AS (
			SELECT id
			FROM job_runs
			WHERE id = $1
			UNION ALL
			SELECT jr.id
			FROM job_runs jr
			JOIN lineage l ON jr.continuation_of = l.id
		)
		SELECT jr.id, jr.job_id, jr.project_id, COALESCE(s.status, jr.status), COALESCE(s.attempt, jr.attempt), jr.payload,
		       CASE WHEN terminal.fields ? 'result' THEN terminal.fields->'result' ELSE jr.result END,
		       COALESCE(jr.metadata, '{}'::jsonb) || COALESCE(metadata_delta.metadata, '{}'::jsonb),
		       CASE WHEN terminal.fields ? 'error' THEN terminal.fields->>'error' ELSE jr.error END,
		       CASE WHEN terminal.fields ? 'error_class' THEN terminal.fields->>'error_class' ELSE jr.error_class END,
		       jr.triggered_by, COALESCE(s.scheduled_at, jr.scheduled_at), COALESCE(s.started_at, jr.started_at), COALESCE(s.finished_at, jr.finished_at), COALESCE(h.heartbeat_at, s.heartbeat_at, jr.heartbeat_at),
		       COALESCE(s.next_retry_at, jr.next_retry_at), COALESCE(s.expires_at, jr.expires_at), jr.parent_run_id, COALESCE(s.priority, jr.priority), jr.idempotency_key, jr.job_version, jr.created_at, jr.workflow_step_run_id,
		       CASE WHEN terminal.fields ? 'execution_trace' THEN terminal.fields->'execution_trace' ELSE jr.execution_trace END,
		       jr.debug_mode, jr.continuation_of, jr.lineage_depth, jr.tags, jr.job_version_id, jr.created_by, jr.batch_id, COALESCE(NULLIF(s.concurrency_key, ''), jr.concurrency_key), COALESCE(NULLIF(s.execution_mode, ''), jr.execution_mode), jr.is_rollback, jr.replayed_run_id
		FROM lineage l
		JOIN job_runs jr ON jr.id = l.id
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
		ORDER BY jr.lineage_depth ASC
		LIMIT $2`

	rows, err := q.db.Query(ctx, query, rootID, limit)
	if err != nil {
		return nil, fmt.Errorf("list run lineage: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list run lineage scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list run lineage rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListRunsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunsByTag")
	defer span.End()

	base := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, is_rollback, replayed_run_id
		FROM job_runs
		WHERE project_id = $1`

	args := []any{projectID, tagKey}
	param := 3
	if tagValue == "" {
		base += ` AND tags ? $2`
	} else {
		base += ` AND tags ->> $2 = $3`
		args = append(args, tagValue)
		param++
	}

	if cursor != nil {
		base += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	base += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, base, args...)
	if err != nil {
		return nil, fmt.Errorf("list runs by tag: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, scanErr := dbscan.ScanRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list runs by tag scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runs by tag rows: %w", err)
	}

	return runs, nil
}

func marshalTagsForRun(tags map[string]string) []byte {
	if len(tags) == 0 {
		return []byte("{}")
	}
	b, _ := json.Marshal(tags)
	return b
}

// CancelJobRunsByWorkflowRun bulk-cancels all non-terminal job runs linked
// to step runs of the given workflow run.
func (q *Queries) CancelJobRunsByWorkflowRun(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CancelJobRunsByWorkflowRun")
	defer span.End()

	query := bulkCancelTerminalQuery(
		`JOIN workflow_step_runs wsr ON wsr.job_run_id = s.run_id`,
		`wsr.workflow_run_id = $3`,
		"",
		`SELECT COUNT(*) FROM inserted`,
	)

	var count int64
	if err := q.db.QueryRow(ctx, query, finishedAt, reason, workflowRunID).Scan(&count); err != nil {
		return 0, fmt.Errorf("cancel job runs by workflow run: %w", err)
	}
	return count, nil
}

// MarkJobRunsPausedByWorkflowRun transitions executing job runs linked to this
// workflow run to paused status.
func (q *Queries) MarkJobRunsPausedByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkJobRunsPausedByWorkflowRun")
	defer span.End()

	query := `
		WITH candidates AS MATERIALIZED (
			SELECT
				s.run_id,
				s.job_id,
				COALESCE(s.concurrency_key, '') AS concurrency_key,
				s.attempt,
				CASE
					WHEN c.run_id IS NOT NULL AND s.status IN ('queued', 'delayed') THEN 'executing'
					ELSE s.status
				END AS previous_status,
				c.started_at AS claim_started_at,
				s.job_max_concurrency,
				s.job_max_concurrency_per_key,
				c.run_id IS NOT NULL AS uses_active_claim
			FROM job_run_state s
			JOIN workflow_step_runs wsr ON wsr.job_run_id = s.run_id
			LEFT JOIN job_run_active_claims c
			  ON c.run_id = s.run_id
			 AND c.ready_generation = s.ready_generation
			WHERE wsr.workflow_run_id = $1
			  AND NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
			  AND (s.status = 'executing' OR (s.status IN ('queued', 'delayed') AND c.run_id IS NOT NULL))
			FOR UPDATE OF s SKIP LOCKED
		),
		updated AS (
			UPDATE job_run_state s
			SET status = 'paused',
			    started_at = COALESCE(c.claim_started_at, s.started_at),
			    heartbeat_at = NULL,
			    updated_at = NOW()
			FROM candidates c
			WHERE s.run_id = c.run_id
			RETURNING s.run_id, c.job_id, c.concurrency_key, c.attempt, c.previous_status,
			          c.job_max_concurrency, c.job_max_concurrency_per_key,
			          c.uses_active_claim
		),
		released AS (
			UPDATE job_active_counts c
			SET count = GREATEST(c.count - 1, 0),
			    updated_at = NOW()
			FROM updated u
			WHERE u.previous_status IN ('dequeued', 'executing')
			  AND NOT u.uses_active_claim
			  AND (u.job_max_concurrency IS NOT NULL OR u.job_max_concurrency_per_key IS NOT NULL)
			  AND c.job_id = u.job_id
			  AND c.concurrency_key = u.concurrency_key
			  AND c.count <> 0
			RETURNING 1
		),
		lifecycle_events AS (
			INSERT INTO job_run_lifecycle_events (run_id, from_status, to_status, attempt, fields)
			SELECT run_id, previous_status, 'paused', attempt, '{}'::jsonb
			FROM updated
			RETURNING 1
		),
		cache_versions AS (
			INSERT INTO job_run_cache_versions (run_id, cache_version)
			SELECT run_id, strait_next_run_cache_version(run_id)
			FROM updated
			RETURNING 1
		)
		SELECT COUNT(*) FROM updated`

	var count int64
	if err := q.db.QueryRow(ctx, query, workflowRunID).Scan(&count); err != nil {
		return 0, fmt.Errorf("mark job runs paused by workflow run: %w", err)
	}
	return count, nil
}

// RequeuePausedJobRuns transitions paused job runs linked to a workflow run
// back to queued status, clearing pause metadata and resetting timing fields.
func (q *Queries) RequeuePausedJobRuns(ctx context.Context, workflowRunID string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RequeuePausedJobRuns")
	defer span.End()

	query := `
		WITH candidates AS MATERIALIZED (
			SELECT s.run_id, s.attempt
			FROM job_run_state s
			JOIN workflow_step_runs wsr ON wsr.job_run_id = s.run_id
			WHERE wsr.workflow_run_id = $1
			  AND s.status = 'paused'
			  AND NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
			FOR UPDATE OF s SKIP LOCKED
		),
		updated AS (
			UPDATE job_run_state s
			SET status = 'queued',
			    started_at = NULL,
			    finished_at = NULL,
			    heartbeat_at = NULL,
			    ready_generation = ready_generation + 1,
			    updated_at = NOW()
			FROM candidates c
			WHERE s.run_id = c.run_id
			RETURNING s.run_id, c.attempt
		),
		lifecycle_events AS (
			INSERT INTO job_run_lifecycle_events (run_id, from_status, to_status, attempt, fields)
			SELECT run_id, 'paused', 'queued', attempt, '{}'::jsonb
			FROM updated
			RETURNING 1
		),
		cache_versions AS (
			INSERT INTO job_run_cache_versions (run_id, cache_version)
			SELECT run_id, strait_next_run_cache_version(run_id)
			FROM updated
			RETURNING 1
		)
		SELECT COUNT(*) FROM updated`

	var count int64
	if err := q.db.QueryRow(ctx, query, workflowRunID).Scan(&count); err != nil {
		return 0, fmt.Errorf("requeue paused job runs: %w", err)
	}
	return count, nil
}

func (q *Queries) ActivateDueRuns(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}

	query := `
		WITH candidates AS MATERIALIZED (
			SELECT s.run_id, s.ready_generation, s.attempt
			FROM job_run_state s
			WHERE s.status = 'delayed'
			  AND s.scheduled_at <= NOW()
			  AND NOT EXISTS (
			      SELECT 1
			      FROM job_run_terminal_state t
			      WHERE t.run_id = s.run_id
			  )
			  AND NOT EXISTS (
			      SELECT 1
			      FROM job_run_ready_events ready
			      WHERE ready.run_id = s.run_id
			        AND ready.ready_generation = s.ready_generation
			        AND ready.reason = 'delayed_due'
			  )
			ORDER BY s.scheduled_at ASC, s.run_id ASC
			LIMIT $1
			FOR UPDATE OF s SKIP LOCKED
		),
		inserted_ready AS (
			INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
			SELECT run_id, ready_generation, attempt, 'delayed_due'
			FROM candidates
			ON CONFLICT (run_id, ready_generation, reason) DO NOTHING
			RETURNING run_id, attempt
		),
		lifecycle_events AS (
			INSERT INTO job_run_lifecycle_events (run_id, from_status, to_status, attempt, fields)
			SELECT run_id, 'delayed', 'queued', attempt, '{"ready_event": true}'::jsonb
			FROM inserted_ready
			RETURNING 1
		),
		cache_versions AS (
			INSERT INTO job_run_cache_versions (run_id, cache_version)
			SELECT run_id, strait_next_run_cache_version(run_id)
			FROM inserted_ready
			RETURNING 1
		)
		SELECT COUNT(*) FROM inserted_ready`

	var count int64
	if err := q.db.QueryRow(ctx, query, limit).Scan(&count); err != nil {
		return 0, fmt.Errorf("activate due runs: %w", err)
	}
	return count, nil
}

// BulkCancelResult holds the per-run outcome of a bulk cancel.
type BulkCancelResult struct {
	ID             string
	PreviousStatus domain.RunStatus
	Canceled       bool
	Error          string
}

func bulkCancelTerminalQuery(extraJoins, whereClause, orderLimit, selectClause string) string {
	var query strings.Builder
	query.WriteString(`
		WITH candidates AS MATERIALIZED (
			SELECT
				s.run_id,
				s.project_id,
				s.job_id,
				CASE
					WHEN c.run_id IS NOT NULL AND s.status IN ('queued', 'delayed') THEN 'executing'
					ELSE s.status
				END AS previous_status,
				COALESCE(c.attempt, s.attempt) AS attempt,
				s.priority,
				s.scheduled_at,
				COALESCE(c.started_at, s.started_at) AS started_at,
				s.finished_at,
				s.heartbeat_at,
				s.next_retry_at,
				s.expires_at,
				s.concurrency_key,
				s.execution_mode,
				s.queue_name,
				s.environment_id,
				s.job_enabled,
				s.job_paused,
				s.job_max_concurrency,
				s.job_max_concurrency_per_key,
				s.ready_generation,
				c.run_id IS NOT NULL AS uses_active_claim,
				COALESCE(jr.workflow_step_run_id, '') AS workflow_step_run_id
			FROM job_run_state s
			JOIN job_runs jr ON jr.id = s.run_id
			LEFT JOIN job_run_active_claims c
			  ON c.run_id = s.run_id
			 AND c.ready_generation = s.ready_generation`)
	if extraJoins != "" {
		query.WriteString("\n\t\t")
		query.WriteString(extraJoins)
	}
	query.WriteString(`
			WHERE NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
			  AND s.status IN ('delayed', 'queued', 'dequeued', 'executing', 'waiting')
			  AND (`)
	query.WriteString(whereClause)
	query.WriteString(`)`)
	if orderLimit != "" {
		query.WriteString("\n\t\t")
		query.WriteString(orderLimit)
	}
	query.WriteString(`
			FOR UPDATE OF s
		),
		inserted AS (
			INSERT INTO job_run_terminal_state (
				run_id,
				project_id,
				job_id,
				status,
				attempt,
				priority,
				scheduled_at,
				started_at,
				finished_at,
				heartbeat_at,
				next_retry_at,
				expires_at,
				concurrency_key,
				execution_mode,
				queue_name,
				environment_id,
				job_enabled,
				job_paused,
				job_max_concurrency,
				job_max_concurrency_per_key,
				ready_generation,
				updated_at
			)
			SELECT
				run_id,
				project_id,
				job_id,
				'canceled',
				attempt,
				priority,
				scheduled_at,
				started_at,
				$1::TIMESTAMPTZ,
				heartbeat_at,
				next_retry_at,
				expires_at,
				concurrency_key,
				execution_mode,
				queue_name,
				environment_id,
				job_enabled,
				job_paused,
				job_max_concurrency,
				job_max_concurrency_per_key,
				ready_generation,
				NOW()
			FROM candidates
			ON CONFLICT (run_id) DO NOTHING
			RETURNING run_id
		),
		released AS (
			UPDATE job_active_counts c
			SET count = GREATEST(c.count - 1, 0),
			    updated_at = NOW()
			FROM candidates s
			JOIN inserted i ON i.run_id = s.run_id
			WHERE s.previous_status IN ('dequeued', 'executing')
			  AND NOT s.uses_active_claim
			  AND (s.job_max_concurrency IS NOT NULL OR s.job_max_concurrency_per_key IS NOT NULL)
			  AND c.job_id = s.job_id
			  AND c.concurrency_key = COALESCE(s.concurrency_key, '')
			  AND c.count <> 0
			RETURNING 1
		),
		lifecycle_events AS (
			INSERT INTO job_run_lifecycle_events (run_id, from_status, to_status, attempt, fields)
			SELECT
				c.run_id,
				c.previous_status,
				'canceled',
				c.attempt,
				jsonb_strip_nulls(jsonb_build_object(
					'error', NULLIF($2::TEXT, ''),
					'finished_at', $1::TIMESTAMPTZ
				))
			FROM candidates c
			JOIN inserted i ON i.run_id = c.run_id
			RETURNING 1
		),
		cache_versions AS (
			INSERT INTO job_run_cache_versions (run_id, cache_version)
			SELECT run_id, strait_next_run_cache_version(run_id)
			FROM inserted
			RETURNING 1
		)
		`)
	query.WriteString(selectClause)

	return query.String()
}

func (q *Queries) GetRunsByIDs(ctx context.Context, ids []string) (map[string]*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRunsByIDs")
	defer span.End()
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.db.Query(ctx,
		`SELECT jr.id, jr.job_id, jr.project_id, COALESCE(s.status, jr.status), COALESCE(s.attempt, jr.attempt), jr.payload,
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
		 WHERE jr.id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("get runs by ids: %w", err)
	}
	defer rows.Close()
	result := make(map[string]*domain.JobRun, len(ids))
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		result[run.ID] = run
	}
	return result, rows.Err()
}

func (q *Queries) BulkCancelRuns(ctx context.Context, ids []string, finishedAt time.Time, reason string) ([]BulkCancelResult, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BulkCancelRuns")
	defer span.End()
	if len(ids) == 0 {
		return nil, nil
	}
	query := bulkCancelTerminalQuery(
		"",
		`s.run_id = ANY($3::text[])`,
		`ORDER BY array_position($3::text[], s.run_id)`,
		`SELECT c.run_id, c.previous_status
		 FROM candidates c
		 JOIN inserted i ON i.run_id = c.run_id
		 ORDER BY array_position($3::text[], c.run_id)`,
	)
	rows, err := q.db.Query(ctx, query, finishedAt, reason, ids)
	if err != nil {
		return nil, fmt.Errorf("bulk cancel runs: %w", err)
	}
	defer rows.Close()
	canceled := make(map[string]domain.RunStatus, len(ids))
	for rows.Next() {
		var id string
		var previousStatus domain.RunStatus
		if err := rows.Scan(&id, &previousStatus); err != nil {
			return nil, fmt.Errorf("scan canceled id: %w", err)
		}
		canceled[id] = previousStatus
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("bulk cancel rows: %w", err)
	}
	results := make([]BulkCancelResult, 0, len(ids))
	for _, id := range ids {
		if previousStatus, ok := canceled[id]; ok {
			results = append(results, BulkCancelResult{
				ID:             id,
				PreviousStatus: previousStatus,
				Canceled:       true,
			})
		}
	}
	return results, nil
}

func (q *Queries) CancelChildRunsByParentIDs(ctx context.Context, parentIDs []string, finishedAt time.Time, reason string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CancelChildRunsByParentIDs")
	defer span.End()
	if len(parentIDs) == 0 {
		return 0, nil
	}
	query := bulkCancelTerminalQuery(
		"",
		`jr.parent_run_id = ANY($3::text[])`,
		"",
		`SELECT COUNT(*) FROM inserted`,
	)
	var count int64
	if err := q.db.QueryRow(ctx, query, finishedAt, reason, parentIDs).Scan(&count); err != nil {
		return 0, fmt.Errorf("cancel child runs: %w", err)
	}
	return count, nil
}

func (q *Queries) UpdateRunStatusReturningOld(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) (domain.RunStatus, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateRunStatus")
	defer span.End()

	if err := domain.ValidateTransition(from, to); err != nil {
		return "", fmt.Errorf("invalid status transition: %w", err)
	}

	if err := validateRunStatusFields(fields); err != nil {
		return "", err
	}

	updatedState, err := q.tryUpdateRunStateStatus(ctx, id, from, to, fields, nil)
	if err != nil {
		return "", err
	}
	if updatedState {
		return from, nil
	}

	currentStatus, _, err := q.currentRunMutableState(ctx, id)
	if err != nil {
		return "", fmt.Errorf("checking current status: %w", err)
	}
	if currentStatus == to {
		return from, nil // idempotent: already in target state
	}
	return "", fmt.Errorf("%w: id %s from %s", ErrRunConflict, id, from)
}

func (q *Queries) BatchUpdateHeartbeat(ctx context.Context, ids []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BatchUpdateHeartbeat")
	defer span.End()

	if len(ids) == 0 {
		return nil
	}

	query := `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		SELECT DISTINCT id, NOW(), FALSE
		FROM job_runs
		WHERE id = ANY($1)`

	if _, err := q.db.Exec(ctx, query, ids); err != nil {
		return fmt.Errorf("batch update heartbeat: %w", err)
	}
	return nil
}

func (q *Queries) ResetRunIdempotencyKey(ctx context.Context, runID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ResetRunIdempotencyKey")
	defer span.End()

	_, ok := q.db.(TxBeginner)
	if !ok {
		return fmt.Errorf("reset idempotency key requires transaction support")
	}

	return q.withTx(ctx, func(txQ *Queries) error {
		// Fetch run details needed for idempotency cleanup.
		var idempotencyKey *string
		var jobID string
		var createdAt time.Time
		err := txQ.db.QueryRow(ctx, `
			SELECT idempotency_key, job_id, created_at
			FROM job_runs
			WHERE id = $1
			  AND status NOT IN ('dequeued', 'executing', 'waiting')`,
			runID,
		).Scan(&idempotencyKey, &jobID, &createdAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrRunNotFound
			}
			return fmt.Errorf("reset idempotency key fetch: %w", err)
		}

		if idempotencyKey == nil || *idempotencyKey == "" {
			return nil
		}

		// Clear idempotency_key on the run. Use created_at for partition pruning.
		_, err = txQ.db.Exec(ctx, `
			UPDATE job_runs
			SET idempotency_key = NULL
			WHERE id = $1 AND created_at = $2`,
			runID, createdAt,
		)
		if err != nil {
			return fmt.Errorf("reset idempotency key update: %w", err)
		}

		// Remove from global dedup table.
		_, err = txQ.db.Exec(ctx, `
			DELETE FROM job_run_idempotency
			WHERE job_id = $1 AND idempotency_key = $2`,
			jobID, *idempotencyKey,
		)
		if err != nil {
			return fmt.Errorf("reset idempotency key cleanup: %w", err)
		}

		return nil
	})
}

// DeleteExpiredIdempotencyEntries removes rows from job_run_idempotency
// where expires_at has passed. Used by the idempotency GC to bound the
// table size. The limit caps each call so a large purge is spread across
// multiple ticks.
func (q *Queries) DeleteExpiredIdempotencyEntries(ctx context.Context, limit int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteExpiredIdempotencyEntries")
	defer span.End()

	if limit <= 0 {
		limit = 10000
	}
	const sql = `
		WITH victims AS (
			SELECT job_id, idempotency_key
			FROM job_run_idempotency
			WHERE expires_at IS NOT NULL
			  AND expires_at < NOW()
			LIMIT $1
		)
		DELETE FROM job_run_idempotency
		USING victims
		WHERE job_run_idempotency.job_id = victims.job_id
		  AND job_run_idempotency.idempotency_key = victims.idempotency_key`
	tag, err := q.db.Exec(ctx, sql, limit)
	if err != nil {
		return 0, fmt.Errorf("delete expired idempotency entries: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (q *Queries) RescheduleRun(ctx context.Context, runID string, scheduledAt time.Time, payload json.RawMessage) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RescheduleRun")
	defer span.End()

	err := q.db.QueryRow(ctx, `
		WITH params AS (
			SELECT
				$2::timestamptz AS scheduled_at,
				$3::jsonb AS payload,
				CASE WHEN $2 <= NOW() THEN 'queued' ELSE 'delayed' END AS target_status
		),
		candidates AS MATERIALIZED (
			SELECT s.run_id, s.status AS previous_status, p.scheduled_at, p.payload, p.target_status
			FROM job_run_state s
			CROSS JOIN params p
			WHERE s.run_id = $1
			  AND s.status IN ('delayed', 'queued')
			  AND s.started_at IS NULL
			  AND NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
			FOR UPDATE OF s
		),
		updated_state AS (
			UPDATE job_run_state s
			SET scheduled_at = c.scheduled_at,
			    status = c.target_status,
			    ready_generation = CASE
			        WHEN c.target_status = 'queued' AND s.status <> 'queued' THEN s.ready_generation + 1
			        ELSE s.ready_generation
			    END,
			    updated_at = NOW()
			FROM candidates c
			WHERE s.run_id = c.run_id
			  AND (s.scheduled_at IS DISTINCT FROM c.scheduled_at OR s.status IS DISTINCT FROM c.target_status)
			RETURNING s.run_id
		),
		updated_ledger AS (
			UPDATE job_runs jr
			SET payload = c.payload
			FROM candidates c
			WHERE jr.id = c.run_id
			  AND c.payload IS NOT NULL
			  AND jr.payload IS DISTINCT FROM c.payload
			RETURNING jr.id AS run_id
		),
		mutated_runs AS (
			SELECT run_id FROM updated_state
			UNION
			SELECT run_id FROM updated_ledger
		),
		cache_versions AS (
			INSERT INTO job_run_cache_versions (run_id, cache_version)
			SELECT run_id, strait_next_run_cache_version(run_id)
			FROM mutated_runs
			RETURNING 1
		)
		SELECT run_id FROM candidates
	`, runID, scheduledAt, dbscan.NilIfEmptyRawMessage(payload)).Scan(&runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRunNotFound
		}
		return fmt.Errorf("reschedule run: %w", err)
	}
	return nil
}

// BulkCancelFilter defines optional filters for bulk-canceling runs by criteria.
type BulkCancelFilter struct {
	JobID       string
	BatchID     string
	TriggeredBy string
	Status      domain.RunStatus
}

func (q *Queries) BulkCancelByFilter(ctx context.Context, projectID string, f BulkCancelFilter, now time.Time, reason string) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BulkCancelByFilter")
	defer span.End()

	// A LIMIT 10000 cap prevents locking millions of rows in one statement.
	filterConditions := []string{
		"s.project_id = $3",
		"s.status IN ('delayed', 'queued')",
		"s.started_at IS NULL",
	}

	args := []any{now, reason, projectID}
	param := 4

	if f.JobID != "" {
		filterConditions = append(filterConditions, fmt.Sprintf("s.job_id = $%d", param))
		args = append(args, f.JobID)
		param++
	}
	if f.BatchID != "" {
		filterConditions = append(filterConditions, fmt.Sprintf("jr.batch_id = $%d", param))
		args = append(args, f.BatchID)
		param++
	}
	if f.TriggeredBy != "" {
		filterConditions = append(filterConditions, fmt.Sprintf("jr.triggered_by = $%d", param))
		args = append(args, f.TriggeredBy)
		param++
	}
	if f.Status != "" {
		filterConditions = append(filterConditions, fmt.Sprintf("s.status = $%d", param))
		args = append(args, f.Status)
	}

	query := bulkCancelTerminalQuery(
		"",
		strings.Join(filterConditions, " AND "),
		`ORDER BY s.run_id
		LIMIT 10000`,
		`SELECT c.run_id
		 FROM candidates c
		 JOIN inserted i ON i.run_id = c.run_id
		 ORDER BY c.run_id`,
	)
	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("bulk cancel by filter: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("bulk cancel by filter scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (q *Queries) GetRunErrorClass(ctx context.Context, runID string) (string, error) {
	var errorClass string
	err := q.db.QueryRow(ctx, `
		SELECT COALESCE(terminal.fields->>'error_class', jr.error_class, '')
		FROM job_runs jr
		LEFT JOIN LATERAL (
			SELECT fields
			FROM job_run_lifecycle_events e
			WHERE e.run_id = jr.id
			  AND e.fields ? 'error_class'
			ORDER BY e.created_at DESC, e.id DESC
			LIMIT 1
		) terminal ON true
		WHERE jr.id = $1`,
		runID,
	).Scan(&errorClass)
	return errorClass, err
}

func (q *Queries) CountActiveRunsForJob(ctx context.Context, jobID string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.job_id = $1
		  AND COALESCE(s.status, jr.status) IN ('queued','dequeued','executing','waiting','delayed')`
	var count int
	err := q.db.QueryRow(ctx, query, jobID).Scan(&count)
	return count, err
}

// CanceledRun holds metadata about a run that was canceled.
type CanceledRun struct {
	ID                string
	JobID             string
	ProjectID         string
	WorkflowStepRunID string
	ExecutionMode     domain.ExecutionMode
}

// CancelActiveRunsForJob cancels all non-terminal runs for a job and returns
// details of each canceled run. Used by the cron overlap policy cancel_running.
func (q *Queries) CancelActiveRunsForJob(ctx context.Context, jobID string, reason string) ([]CanceledRun, error) {
	return q.cancelActiveRunsForJob(ctx, jobID, "", reason)
}

// CancelActiveRunsForJobExcept cancels all non-terminal runs for a job except
// excludeRunID. Cron cancel_running uses this after inserting the replacement
// run so the replacement cannot be canceled by the broad active-run update.
func (q *Queries) CancelActiveRunsForJobExcept(ctx context.Context, jobID string, excludeRunID string, reason string) ([]CanceledRun, error) {
	if excludeRunID == "" {
		return q.CancelActiveRunsForJob(ctx, jobID, reason)
	}
	return q.cancelActiveRunsForJob(ctx, jobID, excludeRunID, reason)
}

func (q *Queries) cancelActiveRunsForJob(ctx context.Context, jobID string, excludeRunID string, reason string) ([]CanceledRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CancelActiveRunsForJob")
	defer span.End()

	query := bulkCancelTerminalQuery(
		"",
		`s.job_id = $3 AND ($4 = '' OR s.run_id <> $4)`,
		`ORDER BY s.run_id`,
		`SELECT c.run_id, c.job_id, c.project_id, c.workflow_step_run_id, COALESCE(c.execution_mode, 'http')
		 FROM candidates c
		 JOIN inserted i ON i.run_id = c.run_id
		 ORDER BY c.run_id`,
	)
	rows, err := q.db.Query(ctx, query, time.Now().UTC(), reason, jobID, excludeRunID)
	if err != nil {
		return nil, fmt.Errorf("cancel active runs for job: %w", err)
	}
	defer rows.Close()

	var result []CanceledRun
	for rows.Next() {
		var cr CanceledRun
		var execMode string
		if err := rows.Scan(&cr.ID, &cr.JobID, &cr.ProjectID, &cr.WorkflowStepRunID, &execMode); err != nil {
			return nil, fmt.Errorf("scan canceled run: %w", err)
		}
		cr.ExecutionMode = domain.ExecutionMode(execMode)
		result = append(result, cr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cancel active runs rows: %w", err)
	}
	return result, nil
}

// CountRunIterations returns the number of iterations recorded for a run.
func (q *Queries) CountRunIterations(ctx context.Context, runID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountRunIterations")
	defer span.End()

	query := `SELECT COUNT(*) FROM run_iterations WHERE run_id = $1`
	var count int
	if err := q.db.QueryRow(ctx, query, runID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count run iterations: %w", err)
	}
	return count, nil
}

// CreateRunIteration inserts a new run iteration record.
func (q *Queries) CreateRunIteration(ctx context.Context, iter *domain.RunIteration) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateRunIteration")
	defer span.End()

	if iter.ID == "" {
		iter.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO run_iterations (id, run_id, iteration, description)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query, iter.ID, iter.RunID, iter.Iteration, iter.Description).Scan(&iter.CreatedAt)
	if err != nil {
		return fmt.Errorf("create run iteration: %w", err)
	}
	return nil
}

func (q *Queries) CreateRunIterationForActiveRun(ctx context.Context, iter *domain.RunIteration, attempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateRunIterationForActiveRun")
	defer span.End()

	if iter.ID == "" {
		iter.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		WITH active_run AS (
			SELECT jr.id
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE jr.id = $2
			  AND COALESCE(s.attempt, jr.attempt) = $5
			  AND COALESCE(s.status, jr.status) IN ('executing', 'waiting')
			FOR UPDATE OF jr
		)
		INSERT INTO run_iterations (id, run_id, iteration, description)
		SELECT $1, id, $3, $4
		FROM active_run
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query, iter.ID, iter.RunID, iter.Iteration, iter.Description, attempt).Scan(&iter.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, iter.RunID, attempt)
		}
		return fmt.Errorf("create active run iteration: %w", err)
	}
	return nil
}
