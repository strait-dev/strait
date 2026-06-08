package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"strait/internal/dbscan"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/jackc/pgx/v5"
)

const pgQueSmallReadyWorkerJobSetLimit = 8

func (q *PgQueQueue) sendReadyEvent(ctx context.Context, db store.DBTX, run *domain.JobRun) error {
	routeKey, err := q.routeKeyForRun(ctx, db, run)
	if err != nil {
		return err
	}
	queueName := pgQueQueueName(routeKey)
	if err := q.prepareReadyRoute(ctx, routeKey, queueName); err != nil {
		return err
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
	if err := q.recordReadyEmitBatch(ctx, db, []string{run.ID}, []int64{generation}); err != nil {
		return err
	}
	return nil
}

func (q *PgQueQueue) sendReadyEvents(ctx context.Context, db store.DBTX, runs []*domain.JobRun) error {
	readyRuns, runIDs, err := q.readyRunsForEvents(ctx, db, runs)
	if err != nil {
		return err
	}
	if len(readyRuns) == 0 {
		return nil
	}
	generations, err := q.readyGenerations(ctx, db, runIDs)
	if err != nil {
		return err
	}

	routeKey := readyRuns[0].routeKey
	payloads := make([]string, 0, len(readyRuns))
	var byRoute map[string][]string
	readyGenerations := make([]int64, 0, len(readyRuns))
	for _, readyRun := range readyRuns {
		generation, ok := generations[readyRun.run.ID]
		if !ok {
			return fmt.Errorf("pgque ready generation: missing run %s", readyRun.run.ID)
		}
		readyGenerations = append(readyGenerations, generation)
		payload, err := json.Marshal(pgQueReadyEvent{
			RunID:      readyRun.run.ID,
			RouteKey:   readyRun.routeKey,
			Generation: generation,
			Priority:   readyRun.run.Priority,
		})
		if err != nil {
			return fmt.Errorf("pgque ready event: marshal: %w", err)
		}
		payloadText := string(payload)
		if byRoute != nil {
			byRoute[readyRun.routeKey] = append(byRoute[readyRun.routeKey], payloadText)
			continue
		}
		if readyRun.routeKey == routeKey {
			payloads = append(payloads, payloadText)
			continue
		}
		byRoute = make(map[string][]string, len(readyRuns))
		byRoute[routeKey] = payloads
		byRoute[readyRun.routeKey] = append(byRoute[readyRun.routeKey], payloadText)
	}
	if byRoute == nil {
		if err := q.sendReadyPayloadBatch(ctx, db, routeKey, payloads); err != nil {
			return err
		}
	} else {
		for routeKey, payloads := range byRoute {
			if err := q.sendReadyPayloadBatch(ctx, db, routeKey, payloads); err != nil {
				return err
			}
		}
	}
	if err := q.recordReadyEmitBatch(ctx, db, runIDs, readyGenerations); err != nil {
		return err
	}
	return nil
}

func (q *PgQueQueue) sendReadyPayloadBatch(ctx context.Context, db store.DBTX, routeKey string, payloads []string) error {
	queueName := pgQueQueueName(routeKey)
	if err := q.prepareReadyRoute(ctx, routeKey, queueName); err != nil {
		return err
	}
	if err := q.pgque(db).sendTextBatch(ctx, queueName, pgQueReadyEventType, payloads); err != nil {
		return fmt.Errorf("pgque send ready event batch: %w", err)
	}
	return nil
}

func (q *PgQueQueue) prepareReadyRoute(ctx context.Context, routeKey, queueName string) error {
	if q.routeConfigured(routeKey) {
		return nil
	}
	state := q.routeState(routeKey)
	return q.ensureRouteCached(ctx, state, routeKey, queueName)
}

func (q *PgQueQueue) readyRunsForEvents(ctx context.Context, db store.DBTX, runs []*domain.JobRun) ([]pgQueReadyRun, []string, error) {
	var readyRuns []pgQueReadyRun
	var runIDs []string
	var workerJobIDs []string
	var seenWorkerJobs map[string]struct{}
	for _, run := range runs {
		if run == nil || run.Status != domain.StatusQueued {
			continue
		}
		if readyRuns == nil {
			readyRuns = make([]pgQueReadyRun, 0, len(runs))
			runIDs = make([]string, 0, len(runs))
		}
		readyRuns = append(readyRuns, pgQueReadyRun{run: run})
		runIDs = append(runIDs, run.ID)
		if run.ExecutionMode != domain.ExecutionModeWorker {
			continue
		}
		if run.JobID == "" {
			return nil, nil, fmt.Errorf("pgque worker route lookup: missing job id for run %s", run.ID)
		}
		workerJobIDs, seenWorkerJobs = appendUniqueReadyWorkerJobID(workerJobIDs, seenWorkerJobs, run.JobID)
	}
	if len(workerJobIDs) == 0 {
		for i := range readyRuns {
			readyRuns[i].routeKey = pgQueHTTPRouteKey
		}
		return readyRuns, runIDs, nil
	}
	workerRoutes, err := q.workerJobRoutes(ctx, db, workerJobIDs)
	if err != nil {
		return nil, nil, err
	}
	for i := range readyRuns {
		run := readyRuns[i].run
		if run.ExecutionMode != domain.ExecutionModeWorker {
			readyRuns[i].routeKey = pgQueHTTPRouteKey
			continue
		}
		route, ok := workerRoutes[run.JobID]
		if !ok {
			return nil, nil, fmt.Errorf("pgque worker route lookup: missing job %s", run.JobID)
		}
		queueName := runQueueName(run.QueueName)
		if run.QueueName == "" {
			queueName = route.queueName
		}
		readyRuns[i].routeKey = pgQueWorkerRouteKey(run.ProjectID, queueName, route.environmentID)
	}
	return readyRuns, runIDs, nil
}

func appendUniqueReadyWorkerJobID(workerJobIDs []string, seen map[string]struct{}, jobID string) ([]string, map[string]struct{}) {
	if seen != nil {
		if _, ok := seen[jobID]; ok {
			return workerJobIDs, seen
		}
		seen[jobID] = struct{}{}
		return append(workerJobIDs, jobID), seen
	}

	for _, workerJobID := range workerJobIDs {
		if workerJobID == jobID {
			return workerJobIDs, nil
		}
	}
	workerJobIDs = append(workerJobIDs, jobID)
	if len(workerJobIDs) > pgQueSmallReadyWorkerJobSetLimit {
		seen = make(map[string]struct{}, len(workerJobIDs))
		for _, workerJobID := range workerJobIDs {
			seen[workerJobID] = struct{}{}
		}
	}
	return workerJobIDs, seen
}

type pgQueReadyRun struct {
	run      *domain.JobRun
	routeKey string
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
	return scanPgQueReadyRuns(rows, min(limit, 1024), "pgque promote due runs")
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
	return scanPgQueReadyRuns(rows, min(limit, 1024), "pgque promote ready retries")
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

	return scanPgQueReadyRuns(rows, 16, "pgque requeue paused job runs")
}

func scanPgQueReadyRuns(rows pgx.Rows, capacity int, label string) ([]*domain.JobRun, error) {
	defer rows.Close()

	runs := make([]*domain.JobRun, 0, capacity)
	for rows.Next() {
		run, scanErr := dbscan.ScanRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("%s scan: %w", label, scanErr)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", label, err)
	}
	return runs, nil
}

func (q *PgQueQueue) recordReadyEmitBatch(ctx context.Context, db store.DBTX, runIDs []string, readyGenerations []int64) error {
	if len(runIDs) == 0 {
		return nil
	}
	if len(runIDs) != len(readyGenerations) {
		return fmt.Errorf("pgque record ready emits: mismatched id/generation counts")
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
