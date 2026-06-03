package queue

// Strait's pgque queue engine uses a vendored and modified SQL snapshot of
// PgQue, PgQ Universal Edition: https://github.com/NikolayS/pgque.
// PgQue is Apache-2.0 licensed and includes code derived from PgQ, originally
// developed at Skype Technologies OU by Marko Kreen under the ISC License.
// Permission to use, copy, modify, and distribute the PgQ-derived portions is
// granted with copyright and permission notices retained; those portions are
// provided "AS IS" without warranty.
// Strait uses PgQue as its PostgreSQL ready-event log; Strait owns run state,
// execution ownership, retries, workflows, workers, observability, and APIs.

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"
	"strait/internal/store"
)

const (
	pgQueConsumerName               = "strait"
	pgQueReceiveAll                 = 2147483647
	pgQueMaxAttempts                = 3
	pgQueDefaultMaintenanceInterval = 30 * time.Second
	pgQueDefaultRotationPeriod      = 5 * time.Minute
)

type PgQueConfig struct {
	TickInterval        time.Duration
	MaintenanceInterval time.Duration
	RotationPeriod      time.Duration
	ConsumerName        string
	NackDelay           time.Duration
	ReceiveWindow       int
	Logger              *slog.Logger
}

func (c PgQueConfig) normalized() PgQueConfig {
	if c.TickInterval <= 0 {
		c.TickInterval = 50 * time.Millisecond
	}
	if c.MaintenanceInterval <= 0 {
		c.MaintenanceInterval = pgQueDefaultMaintenanceInterval
	}
	if c.RotationPeriod <= 0 {
		c.RotationPeriod = pgQueDefaultRotationPeriod
	}
	if c.ConsumerName == "" {
		c.ConsumerName = pgQueConsumerName
	}
	if c.NackDelay <= 0 {
		c.NackDelay = time.Second
	}
	if c.ReceiveWindow <= 0 {
		c.ReceiveWindow = 100
	}
	return c
}

type PgQueQueue struct {
	db          store.DBTX
	runWriter   *PostgresRunWriter
	cfg         PgQueConfig
	logger      *slog.Logger
	routeMu     sync.Mutex
	routeStates map[string]*pgQueRouteState
	routeCache  map[string]pgQueRouteCacheEntry

	workerRouteCursor atomic.Uint64
}

type pgQueRouteState struct {
	mu            sync.Mutex
	configMu      sync.Mutex
	configured    atomic.Bool
	lastForceTick time.Time
	activeBatch   *pgQueActiveBatch
}

type pgQueRouteCacheEntry struct {
	routes    []string
	expiresAt time.Time
}

type pgQueReadyEvent struct {
	RunID      string `json:"run_id"`
	RouteKey   string `json:"route_key"`
	Generation int64  `json:"generation"`
	Priority   int    `json:"priority"`
}

type pgQueMessage struct {
	ID         int64
	BatchID    int64
	Type       string
	Payload    string
	RetryCount *int32
	CreatedAt  time.Time
	Extra1     *string
	Extra2     *string
	Extra3     *string
	Extra4     *string
}

func NewPgQueQueue(db store.DBTX, runWriter *PostgresRunWriter, cfg PgQueConfig) *PgQueQueue {
	if runWriter == nil {
		runWriter = NewPostgresRunWriter(db)
	}
	cfg = cfg.normalized()
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &PgQueQueue{
		db:          db,
		runWriter:   runWriter,
		cfg:         cfg,
		logger:      logger,
		routeStates: make(map[string]*pgQueRouteState),
		routeCache:  make(map[string]pgQueRouteCacheEntry),
	}
}

// ReconcileReadyRuns re-emits PgQue ready events for currently claimable runs
// whose ready generation is missing a PgQue emit marker. Active claims remain
// the execution ownership guard, so re-emitted events cannot duplicate a run.
func (q *PgQueQueue) ReconcileReadyRuns(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	rows, err := q.db.Query(ctx, `
		SELECT
			rs.run_id,
			rs.job_id,
			rs.project_id,
			rs.status,
			rs.attempt,
			rs.priority,
			rs.execution_mode,
			rs.queue_name
		FROM job_run_read_state rs
		JOIN job_run_state s ON s.run_id = rs.run_id
		LEFT JOIN job_run_active_claims claim
		  ON claim.run_id = rs.run_id
		 AND claim.ready_generation = rs.ready_generation
		LEFT JOIN job_run_terminal_state terminal ON terminal.run_id = rs.run_id
		LEFT JOIN strait_pgque_ready_events emit
		  ON emit.run_id = rs.run_id
		 AND emit.ready_generation = rs.ready_generation
		WHERE rs.status = 'queued'
		  AND claim.run_id IS NULL
		  AND terminal.run_id IS NULL
		  AND emit.run_id IS NULL
		  AND COALESCE(rs.job_enabled, true) = true
		  AND COALESCE(rs.job_paused, false) = false
		  AND (
		      rs.scheduled_at IS NULL
		      OR rs.scheduled_at <= NOW()
		  )
		  AND (
		      rs.next_retry_at IS NULL
		      OR rs.next_retry_at <= NOW()
		  )
		  AND NOT strait_run_retry_blocked(rs.run_id)
		ORDER BY rs.priority DESC, s.updated_at ASC, rs.run_id ASC
		LIMIT $1`, limit)
	if err != nil {
		return 0, fmt.Errorf("pgque reconcile ready runs: query: %w", err)
	}
	defer rows.Close()

	runs := make([]*domain.JobRun, 0, limit)
	for rows.Next() {
		run := &domain.JobRun{}
		if err := rows.Scan(
			&run.ID,
			&run.JobID,
			&run.ProjectID,
			&run.Status,
			&run.Attempt,
			&run.Priority,
			&run.ExecutionMode,
			&run.QueueName,
		); err != nil {
			return 0, fmt.Errorf("pgque reconcile ready runs: scan: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("pgque reconcile ready runs: rows: %w", err)
	}
	if len(runs) == 0 {
		return 0, nil
	}
	if err := q.sendReadyEvents(ctx, q.db, runs); err != nil {
		return 0, err
	}
	_ = q.tickReadyRoutes(ctx, runs)
	return int64(len(runs)), nil
}

// ActivateDueRuns promotes due delayed runs and ready retries through the PgQue
// storage path. Ready-event inserts and PgQue emits happen in one transaction
// so a crash cannot leave a promoted run without a PgQue event.
func (q *PgQueQueue) ActivateDueRuns(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	beginner, ok := q.db.(store.TxBeginner)
	if !ok {
		return 0, fmt.Errorf("pgque activate due runs requires transaction support")
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("pgque activate due runs: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	delayedLimit := limit
	if limit > 1 {
		delayedLimit = limit / 2
	}
	runs, err := q.promoteDueRunsInTx(ctx, tx, delayedLimit)
	if err != nil {
		return 0, err
	}
	if remaining := limit - len(runs); remaining > 0 {
		retryRuns, retryErr := q.promoteReadyRetriesInTx(ctx, tx, remaining)
		if retryErr != nil {
			return 0, retryErr
		}
		runs = append(runs, retryRuns...)
	}
	if remaining := limit - len(runs); remaining > 0 && delayedLimit < limit {
		moreDelayedRuns, delayedErr := q.promoteDueRunsInTx(ctx, tx, remaining)
		if delayedErr != nil {
			return 0, delayedErr
		}
		runs = append(runs, moreDelayedRuns...)
	}
	if len(runs) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return 0, fmt.Errorf("pgque activate due runs: commit empty promotion: %w", err)
		}
		return 0, nil
	}

	if err := q.sendReadyEvents(ctx, tx, runs); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("pgque activate due runs: commit: %w", err)
	}
	_ = q.tickReadyRoutes(ctx, runs)
	return int64(len(runs)), nil
}

// RequeuePausedJobRuns resumes paused workflow-owned runs through PgQue. The
// state generation bump and ready-event insert share a transaction so resume
// cannot strand a queued run without a matching PgQue event.
func (q *PgQueQueue) RequeuePausedJobRuns(ctx context.Context, workflowRunID string) (int64, error) {
	if workflowRunID == "" {
		return 0, nil
	}
	beginner, ok := q.db.(store.TxBeginner)
	if !ok {
		return 0, fmt.Errorf("pgque requeue paused job runs requires transaction support")
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("pgque requeue paused job runs: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	runs, err := q.requeuePausedJobRunsInTx(ctx, tx, workflowRunID)
	if err != nil {
		return 0, err
	}
	if len(runs) > 0 {
		if err := q.sendReadyEvents(ctx, tx, runs); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("pgque requeue paused job runs: commit: %w", err)
	}
	_ = q.tickReadyRoutes(ctx, runs)
	return int64(len(runs)), nil
}

func (q *PgQueQueue) promoteDueRunsInTx(ctx context.Context, tx store.DBTX, limit int) ([]*domain.JobRun, error) {
	rows, err := tx.Query(ctx, `
		WITH candidates AS MATERIALIZED (
			SELECT
				s.run_id,
				s.job_id,
				s.project_id,
				s.status,
				s.attempt,
				s.scheduled_at,
				s.started_at,
				s.finished_at,
				s.heartbeat_at,
				s.next_retry_at,
				s.expires_at,
				s.priority,
				s.concurrency_key,
				s.execution_mode,
				s.ready_generation
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
			FROM candidates c
			ON CONFLICT (run_id, ready_generation, reason) DO NOTHING
			RETURNING
				run_id,
				ready_generation,
				attempt
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
		SELECT
			jr.id,
			c.job_id,
			c.project_id,
			'queued'::text AS status,
			r.attempt,
			jr.payload,
			jr.result,
			jr.metadata,
			jr.error,
			jr.error_class,
			jr.triggered_by,
			c.scheduled_at,
			c.started_at,
			c.finished_at,
			c.heartbeat_at,
			c.next_retry_at,
			c.expires_at,
			jr.parent_run_id,
			c.priority,
			jr.idempotency_key,
			jr.job_version,
			jr.created_at,
			jr.workflow_step_run_id,
			jr.execution_trace,
			jr.debug_mode,
			jr.continuation_of,
			jr.lineage_depth,
			jr.tags,
			jr.job_version_id,
			jr.created_by,
			jr.batch_id,
			c.concurrency_key,
			c.execution_mode,
			jr.is_rollback,
			jr.replayed_run_id
		FROM inserted_ready r
		JOIN candidates c ON c.run_id = r.run_id
		JOIN job_runs jr ON jr.id = r.run_id
		ORDER BY c.scheduled_at ASC, c.run_id ASC`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("pgque promote due runs: %w", err)
	}
	defer rows.Close()

	runs := make([]*domain.JobRun, 0, min(limit, 1024))
	for rows.Next() {
		run, scanErr := dbscan.ScanRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("pgque promote due runs scan: %w", scanErr)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgque promote due runs rows: %w", err)
	}
	return runs, nil
}

func (q *PgQueQueue) promoteReadyRetriesInTx(ctx context.Context, tx store.DBTX, limit int) ([]*domain.JobRun, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		WITH candidates AS MATERIALIZED (
			SELECT
				rt.id,
				rt.run_id,
				rt.attempt,
				s.ready_generation,
				s.job_id,
				s.project_id,
				s.scheduled_at,
				s.expires_at,
				s.priority,
				s.concurrency_key,
				s.execution_mode
			FROM job_retries rt
			JOIN job_run_state s ON s.run_id = rt.run_id
			WHERE rt.next_retry_at <= NOW()
			  AND rt.cleared = FALSE
			  AND s.status = 'queued'
			  AND NOT EXISTS (
			      SELECT 1
			      FROM job_run_terminal_state t
			      WHERE t.run_id = s.run_id
			  )
			  AND NOT EXISTS (
			      SELECT 1
			      FROM job_retries newer
			      WHERE newer.run_id = rt.run_id
			        AND newer.id > rt.id
			  )
			ORDER BY rt.next_retry_at ASC, rt.run_id ASC
			LIMIT $1
			FOR UPDATE OF rt, s SKIP LOCKED
		),
		cleared_retries AS (
			INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at, cleared)
			SELECT run_id, NULL::timestamptz, 0, NOW(), TRUE
			FROM candidates
			RETURNING
				run_id
		),
		inserted_ready AS (
			INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
			SELECT c.run_id, c.ready_generation, c.attempt, 'retry_ready'
			FROM candidates c
			JOIN cleared_retries cleared ON cleared.run_id = c.run_id
			ON CONFLICT (run_id, ready_generation, reason) DO NOTHING
			RETURNING
				run_id,
				ready_generation,
				attempt
		),
		lifecycle_events AS (
			INSERT INTO job_run_lifecycle_events (run_id, from_status, to_status, attempt, fields)
			SELECT run_id, 'queued', 'queued', attempt, '{"retry_ready": true, "ready_event": true}'::jsonb
			FROM inserted_ready
			RETURNING 1
		),
		cache_versions AS (
			INSERT INTO job_run_cache_versions (run_id, cache_version)
			SELECT run_id, strait_next_run_cache_version(run_id)
			FROM inserted_ready
			RETURNING 1
		)
		SELECT
			jr.id,
			c.job_id,
			c.project_id,
			'queued'::text AS status,
			r.attempt,
			jr.payload,
			jr.result,
			jr.metadata,
			jr.error,
			jr.error_class,
			jr.triggered_by,
			c.scheduled_at,
			NULL::timestamptz AS started_at,
			NULL::timestamptz AS finished_at,
			NULL::timestamptz AS heartbeat_at,
			NULL::timestamptz AS next_retry_at,
			c.expires_at,
			jr.parent_run_id,
			c.priority,
			jr.idempotency_key,
			jr.job_version,
			jr.created_at,
			jr.workflow_step_run_id,
			jr.execution_trace,
			jr.debug_mode,
			jr.continuation_of,
			jr.lineage_depth,
			jr.tags,
			jr.job_version_id,
			jr.created_by,
			jr.batch_id,
			c.concurrency_key,
			c.execution_mode,
			jr.is_rollback,
			jr.replayed_run_id
		FROM inserted_ready r
		JOIN candidates c ON c.run_id = r.run_id
		JOIN job_runs jr ON jr.id = r.run_id
		ORDER BY jr.created_at ASC, c.run_id ASC`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("pgque promote ready retries: %w", err)
	}
	defer rows.Close()

	runs := make([]*domain.JobRun, 0, min(limit, 1024))
	for rows.Next() {
		run, scanErr := dbscan.ScanRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("pgque promote ready retries scan: %w", scanErr)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgque promote ready retries rows: %w", err)
	}
	return runs, nil
}

func (q *PgQueQueue) requeuePausedJobRunsInTx(ctx context.Context, tx store.DBTX, workflowRunID string) ([]*domain.JobRun, error) {
	rows, err := tx.Query(ctx, `
		WITH candidates AS MATERIALIZED (
			SELECT s.run_id, s.attempt
			FROM job_run_state s
			JOIN workflow_step_runs wsr ON wsr.job_run_id = s.run_id
			WHERE wsr.workflow_run_id = $1
			  AND s.status = 'paused'
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
			        AND ready.reason = 'paused_resume'
			  )
			FOR UPDATE OF s SKIP LOCKED
		),
		updated AS (
			UPDATE job_run_state s
			SET started_at = NULL,
			    finished_at = NULL,
			    heartbeat_at = NULL,
			    ready_generation = ready_generation + 1,
			    updated_at = NOW()
			FROM candidates c
			WHERE s.run_id = c.run_id
			RETURNING
				s.run_id,
				s.job_id,
				s.project_id,
				s.status,
				s.attempt,
				s.scheduled_at,
				s.started_at,
				s.finished_at,
				s.heartbeat_at,
				s.next_retry_at,
				s.expires_at,
				s.priority,
				s.concurrency_key,
				s.execution_mode,
				s.ready_generation
		),
		inserted_ready AS (
			INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
			SELECT run_id, ready_generation, attempt, 'paused_resume'
			FROM updated
			ON CONFLICT (run_id, ready_generation, reason) DO NOTHING
			RETURNING run_id, ready_generation, attempt
		),
		lifecycle_events AS (
			INSERT INTO job_run_lifecycle_events (run_id, from_status, to_status, attempt, fields)
			SELECT run_id, 'paused', 'queued', attempt, '{}'::jsonb
			FROM inserted_ready
			RETURNING 1
		),
		cache_versions AS (
			INSERT INTO job_run_cache_versions (run_id, cache_version)
			SELECT run_id, strait_next_run_cache_version(run_id)
			FROM inserted_ready
			RETURNING 1
		)
		SELECT
			jr.id,
			u.job_id,
			u.project_id,
			'queued'::text AS status,
			u.attempt,
			jr.payload,
			jr.result,
			jr.metadata,
			jr.error,
			jr.error_class,
			jr.triggered_by,
			u.scheduled_at,
			u.started_at,
			u.finished_at,
			u.heartbeat_at,
			u.next_retry_at,
			u.expires_at,
			jr.parent_run_id,
			u.priority,
			jr.idempotency_key,
			jr.job_version,
			jr.created_at,
			jr.workflow_step_run_id,
			jr.execution_trace,
			jr.debug_mode,
			jr.continuation_of,
			jr.lineage_depth,
			jr.tags,
			jr.job_version_id,
			jr.created_by,
			jr.batch_id,
			u.concurrency_key,
			u.execution_mode,
			jr.is_rollback,
			jr.replayed_run_id
		FROM updated u
		JOIN job_runs jr ON jr.id = u.run_id
		ORDER BY jr.created_at ASC, u.run_id ASC`,
		workflowRunID,
	)
	if err != nil {
		return nil, fmt.Errorf("pgque requeue paused job runs: %w", err)
	}
	defer rows.Close()

	runs := make([]*domain.JobRun, 0, 16)
	for rows.Next() {
		run, scanErr := dbscan.ScanRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("pgque requeue paused job runs scan: %w", scanErr)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgque requeue paused job runs rows: %w", err)
	}
	return runs, nil
}

func (q *PgQueQueue) markPgQueStorage(ctx context.Context, db store.DBTX) error {
	if _, err := db.Exec(ctx, `SET LOCAL strait.queue_backend = 'pgque'`); err != nil {
		return fmt.Errorf("pgque mark queue storage: %w", err)
	}
	return nil
}

var _ Queue = (*PgQueQueue)(nil)
var _ interface {
	EnqueueExisting(context.Context, *domain.JobRun) error
} = (*PgQueQueue)(nil)
var _ interface {
	RunTicker(context.Context)
	Maintain(context.Context) error
} = (*PgQueQueue)(nil)
