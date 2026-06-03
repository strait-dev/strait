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
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"
	"strait/internal/store"

	"go.opentelemetry.io/otel"
)

const (
	pgQueConsumerName               = "strait"
	pgQueReceiveAll                 = 2147483647
	pgQueMaxAttempts                = 3
	pgQueDefaultMaintenanceInterval = 30 * time.Second
	pgQueDefaultRotationPeriod      = 5 * time.Minute
)

var errPgQueNoMessages = errors.New("pgque: no messages available")

type PgQueConfig struct {
	TickInterval        time.Duration
	MaintenanceInterval time.Duration
	RotationPeriod      time.Duration
	ConsumerName        string
	NackDelay           time.Duration
	ReceiveWindow       int
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
	legacy      *PostgresQueue
	cfg         PgQueConfig
	routeMu     sync.Mutex
	routeStates map[string]*pgQueRouteState
}

type pgQueRouteState struct {
	mu            sync.Mutex
	configMu      sync.Mutex
	configured    atomic.Bool
	lastForceTick time.Time
	activeBatch   *pgQueActiveBatch
}

type pgQueActiveBatch struct {
	BatchID  int64
	Messages []pgQueMessage
	InFlight int
	Closing  bool
}

type pgQueReadyEvent struct {
	RunID      string `json:"run_id"`
	RouteKey   string `json:"route_key"`
	Generation int64  `json:"generation"`
	Priority   int    `json:"priority"`
}

type pgQueCandidate struct {
	Message pgQueMessage
	Event   pgQueReadyEvent
	Order   int
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

type pgQueClaimFilter struct {
	ProjectID     string
	ExecutionMode domain.ExecutionMode
	WorkerRefs    []domain.WorkerQueueRef
}

const pgQueClaimDequeueColumns = `u.run_id, u.job_id, u.project_id, u.status, u.attempt, u.payload, u.result, u.metadata, u.error, u.error_class,
		          u.triggered_by, u.scheduled_at, u.started_at, u.finished_at, u.heartbeat_at,
		          u.next_retry_at, u.expires_at, u.parent_run_id, u.priority, u.idempotency_key, u.job_version, u.created_at, u.workflow_step_run_id, u.execution_trace, u.debug_mode, u.continuation_of, u.lineage_depth, u.tags, u.job_version_id, u.created_by, u.batch_id, u.concurrency_key, u.execution_mode, u.is_rollback, u.replayed_run_id`

func NewPgQueQueue(db store.DBTX, legacy *PostgresQueue, cfg PgQueConfig) *PgQueQueue {
	if legacy == nil {
		legacy = NewPostgresQueue(db)
	}
	return &PgQueQueue{
		db:          db,
		legacy:      legacy,
		cfg:         cfg.normalized(),
		routeStates: make(map[string]*pgQueRouteState),
	}
}

func (q *PgQueQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	beginner, ok := q.db.(store.TxBeginner)
	if !ok {
		if err := q.legacy.Enqueue(ctx, run); err != nil {
			return err
		}
		if run.Status == domain.StatusQueued {
			if err := q.sendReadyEvent(ctx, q.db, run); err != nil {
				return err
			}
			_ = q.tickReadyRoute(ctx, run)
		}
		return nil
	}

	if run.Status == domain.StatusQueued {
		if err := q.ensureRunRouteCached(ctx, run); err != nil {
			return err
		}
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgque enqueue: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := q.EnqueueInTx(ctx, tx, run); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pgque enqueue: commit: %w", err)
	}
	if run.Status == domain.StatusQueued {
		_ = q.tickReadyRoute(ctx, run)
	}
	return nil
}

func (q *PgQueQueue) EnqueueInTx(ctx context.Context, tx store.DBTX, run *domain.JobRun) error {
	if err := q.markPgQueStorage(ctx, tx); err != nil {
		return err
	}
	if err := q.legacy.EnqueueInTx(ctx, tx, run); err != nil {
		return err
	}
	if run.Status != domain.StatusQueued {
		return nil
	}
	return q.sendReadyEvent(ctx, tx, run)
}

func (q *PgQueQueue) EnqueueBatch(ctx context.Context, runs []*domain.JobRun) (int64, error) {
	if len(runs) == 0 {
		return 0, nil
	}
	if err := q.ensureRunRoutesCached(ctx, runs); err != nil {
		return 0, err
	}
	beginner, ok := q.db.(store.TxBeginner)
	if !ok {
		inserted, err := q.legacy.EnqueueBatch(ctx, runs)
		if err != nil {
			return 0, err
		}
		if err := q.sendReadyEvents(ctx, q.db, runs); err != nil {
			return 0, err
		}
		_ = q.tickReadyRoutes(ctx, runs)
		return inserted, nil
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("pgque enqueue batch: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	legacy := NewPostgresQueue(tx)
	if err := q.markPgQueStorage(ctx, tx); err != nil {
		return 0, err
	}
	inserted, err := legacy.EnqueueBatch(ctx, runs)
	if err != nil {
		return 0, err
	}
	if err := q.sendReadyEvents(ctx, tx, runs); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("pgque enqueue batch: commit: %w", err)
	}
	_ = q.tickReadyRoutes(ctx, runs)
	return inserted, nil
}

func (q *PgQueQueue) EnqueueExisting(ctx context.Context, run *domain.JobRun) error {
	if run == nil || run.Status != domain.StatusQueued {
		return nil
	}
	if err := q.ensureRunRouteCached(ctx, run); err != nil {
		return err
	}
	if err := q.sendReadyEvent(ctx, q.db, run); err != nil {
		return err
	}
	return q.tickReadyRoute(ctx, run)
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

func (q *PgQueQueue) Dequeue(ctx context.Context) (*domain.JobRun, error) {
	runs, err := q.DequeueN(ctx, 1)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return &runs[0], nil
}

func (q *PgQueQueue) DequeueN(ctx context.Context, n int) ([]domain.JobRun, error) {
	return q.dequeueFromRoute(ctx, n, pgQueHTTPRouteKey, pgQueClaimFilter{
		ExecutionMode: domain.ExecutionModeHTTP,
	})
}

func (q *PgQueQueue) DequeueNFair(ctx context.Context, n int) ([]domain.JobRun, error) {
	return q.DequeueN(ctx, n)
}

func (q *PgQueQueue) DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	return q.dequeueFromRoute(ctx, n, pgQueHTTPRouteKey, pgQueClaimFilter{
		ProjectID:     projectID,
		ExecutionMode: domain.ExecutionModeHTTP,
	})
}

func (q *PgQueQueue) DequeueNForWorkerQueues(ctx context.Context, n int, queues []domain.WorkerQueueRef) ([]domain.JobRun, error) {
	refs := normalizePgQueWorkerQueueRefs(queues)
	if n <= 0 || len(refs) == 0 {
		return nil, nil
	}
	routes, err := q.workerRouteKeys(ctx, refs)
	if err != nil {
		return nil, err
	}
	claimed := make([]domain.JobRun, 0, n)
	for _, routeKey := range routes {
		if len(claimed) >= n {
			break
		}
		batch, err := q.dequeueFromRoute(ctx, n-len(claimed), routeKey, pgQueClaimFilter{
			ExecutionMode: domain.ExecutionModeWorker,
			WorkerRefs:    refs,
		})
		if err != nil {
			return claimed, err
		}
		claimed = append(claimed, batch...)
	}
	return claimed, nil
}

func (q *PgQueQueue) ForceTick(ctx context.Context, routeKey string) error {
	queueName := pgQueQueueName(routeKey)
	return q.forceTickQueue(ctx, queueName)
}

func (q *PgQueQueue) forceTickQueue(ctx context.Context, queueName string) error {
	client := q.pgque(q.db)
	if err := client.forceNextTick(ctx, queueName); err != nil {
		return err
	}
	return client.ticker(ctx, queueName)
}

func (q *PgQueQueue) routeState(routeKey string) *pgQueRouteState {
	q.routeMu.Lock()
	defer q.routeMu.Unlock()
	state := q.routeStates[routeKey]
	if state == nil {
		state = &pgQueRouteState{}
		q.routeStates[routeKey] = state
	}
	return state
}

func (q *PgQueQueue) routeConfigured(routeKey string) bool {
	state := q.routeState(routeKey)
	return state.configured.Load()
}

func (q *PgQueQueue) ensureRunRouteCached(ctx context.Context, run *domain.JobRun) error {
	routeKey, err := q.routeKeyForRun(ctx, q.db, run)
	if err != nil {
		return err
	}
	queueName := pgQueQueueName(routeKey)
	state := q.routeState(routeKey)
	return q.ensureRouteCached(ctx, state, routeKey, queueName)
}

func (q *PgQueQueue) ensureRunRoutesCached(ctx context.Context, runs []*domain.JobRun) error {
	seen := make(map[string]struct{}, len(runs))
	for _, run := range runs {
		if run == nil || run.Status != domain.StatusQueued {
			continue
		}
		routeKey, err := q.routeKeyForRun(ctx, q.db, run)
		if err != nil {
			return err
		}
		if _, ok := seen[routeKey]; ok {
			continue
		}
		seen[routeKey] = struct{}{}
		queueName := pgQueQueueName(routeKey)
		state := q.routeState(routeKey)
		if err := q.ensureRouteCached(ctx, state, routeKey, queueName); err != nil {
			return err
		}
	}
	return nil
}

func (q *PgQueQueue) ensureRouteCached(ctx context.Context, state *pgQueRouteState, routeKey, queueName string) error {
	if state.configured.Load() {
		return nil
	}
	state.configMu.Lock()
	defer state.configMu.Unlock()
	if state.configured.Load() {
		return nil
	}
	if err := q.ensureRoute(ctx, q.db, routeKey, queueName); err != nil {
		return err
	}
	state.configured.Store(true)
	return nil
}

func (q *PgQueQueue) maybeForceTick(ctx context.Context, state *pgQueRouteState, queueName string) {
	if !state.lastForceTick.IsZero() && time.Since(state.lastForceTick) < q.cfg.TickInterval {
		return
	}
	if err := q.forceTickQueue(ctx, queueName); err == nil {
		state.lastForceTick = time.Now()
	}
}

func (q *PgQueQueue) RunTicker(ctx context.Context) {
	ticker := time.NewTicker(q.cfg.TickInterval)
	defer ticker.Stop()
	maintenance := time.NewTicker(q.cfg.MaintenanceInterval)
	defer maintenance.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = q.pgque(q.db).tickerAll(ctx)
		case <-maintenance.C:
			_ = q.Maintain(ctx)
		}
	}
}

func (q *PgQueQueue) Maintain(ctx context.Context) error {
	rotationQueues, err := q.rotationQueuesDueForMaintenance(ctx)
	if err != nil {
		return err
	}
	client := q.pgque(q.db)
	for _, queueName := range rotationQueues {
		if err := client.rotateTablesStep1(ctx, queueName); err != nil {
			return err
		}
	}
	return client.rotateTablesStep2(ctx)
}

func (q *PgQueQueue) rotationQueuesDueForMaintenance(ctx context.Context) ([]string, error) {
	rows, err := q.db.Query(ctx, `
		SELECT func_arg
		FROM pgque.maint_operations()
		WHERE func_name = 'pgque.maint_rotate_tables_step1'
		  AND func_arg IS NOT NULL
		ORDER BY func_arg`)
	if err != nil {
		return nil, fmt.Errorf("pgque maintain operations: %w", err)
	}
	defer rows.Close()

	queueNames := []string{}
	for rows.Next() {
		var queueName string
		if err := rows.Scan(&queueName); err != nil {
			return nil, fmt.Errorf("pgque maintain operation scan: %w", err)
		}
		queueNames = append(queueNames, queueName)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgque maintain operations rows: %w", err)
	}
	return queueNames, nil
}

func (q *PgQueQueue) sendReadyEvent(ctx context.Context, db store.DBTX, run *domain.JobRun) error {
	routeKey, err := q.routeKeyForRun(ctx, db, run)
	if err != nil {
		return err
	}
	queueName := pgQueQueueName(routeKey)
	if !q.routeConfigured(routeKey) {
		if err := q.ensureRoute(ctx, db, routeKey, queueName); err != nil {
			return err
		}
	}
	generation, err := q.readyGeneration(ctx, db, run.ID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(pgQueReadyEvent{
		RunID:      run.ID,
		RouteKey:   routeKey,
		Generation: generation,
		Priority:   run.Priority,
	})
	if err != nil {
		return fmt.Errorf("pgque ready event: marshal: %w", err)
	}
	if err := q.pgque(db).sendText(ctx, queueName, pgQueReadyEventType, string(payload)); err != nil {
		return fmt.Errorf("pgque send ready event: %w", err)
	}
	if err := q.recordReadyEmits(ctx, db, map[string]int64{run.ID: generation}); err != nil {
		return err
	}
	return nil
}

func (q *PgQueQueue) sendReadyEvents(ctx context.Context, db store.DBTX, runs []*domain.JobRun) error {
	readyRuns := make([]pgQueReadyRun, 0, len(runs))
	runIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		if run == nil || run.Status != domain.StatusQueued {
			continue
		}
		routeKey, err := q.routeKeyForRun(ctx, db, run)
		if err != nil {
			return err
		}
		readyRuns = append(readyRuns, pgQueReadyRun{
			run:      run,
			routeKey: routeKey,
		})
		runIDs = append(runIDs, run.ID)
	}
	generations, err := q.readyGenerations(ctx, db, runIDs)
	if err != nil {
		return err
	}

	byRoute := make(map[string][]string, len(runs))
	for _, readyRun := range readyRuns {
		generation, ok := generations[readyRun.run.ID]
		if !ok {
			return fmt.Errorf("pgque ready generation: missing run %s", readyRun.run.ID)
		}
		payload, err := json.Marshal(pgQueReadyEvent{
			RunID:      readyRun.run.ID,
			RouteKey:   readyRun.routeKey,
			Generation: generation,
			Priority:   readyRun.run.Priority,
		})
		if err != nil {
			return fmt.Errorf("pgque ready event: marshal: %w", err)
		}
		byRoute[readyRun.routeKey] = append(byRoute[readyRun.routeKey], string(payload))
	}
	for routeKey, payloads := range byRoute {
		queueName := pgQueQueueName(routeKey)
		if !q.routeConfigured(routeKey) {
			if err := q.ensureRoute(ctx, db, routeKey, queueName); err != nil {
				return err
			}
		}
		if err := q.pgque(db).sendTextBatch(ctx, queueName, pgQueReadyEventType, payloads); err != nil {
			return fmt.Errorf("pgque send ready event batch: %w", err)
		}
	}
	if err := q.recordReadyEmits(ctx, db, generations); err != nil {
		return err
	}
	return nil
}

type pgQueReadyRun struct {
	run      *domain.JobRun
	routeKey string
}

func (q *PgQueQueue) recordReadyEmits(ctx context.Context, db store.DBTX, generations map[string]int64) error {
	if len(generations) == 0 {
		return nil
	}
	runIDs := make([]string, 0, len(generations))
	for runID := range generations {
		runIDs = append(runIDs, runID)
	}
	sort.Strings(runIDs)
	readyGenerations := make([]int64, 0, len(generations))
	for _, runID := range runIDs {
		readyGenerations = append(readyGenerations, generations[runID])
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO strait_pgque_ready_events (run_id, ready_generation)
		SELECT run_id, ready_generation
		FROM unnest($1::text[], $2::bigint[]) AS input(run_id, ready_generation)
		ON CONFLICT (run_id, ready_generation) DO NOTHING`, runIDs, readyGenerations); err != nil {
		return fmt.Errorf("pgque record ready emits: %w", err)
	}
	return nil
}

func (q *PgQueQueue) ensureRoute(ctx context.Context, db store.DBTX, routeKey, queueName string) error {
	if _, err := db.Exec(ctx, `
		INSERT INTO strait_pgque_routes (route_key, queue_name)
		VALUES ($1, $2)
		ON CONFLICT (route_key) DO NOTHING`, routeKey, queueName); err != nil {
		return fmt.Errorf("pgque route upsert: %w", err)
	}
	client := q.pgque(db)
	if err := client.createQueue(ctx, queueName); err != nil {
		return err
	}
	if err := client.setQueueConfig(ctx, queueName, "ticker_max_count", strconv.Itoa(q.cfg.ReceiveWindow)); err != nil {
		return err
	}
	rotationPeriod := pgQueIntervalSetting(q.cfg.RotationPeriod)
	if err := client.setQueueConfig(ctx, queueName, "rotation_period", rotationPeriod); err != nil {
		return err
	}
	return client.registerConsumer(ctx, queueName)
}

func (q *PgQueQueue) tickReadyRoute(ctx context.Context, run *domain.JobRun) error {
	routeKey, err := q.routeKeyForRun(ctx, q.db, run)
	if err != nil {
		return err
	}
	queueName := pgQueQueueName(routeKey)
	if err := q.pgque(q.db).ticker(ctx, queueName); err != nil {
		return fmt.Errorf("pgque tick ready route %s: %w", routeKey, err)
	}
	return nil
}

func (q *PgQueQueue) tickReadyRoutes(ctx context.Context, runs []*domain.JobRun) error {
	seen := make(map[string]struct{}, len(runs))
	for _, run := range runs {
		if run == nil || run.Status != domain.StatusQueued {
			continue
		}
		routeKey, err := q.routeKeyForRun(ctx, q.db, run)
		if err != nil {
			return err
		}
		if _, ok := seen[routeKey]; ok {
			continue
		}
		seen[routeKey] = struct{}{}
		queueName := pgQueQueueName(routeKey)
		if err := q.pgque(q.db).ticker(ctx, queueName); err != nil {
			return fmt.Errorf("pgque tick ready route %s: %w", routeKey, err)
		}
	}
	return nil
}

func normalizePgQueWorkerQueueRefs(refs []domain.WorkerQueueRef) []domain.WorkerQueueRef {
	if len(refs) == 0 {
		return nil
	}
	normalized := make([]domain.WorkerQueueRef, 0, len(refs))
	seen := make(map[domain.WorkerQueueRef]struct{}, len(refs))
	for _, ref := range refs {
		if ref.ProjectID == "" || ref.QueueName == "" {
			continue
		}
		ref.QueueName = runQueueName(ref.QueueName)
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		normalized = append(normalized, ref)
	}
	return normalized
}

func pgQueIntervalSetting(d time.Duration) string {
	if d <= 0 {
		d = pgQueDefaultRotationPeriod
	}
	return strconv.FormatInt(d.Microseconds(), 10) + " microseconds"
}

func (q *PgQueQueue) routeKeyForRun(ctx context.Context, db store.DBTX, run *domain.JobRun) (string, error) {
	if run == nil || run.ExecutionMode != domain.ExecutionModeWorker {
		return pgQueHTTPRouteKey, nil
	}
	var queueName, environmentID string
	if err := db.QueryRow(ctx, `
		SELECT COALESCE(NULLIF($2, ''), NULLIF(j.queue_name, ''), 'default'),
		       COALESCE(j.environment_id, '')
		FROM jobs j
		WHERE j.id = $1`, run.JobID, run.QueueName).Scan(&queueName, &environmentID); err != nil {
		return "", fmt.Errorf("pgque worker route lookup: %w", err)
	}
	return pgQueWorkerRouteKey(run.ProjectID, queueName, environmentID), nil
}

func (q *PgQueQueue) readyGeneration(ctx context.Context, db store.DBTX, runID string) (int64, error) {
	var generation int64
	if err := db.QueryRow(ctx, `SELECT ready_generation FROM job_run_state WHERE run_id = $1`, runID).Scan(&generation); err != nil {
		return 0, fmt.Errorf("pgque ready generation: %w", err)
	}
	return generation, nil
}

func (q *PgQueQueue) readyGenerations(ctx context.Context, db store.DBTX, runIDs []string) (map[string]int64, error) {
	generations := make(map[string]int64, len(runIDs))
	if len(runIDs) == 0 {
		return generations, nil
	}
	rows, err := db.Query(ctx, `
		SELECT run_id, ready_generation
		FROM job_run_state
		WHERE run_id = ANY($1::text[])`, runIDs)
	if err != nil {
		return nil, fmt.Errorf("pgque ready generations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var runID string
		var generation int64
		if err := rows.Scan(&runID, &generation); err != nil {
			return nil, fmt.Errorf("pgque ready generations scan: %w", err)
		}
		generations[runID] = generation
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgque ready generations rows: %w", err)
	}
	return generations, nil
}

func (q *PgQueQueue) workerRouteKeys(ctx context.Context, refs []domain.WorkerQueueRef) ([]string, error) {
	routes := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		queueName := runQueueName(ref.QueueName)
		if ref.EnvironmentID != "" {
			key := pgQueWorkerRouteKey(ref.ProjectID, queueName, ref.EnvironmentID)
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				routes = append(routes, key)
			}
			continue
		}
		prefix := pgQueWorkerRouteKey(ref.ProjectID, queueName, "")
		var err error
		routes, err = q.appendKnownWorkerRoutes(ctx, prefix, seen, routes)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[prefix]; !ok {
			seen[prefix] = struct{}{}
			routes = append(routes, prefix)
		}
	}
	return routes, nil
}

func (q *PgQueQueue) appendKnownWorkerRoutes(
	ctx context.Context,
	prefix string,
	seen map[string]struct{},
	routes []string,
) ([]string, error) {
	rows, err := q.db.Query(ctx, `
		SELECT route_key
		FROM strait_pgque_routes
		WHERE route_key = $1 OR route_key LIKE $2
		ORDER BY route_key`, prefix, prefix+"%")
	if err != nil {
		return nil, fmt.Errorf("pgque worker route lookup: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("pgque worker route scan: %w", err)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		routes = append(routes, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgque worker route rows: %w", err)
	}
	return routes, nil
}

func (q *PgQueQueue) dequeueFromRoute(ctx context.Context, n int, routeKey string, filter pgQueClaimFilter) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.PgQueDequeue")
	defer span.End()

	if n <= 0 {
		return nil, nil
	}
	queueName := pgQueQueueName(routeKey)
	state := q.routeState(routeKey)

	if err := q.ensureRouteCached(ctx, state, routeKey, queueName); err != nil {
		return nil, err
	}

	for attempt := 0; attempt < pgQueMaxAttempts; attempt++ {
		reservation, err := q.reserveFromActiveBatch(ctx, state, queueName, n)
		if err != nil {
			return nil, err
		}
		if reservation.Batch == nil {
			return nil, nil
		}

		for _, msg := range reservation.Invalid {
			_ = q.pgque(q.db).nack(ctx, msg, q.cfg.NackDelay, "invalid ready event")
		}
		if len(reservation.Candidates) == 0 {
			if err := q.finishBatchReservation(ctx, state, reservation.Batch, nil); err != nil {
				return nil, err
			}
			continue
		}

		runs, unclaimed, nackUnclaimed, err := q.claimReservedCandidates(ctx, reservation.Candidates, n, filter)
		returnCandidates := unclaimed
		if nackUnclaimed {
			for _, candidate := range unclaimed {
				_ = q.pgque(q.db).nack(ctx, candidate.Message, q.cfg.NackDelay, "not claimable")
			}
			returnCandidates = nil
		}
		if err != nil {
			returnCandidates = reservation.Candidates
		}
		if finishErr := q.finishBatchReservation(ctx, state, reservation.Batch, returnCandidates); finishErr != nil {
			return runs, finishErr
		}
		if err != nil {
			return nil, err
		}
		if len(runs) > 0 {
			for i := range runs {
				q.legacy.recordClaimMetrics(ctx, &runs[i])
			}
			return runs, nil
		}
	}
	return nil, nil
}

type pgQueBatchReservation struct {
	Batch      *pgQueActiveBatch
	Candidates []pgQueCandidate
	Invalid    []pgQueMessage
}

func (q *PgQueQueue) reserveFromActiveBatch(ctx context.Context, state *pgQueRouteState, queueName string, limit int) (pgQueBatchReservation, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.activeBatch != nil && state.activeBatch.Closing {
		return pgQueBatchReservation{}, nil
	}
	if state.activeBatch != nil && len(state.activeBatch.Messages) == 0 && state.activeBatch.InFlight == 0 {
		return pgQueBatchReservation{Batch: state.activeBatch}, nil
	}
	if state.activeBatch == nil {
		q.maybeForceTick(ctx, state, queueName)
		_, err := q.activeBatchLocked(ctx, state, queueName)
		if errors.Is(err, errPgQueNoMessages) {
			return pgQueBatchReservation{}, nil
		}
		if err != nil {
			return pgQueBatchReservation{}, err
		}
	}
	batch := state.activeBatch
	if len(batch.Messages) == 0 {
		return pgQueBatchReservation{}, nil
	}

	candidates := make([]pgQueCandidate, 0, len(batch.Messages))
	invalid := make([]pgQueMessage, 0)
	removeIDs := make(map[int64]struct{})
	for i, msg := range batch.Messages {
		var event pgQueReadyEvent
		if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil || event.RunID == "" {
			invalid = append(invalid, msg)
			removeIDs[msg.ID] = struct{}{}
			continue
		}
		candidates = append(candidates, pgQueCandidate{Message: msg, Event: event, Order: i})
	}
	if len(candidates) > 0 {
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].Event.Priority != candidates[j].Event.Priority {
				return candidates[i].Event.Priority > candidates[j].Event.Priority
			}
			return candidates[i].Order < candidates[j].Order
		})
		candidates = candidates[:min(len(candidates), max(limit, q.cfg.ReceiveWindow))]
		for _, candidate := range candidates {
			removeIDs[candidate.Message.ID] = struct{}{}
		}
		batch.InFlight++
	}
	if len(removeIDs) > 0 {
		remaining := make([]pgQueMessage, 0, len(batch.Messages)-len(removeIDs))
		for _, msg := range batch.Messages {
			if _, ok := removeIDs[msg.ID]; ok {
				continue
			}
			remaining = append(remaining, msg)
		}
		batch.Messages = remaining
	}
	return pgQueBatchReservation{Batch: batch, Candidates: candidates, Invalid: invalid}, nil
}

func (q *PgQueQueue) finishBatchReservation(ctx context.Context, state *pgQueRouteState, batch *pgQueActiveBatch, returnCandidates []pgQueCandidate) error {
	if batch == nil {
		return nil
	}
	if !q.closeBatchIfDrained(state, batch, returnCandidates) {
		return nil
	}
	if err := q.pgque(q.db).ack(ctx, batch.BatchID); err != nil {
		q.reopenBatchAfterAckFailure(state, batch)
		return err
	}
	q.clearAckedBatch(state, batch)
	return nil
}

func (q *PgQueQueue) closeBatchIfDrained(state *pgQueRouteState, batch *pgQueActiveBatch, returnCandidates []pgQueCandidate) bool {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.activeBatch == batch && !batch.Closing {
		for _, candidate := range returnCandidates {
			batch.Messages = append(batch.Messages, candidate.Message)
		}
		if batch.InFlight > 0 {
			batch.InFlight--
		}
		if len(batch.Messages) == 0 && batch.InFlight == 0 {
			batch.Closing = true
		}
	}
	return state.activeBatch == batch && batch.Closing
}

func (q *PgQueQueue) reopenBatchAfterAckFailure(state *pgQueRouteState, batch *pgQueActiveBatch) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.activeBatch == batch {
		batch.Closing = false
	}
}

func (q *PgQueQueue) clearAckedBatch(state *pgQueRouteState, batch *pgQueActiveBatch) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.activeBatch == batch {
		state.activeBatch = nil
	}
}

// activeBatchLocked requires state.mu to be held by the caller. PgQue batches
// are acked as a unit, so local reservations must mutate state.activeBatch
// synchronously with receive/ack bookkeeping.
func (q *PgQueQueue) activeBatchLocked(ctx context.Context, state *pgQueRouteState, queueName string) (*pgQueActiveBatch, error) {
	if batch := state.activeBatch; batch != nil && (len(batch.Messages) > 0 || batch.InFlight > 0 || batch.Closing) {
		return batch, nil
	}
	messages, err := q.pgque(q.db).receive(ctx, queueName, pgQueReceiveAll)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, errPgQueNoMessages
	}
	batch := &pgQueActiveBatch{BatchID: messages[0].BatchID, Messages: messages}
	state.activeBatch = batch
	return batch, nil
}

func (q *PgQueQueue) claimReservedCandidates(ctx context.Context, candidates []pgQueCandidate, limit int, filter pgQueClaimFilter) ([]domain.JobRun, []pgQueCandidate, bool, error) {
	if len(candidates) == 0 {
		return nil, nil, false, nil
	}
	selected := candidates[:min(len(candidates), max(limit, q.cfg.ReceiveWindow))]
	ids := make([]string, 0, len(selected))
	generations := make([]int64, 0, len(selected))
	for _, candidate := range selected {
		ids = append(ids, candidate.Event.RunID)
		generations = append(generations, candidate.Event.Generation)
	}

	runs, err := q.claimRuns(ctx, ids, generations, limit, filter)
	if err != nil {
		return nil, nil, false, err
	}
	if len(runs) == 0 {
		return nil, selected, true, nil
	}

	claimed := make(map[string]struct{}, len(runs))
	for _, run := range runs {
		claimed[run.ID] = struct{}{}
	}

	unclaimed := make([]pgQueCandidate, 0, len(candidates)-len(runs))
	for _, candidate := range candidates {
		if _, ok := claimed[candidate.Event.RunID]; !ok {
			unclaimed = append(unclaimed, candidate)
		}
	}
	return runs, unclaimed, false, nil
}

func (q *PgQueQueue) claimRuns(ctx context.Context, ids []string, generations []int64, limit int, filter pgQueClaimFilter) ([]domain.JobRun, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) != len(generations) {
		return nil, fmt.Errorf("pgque claim runs: mismatched id/generation counts")
	}
	projectIDs, queueNames, environmentIDs := workerQueueRefArgs(filter.WorkerRefs)
	rows, err := q.db.Query(ctx, fmt.Sprintf(`
		WITH input AS (
			SELECT *
			FROM unnest($1::text[], $2::bigint[]) WITH ORDINALITY AS u(id, generation, ord)
		),
		raw_candidates AS (
			SELECT s.run_id,
			       input.ord,
			       input.generation,
			       s.job_id,
			       s.project_id,
			       s.concurrency_key,
			       s.job_max_concurrency,
			       s.job_max_concurrency_per_key,
			       s.ready_attempt,
			       s.ready_reason,
			       COALESCE(s.promoted_priority, s.priority) AS claim_priority,
			       jr.created_at AS claim_created_at,
			       jr.payload,
			       jr.result,
			       jr.metadata,
			       jr.error,
			       jr.error_class,
			       jr.triggered_by,
			       jr.parent_run_id,
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
			       jr.is_rollback,
			       jr.replayed_run_id
			FROM input
			JOIN LATERAL (
				SELECT s.*,
				       priority.priority AS promoted_priority,
				       ready.attempt AS ready_attempt,
				       ready.reason AS ready_reason
				FROM job_run_state s
				LEFT JOIN LATERAL (
				    SELECT e.priority
				    FROM job_run_priority_events e
				    WHERE e.run_id = s.run_id
				    ORDER BY e.id DESC
				    LIMIT 1
				) priority ON true
				LEFT JOIN LATERAL (
				    SELECT e.attempt, e.reason
				    FROM job_run_ready_events e
				    WHERE e.run_id = s.run_id
				      AND e.ready_generation = s.ready_generation
				    ORDER BY e.id DESC
				    LIMIT 1
				) ready ON true
				WHERE s.run_id = input.id
				  AND (
				      s.status = $4
				      OR (s.status = 'delayed' AND ready.reason = 'delayed_due')
				      OR (s.status = 'delayed' AND ready.reason = 'worker_recovered')
				      OR (s.status = 'paused' AND ready.reason = 'paused_resume')
				  )
				  AND s.ready_generation = input.generation
				  AND s.execution_mode = $6
				  AND ($7::text = '' OR s.project_id = $7)
				  AND (
				      $6 <> 'worker'
				      OR EXISTS (
				          SELECT 1
				          FROM unnest($8::text[], $9::text[], $10::text[]) AS wq(project_id, queue_name, environment_id)
				          WHERE wq.project_id = s.project_id
				            AND wq.queue_name = s.queue_name
				            AND (wq.environment_id = '' OR s.environment_id = wq.environment_id)
				      )
				  )
				  AND COALESCE(s.job_enabled, true) = true
				  AND COALESCE(s.job_paused, false) = false
				  AND (
				      s.scheduled_at IS NULL
				      OR s.scheduled_at <= NOW()
				      OR ready.reason = 'worker_recovered'
				  )
				  AND (
				      s.next_retry_at IS NULL
				      OR s.next_retry_at <= NOW()
				      OR ready.reason = 'retry_ready'
				      OR ready.reason = 'worker_recovered'
				  )
				  AND NOT strait_run_retry_blocked(s.run_id)
				  AND NOT EXISTS (
				      SELECT 1
				      FROM job_run_active_claims c
				      WHERE c.run_id = s.run_id
				        AND c.ready_generation = s.ready_generation
				  )
				FOR UPDATE OF s SKIP LOCKED
			) s ON true
			JOIN job_runs jr ON jr.id = s.run_id
			ORDER BY s.priority DESC, jr.created_at ASC, input.ord
		),
		limited_jobs AS MATERIALIZED (
			SELECT DISTINCT raw_candidates.job_id
			FROM raw_candidates
			WHERE job_max_concurrency IS NOT NULL
			   OR job_max_concurrency_per_key IS NOT NULL
			ORDER BY raw_candidates.job_id
		),
		job_locks AS MATERIALIZED (
			SELECT pg_advisory_xact_lock(hashtextextended(limited_jobs.job_id, 0)) AS locked
			FROM limited_jobs
		),
		lock_barrier AS MATERIALIZED (
			SELECT COUNT(*) AS locked_jobs FROM job_locks
		),
		active_key_counts AS MATERIALIZED (
			SELECT
				active.job_id,
				COALESCE(active.concurrency_key, '') AS concurrency_key,
				COUNT(*)::int AS count
			FROM job_run_state active
			JOIN limited_jobs limited ON limited.job_id = active.job_id
			JOIN job_run_active_claims claim
			  ON claim.run_id = active.run_id
			 AND claim.ready_generation = active.ready_generation
			LEFT JOIN job_run_terminal_state terminal ON terminal.run_id = active.run_id
			CROSS JOIN lock_barrier
			WHERE (
			      active.status IN ($4, 'delayed')
			      OR (
			          active.status = 'paused'
			          AND EXISTS (
			              SELECT 1
			              FROM job_run_ready_events ready
			              WHERE ready.run_id = active.run_id
			                AND ready.ready_generation = active.ready_generation
			                AND ready.reason = 'paused_resume'
			          )
			      )
			  )
			  AND terminal.run_id IS NULL
			GROUP BY active.job_id, COALESCE(active.concurrency_key, '')
		),
		active_job_counts AS MATERIALIZED (
			SELECT active_key_counts.job_id, SUM(active_key_counts.count)::int AS count
			FROM active_key_counts
			GROUP BY active_key_counts.job_id
		),
		ranked_candidates AS (
			SELECT raw_candidates.*,
			       COALESCE(active_job_counts.count, 0) AS active_count,
			       COALESCE(active_key_counts.count, 0) AS key_active_count,
			       ROW_NUMBER() OVER (PARTITION BY raw_candidates.job_id ORDER BY claim_priority DESC, claim_created_at ASC, ord) AS job_rank,
			       ROW_NUMBER() OVER (PARTITION BY raw_candidates.job_id, raw_candidates.concurrency_key ORDER BY claim_priority DESC, claim_created_at ASC, ord) AS key_rank
			FROM raw_candidates
			CROSS JOIN lock_barrier
			LEFT JOIN active_job_counts ON active_job_counts.job_id = raw_candidates.job_id
			LEFT JOIN active_key_counts
			  ON active_key_counts.job_id = raw_candidates.job_id
			 AND active_key_counts.concurrency_key = COALESCE(raw_candidates.concurrency_key, '')
		),
		candidates AS (
			SELECT *
			FROM ranked_candidates
			WHERE (job_max_concurrency IS NULL OR job_rank <= GREATEST(job_max_concurrency - active_count, 0))
			  AND (
			      job_max_concurrency_per_key IS NULL
			      OR concurrency_key = ''
			      OR key_rank <= GREATEST(job_max_concurrency_per_key - key_active_count, 0)
			  )
			ORDER BY claim_priority DESC, claim_created_at ASC, ord
			LIMIT $3
		),
		inserted_claims AS (
			INSERT INTO job_run_active_claims (
				run_id,
				ready_generation,
				attempt,
				started_at
			)
			SELECT
				s.run_id,
				s.ready_generation,
				COALESCE(candidates.ready_attempt, s.attempt),
				NOW()
			FROM job_run_state s
			JOIN candidates ON candidates.run_id = s.run_id
			WHERE (
			      s.status IN ($4, 'delayed')
			      OR (
			          s.status = 'paused'
			          AND candidates.ready_reason = 'paused_resume'
			      )
			  )
			  AND s.ready_generation = candidates.generation
			  AND NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
			ON CONFLICT (run_id, ready_generation) DO NOTHING
			RETURNING run_id, ready_generation, attempt, started_at
		),
		claimed_state AS (
			SELECT s.run_id,
			       candidates.job_id,
			       candidates.project_id,
			       $5::text AS status,
			       i.attempt,
			       candidates.payload,
			       candidates.result,
			       candidates.metadata,
			       candidates.error,
			       candidates.error_class,
			       candidates.triggered_by,
			       s.scheduled_at,
			       i.started_at,
			       CASE WHEN candidates.ready_reason = 'retry_ready' THEN NULL::timestamptz ELSE s.finished_at END AS finished_at,
			       CASE WHEN candidates.ready_reason = 'retry_ready' THEN NULL::timestamptz ELSE s.heartbeat_at END AS heartbeat_at,
			       CASE WHEN candidates.ready_reason = 'retry_ready' THEN NULL::timestamptz ELSE s.next_retry_at END AS next_retry_at,
			       s.expires_at,
			       candidates.parent_run_id,
			       candidates.claim_priority AS priority,
			       candidates.idempotency_key,
			       candidates.job_version,
			       candidates.created_at,
			       candidates.workflow_step_run_id,
			       candidates.execution_trace,
			       candidates.debug_mode,
			       candidates.continuation_of,
			       candidates.lineage_depth,
			       candidates.tags,
			       candidates.job_version_id,
			       candidates.created_by,
			       candidates.batch_id,
			       s.concurrency_key,
			       s.execution_mode,
			       candidates.is_rollback,
			       candidates.replayed_run_id,
			       candidates.claim_priority,
			       candidates.claim_created_at,
			       candidates.ord
			FROM inserted_claims i
			JOIN job_run_state s ON s.run_id = i.run_id AND s.ready_generation = i.ready_generation
			JOIN candidates ON candidates.run_id = i.run_id
		)
		SELECT %s
		FROM claimed_state u
		ORDER BY u.claim_priority DESC, u.claim_created_at ASC, u.ord`,
		pgQueClaimDequeueColumns,
	), ids, generations, limit, domain.StatusQueued, domain.StatusExecuting, filter.ExecutionMode, filter.ProjectID, projectIDs, queueNames, environmentIDs)
	if err != nil {
		return nil, fmt.Errorf("pgque claim runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, limit)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("pgque claim scan: %w", err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgque claim rows: %w", err)
	}
	return runs, nil
}

var _ Queue = (*PgQueQueue)(nil)
var _ interface {
	EnqueueExisting(context.Context, *domain.JobRun) error
} = (*PgQueQueue)(nil)
var _ interface {
	RunTicker(context.Context)
	Maintain(context.Context) error
} = (*PgQueQueue)(nil)
