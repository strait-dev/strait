package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// ActiveWorkerRef identifies a live worker stream by its tenant-local worker ID.
type ActiveWorkerRef struct {
	WorkerID  string
	ProjectID string
}

func activeWorkerRefsFromIDs(workerIDs []string) []ActiveWorkerRef {
	if len(workerIDs) == 0 {
		return nil
	}
	refs := make([]ActiveWorkerRef, 0, len(workerIDs))
	for _, workerID := range workerIDs {
		if workerID == "" {
			continue
		}
		refs = append(refs, ActiveWorkerRef{WorkerID: workerID})
	}
	return refs
}

func activeWorkerRefArrays(activeWorkers []ActiveWorkerRef) ([]string, []string) {
	workerIDs := make([]string, 0, len(activeWorkers))
	projectIDs := make([]string, 0, len(activeWorkers))
	for _, worker := range activeWorkers {
		if worker.WorkerID == "" || worker.ProjectID == "" {
			continue
		}
		workerIDs = append(workerIDs, worker.WorkerID)
		projectIDs = append(projectIDs, worker.ProjectID)
	}
	return workerIDs, projectIDs
}

// RegisterWorker upserts a worker record scoped by project_id and worker id,
// updating last_seen_at and status. Worker IDs are tenant-local identifiers:
// two projects may use the same worker ID without colliding.
func (q *Queries) RegisterWorker(ctx context.Context, w *domain.Worker) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RegisterWorker")
	defer span.End()

	query := `
		INSERT INTO workers (id, project_id, queue_name, hostname, version, status, last_seen_at, registered_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (project_id, id) DO UPDATE
		SET queue_name   = EXCLUDED.queue_name,
		    hostname     = EXCLUDED.hostname,
		    version      = EXCLUDED.version,
		    status       = EXCLUDED.status,
		    last_seen_at = NOW()`

	_, err := q.db.Exec(ctx, query,
		w.ID, w.ProjectID, w.QueueName, w.Hostname, w.Version, string(w.Status),
	)
	if err != nil {
		return fmt.Errorf("register worker: %w", err)
	}
	return nil
}

// GetWorkerProjectByID returns one project_id for an existing worker id, or
// "" with (false, nil) if no row exists. Worker IDs are scoped by project, so
// this helper is only for legacy diagnostics and must not be used as an
// authorization or registration uniqueness check.
func (q *Queries) GetWorkerProjectByID(ctx context.Context, workerID string) (string, bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkerProjectByID")
	defer span.End()

	var projectID string
	err := q.db.QueryRow(ctx,
		`SELECT project_id FROM workers WHERE id = $1 ORDER BY registered_at DESC LIMIT 1`,
		workerID,
	).Scan(&projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("get worker project by id: %w", err)
	}
	return projectID, true, nil
}

// SetWorkerStatus transitions a worker to a new status within one project.
func (q *Queries) SetWorkerStatus(ctx context.Context, workerID, projectID string, status domain.WorkerStatus) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SetWorkerStatus")
	defer span.End()

	_, err := q.db.Exec(ctx,
		`UPDATE workers SET status = $1, last_seen_at = NOW() WHERE id = $2 AND project_id = $3`,
		string(status), workerID, projectID,
	)
	if err != nil {
		return fmt.Errorf("set worker status: %w", err)
	}
	return nil
}

// GetWorker fetches a worker by ID scoped to a project.
func (q *Queries) GetWorker(ctx context.Context, workerID, projectID string) (*domain.Worker, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorker")
	defer span.End()

	var w domain.Worker
	var status string
	err := q.db.QueryRow(ctx,
		`SELECT id, project_id, queue_name, hostname, version, status, last_seen_at, registered_at
		 FROM workers WHERE id = $1 AND project_id = $2`,
		workerID, projectID,
	).Scan(&w.ID, &w.ProjectID, &w.QueueName, &w.Hostname, &w.Version, &status, &w.LastSeenAt, &w.RegisteredAt)
	if err != nil {
		return nil, fmt.Errorf("get worker: %w", err)
	}
	w.Status = domain.WorkerStatus(status)
	return &w, nil
}

// ListWorkers returns workers for a project, optionally filtered by queue.
func (q *Queries) ListWorkers(ctx context.Context, projectID, queueName string, limit, offset int) ([]domain.Worker, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkers")
	defer span.End()

	query := `SELECT id, project_id, queue_name, hostname, version, status, last_seen_at, registered_at
	          FROM workers WHERE project_id = $1`
	args := []any{projectID}
	param := 2

	if queueName != "" {
		query += fmt.Sprintf(" AND queue_name = $%d", param)
		args = append(args, queueName)
		param++
	}

	query += fmt.Sprintf(" ORDER BY registered_at DESC LIMIT $%d OFFSET $%d", param, param+1)
	args = append(args, limit, offset)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	defer rows.Close()

	var workers []domain.Worker
	for rows.Next() {
		var w domain.Worker
		var status string
		if err := rows.Scan(&w.ID, &w.ProjectID, &w.QueueName, &w.Hostname, &w.Version, &status, &w.LastSeenAt, &w.RegisteredAt); err != nil {
			return nil, fmt.Errorf("list workers scan: %w", err)
		}
		w.Status = domain.WorkerStatus(status)
		workers = append(workers, w)
	}
	return workers, rows.Err()
}

// EvictStaleWorkers marks workers offline if they have not sent a heartbeat since cutoff.
func (q *Queries) EvictStaleWorkers(ctx context.Context, cutoff time.Time) (int64, error) {
	return q.EvictStaleWorkersExceptRefs(ctx, cutoff, nil)
}

// EvictStaleWorkersExcept marks stale workers offline unless they are known to
// still be connected on this replica.
func (q *Queries) EvictStaleWorkersExcept(ctx context.Context, cutoff time.Time, activeWorkerIDs []string) (int64, error) {
	return q.EvictStaleWorkersExceptRefs(ctx, cutoff, activeWorkerRefsFromIDs(activeWorkerIDs))
}

// EvictStaleWorkersExceptRefs marks stale workers offline unless the exact
// (project_id, worker_id) pair is known to still be connected on this replica.
func (q *Queries) EvictStaleWorkersExceptRefs(ctx context.Context, cutoff time.Time, activeWorkers []ActiveWorkerRef) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.EvictStaleWorkers")
	defer span.End()
	activeWorkerIDs, activeProjectIDs := activeWorkerRefArrays(activeWorkers)

	tag, err := q.db.Exec(ctx,
		`UPDATE workers
		 SET status = 'offline'
		 WHERE last_seen_at < $1
		   AND (stream_lease_expires_at IS NULL OR stream_lease_expires_at < NOW())
		   AND status != 'offline'
		   AND NOT EXISTS (
		     SELECT 1
		     FROM unnest($2::text[], $3::text[]) AS active(id, project_id)
		     WHERE active.id = workers.id
		       AND active.project_id = workers.project_id
		   )`,
		cutoff, activeWorkerIDs, activeProjectIDs,
	)
	if err != nil {
		return 0, fmt.Errorf("evict stale workers: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RecoverStaleWorkerTasks marks stale workers' open assignments failed and
// requeues still-executing worker-mode runs before the worker row is evicted.
func (q *Queries) RecoverStaleWorkerTasks(ctx context.Context, cutoff time.Time, reason string) (int64, error) {
	return q.RecoverStaleWorkerTasksExceptRefs(ctx, cutoff, reason, nil)
}

// RecoverStaleWorkerTasksExcept requeues open tasks owned by stale workers
// unless the worker is still connected in the caller's local registry.
func (q *Queries) RecoverStaleWorkerTasksExcept(ctx context.Context, cutoff time.Time, reason string, activeWorkerIDs []string) (int64, error) {
	return q.RecoverStaleWorkerTasksExceptRefs(ctx, cutoff, reason, activeWorkerRefsFromIDs(activeWorkerIDs))
}

// RecoverStaleWorkerTasksExceptRefs requeues open tasks owned by stale workers
// unless the exact (project_id, worker_id) pair is still connected locally.
func (q *Queries) RecoverStaleWorkerTasksExceptRefs(ctx context.Context, cutoff time.Time, reason string, activeWorkers []ActiveWorkerRef) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RecoverStaleWorkerTasks")
	defer span.End()
	activeWorkerIDs, activeProjectIDs := activeWorkerRefArrays(activeWorkers)

	_, ok := q.db.(TxBeginner)
	if !ok {
		return 0, fmt.Errorf("recover stale worker tasks requires transaction support")
	}

	var affected int64
	err := q.withTx(ctx, func(txQ *Queries) error {
		const query = `
			WITH stale_workers AS (
				SELECT id, project_id
				FROM workers
				WHERE last_seen_at < $1
				  AND (stream_lease_expires_at IS NULL OR stream_lease_expires_at < NOW())
				  AND NOT EXISTS (
				    SELECT 1
				    FROM unnest($3::text[], $4::text[]) AS active(id, project_id)
				    WHERE active.id = workers.id
				      AND active.project_id = workers.project_id
				  )
			),
			open_tasks AS (
				SELECT wt.id, wt.run_id
				FROM worker_tasks wt
				JOIN stale_workers sw
				  ON sw.id = wt.worker_id
				 AND sw.project_id = wt.project_id
				WHERE wt.status IN ('assigned', 'accepted')
			),
			candidates AS MATERIALIZED (
				SELECT
					ot.id AS task_id,
					s.run_id,
					s.job_id,
					COALESCE(s.concurrency_key, '') AS concurrency_key,
					COALESCE(c.attempt, s.attempt) AS attempt,
					s.status AS from_status,
					s.ready_generation,
					s.job_max_concurrency,
					s.job_max_concurrency_per_key,
					c.run_id IS NOT NULL AS uses_active_claim
				FROM open_tasks ot
				JOIN job_run_state s ON s.run_id = ot.run_id
				LEFT JOIN job_run_active_claims c
				  ON c.run_id = s.run_id
				 AND c.ready_generation = s.ready_generation
				WHERE NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
				  AND (s.status = 'executing' OR (s.status IN ('queued', 'delayed') AND c.run_id IS NOT NULL))
				FOR UPDATE OF s SKIP LOCKED
			),
			requeued_runs AS (
				UPDATE job_run_state s
				SET status = CASE WHEN c.uses_active_claim THEN s.status ELSE 'queued' END,
				    started_at = NULL,
				    finished_at = NULL,
				    heartbeat_at = NULL,
				    next_retry_at = NULL,
				    ready_generation = s.ready_generation + 1,
				    updated_at = NOW()
				FROM candidates c
				WHERE s.run_id = c.run_id
				RETURNING s.run_id, c.job_id, c.concurrency_key, c.attempt, c.from_status,
				          c.job_max_concurrency, c.job_max_concurrency_per_key,
				          c.uses_active_claim, s.ready_generation
			),
			ready_events AS (
				INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
				SELECT run_id, ready_generation, attempt, 'worker_recovered'
				FROM requeued_runs
				WHERE uses_active_claim
				ON CONFLICT (run_id, ready_generation, reason) DO NOTHING
				RETURNING 1
			),
			released AS (
				UPDATE job_active_counts c
				SET count = GREATEST(c.count - 1, 0),
				    updated_at = NOW()
				FROM requeued_runs r
				WHERE (r.job_max_concurrency IS NOT NULL OR r.job_max_concurrency_per_key IS NOT NULL)
				  AND NOT r.uses_active_claim
				  AND c.job_id = r.job_id
				  AND c.concurrency_key = r.concurrency_key
				  AND c.count <> 0
				RETURNING 1
			),
			lifecycle_events AS (
				INSERT INTO job_run_lifecycle_events (run_id, from_status, to_status, attempt, fields)
				SELECT run_id, from_status, 'queued', attempt,
				       jsonb_build_object('error', $2::text, 'error_class', 'transient')
				FROM requeued_runs
				RETURNING 1
			),
			cache_versions AS (
				INSERT INTO job_run_cache_versions (run_id, cache_version)
				SELECT run_id, strait_next_run_cache_version(run_id)
				FROM requeued_runs
				RETURNING 1
			),
			failed_tasks AS (
				UPDATE worker_tasks
				SET status = 'failed',
				    finished_at = NOW()
				WHERE id IN (SELECT id FROM open_tasks)
				RETURNING id
			)
			SELECT COUNT(*) FROM failed_tasks`

		if err := txQ.db.QueryRow(ctx, query, cutoff, reason, activeWorkerIDs, activeProjectIDs).Scan(&affected); err != nil {
			return fmt.Errorf("recover stale worker tasks: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	return affected, nil
}

// ListRecoverableStaleWorkerTaskRunIDs returns the run IDs that the stale
// worker recovery query is eligible to move back to queued.
func (q *Queries) ListRecoverableStaleWorkerTaskRunIDs(ctx context.Context, cutoff time.Time, activeWorkers []ActiveWorkerRef) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRecoverableStaleWorkerTaskRunIDs")
	defer span.End()
	activeWorkerIDs, activeProjectIDs := activeWorkerRefArrays(activeWorkers)

	rows, err := q.db.Query(ctx, `
		WITH stale_workers AS (
			SELECT id, project_id
			FROM workers
			WHERE last_seen_at < $1
			  AND (stream_lease_expires_at IS NULL OR stream_lease_expires_at < NOW())
			  AND NOT EXISTS (
			    SELECT 1
			    FROM unnest($2::text[], $3::text[]) AS active(id, project_id)
			    WHERE active.id = workers.id
			      AND active.project_id = workers.project_id
			  )
		),
		open_tasks AS (
			SELECT DISTINCT wt.run_id
			FROM worker_tasks wt
			JOIN stale_workers sw
			  ON sw.id = wt.worker_id
			 AND sw.project_id = wt.project_id
			WHERE wt.status IN ('assigned', 'accepted')
		)
		SELECT ot.run_id
		FROM open_tasks ot
		JOIN job_run_state s ON s.run_id = ot.run_id
		LEFT JOIN job_run_active_claims c
		  ON c.run_id = s.run_id
		 AND c.ready_generation = s.ready_generation
		WHERE NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
		  AND (s.status = 'executing' OR (s.status IN ('queued', 'delayed') AND c.run_id IS NOT NULL))`,
		cutoff,
		activeWorkerIDs,
		activeProjectIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("list recoverable stale worker task runs: %w", err)
	}
	defer rows.Close()

	runIDs := make([]string, 0)
	for rows.Next() {
		var runID string
		if err := rows.Scan(&runID); err != nil {
			return nil, fmt.Errorf("scan recoverable stale worker task run: %w", err)
		}
		runIDs = append(runIDs, runID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recoverable stale worker task runs: %w", err)
	}
	return runIDs, nil
}

// RenewWorkerStreamLease records an authoritative cross-replica lease for an
// active worker stream. Stale recovery must not requeue tasks while this lease
// remains valid, even if last_seen_at has fallen behind.
func (q *Queries) RenewWorkerStreamLease(ctx context.Context, workerID, projectID string, expiresAt time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RenewWorkerStreamLease")
	defer span.End()

	_, err := q.db.Exec(ctx,
		`UPDATE workers
		 SET stream_lease_expires_at = $1
		 WHERE id = $2
		   AND project_id = $3`,
		expiresAt,
		workerID,
		projectID,
	)
	if err != nil {
		return fmt.Errorf("renew worker stream lease: %w", err)
	}
	return nil
}

// DeleteStaleOfflineWorkers removes old offline worker rows once they have no
// open task handoff state. This prevents stale rows from reserving a globally
// keyed worker_id forever while preserving recent disconnect history.
func (q *Queries) DeleteStaleOfflineWorkers(ctx context.Context, cutoff time.Time) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteStaleOfflineWorkers")
	defer span.End()

	tag, err := q.db.Exec(ctx,
		`DELETE FROM workers w
		 WHERE w.status = 'offline'
		   AND w.last_seen_at < $1
		   AND NOT EXISTS (
			SELECT 1
			FROM worker_tasks wt
			WHERE wt.worker_id = w.id
			  AND wt.project_id = w.project_id
			  AND wt.status IN ('assigned', 'accepted', 'result_received', 'finalizing')
		   )`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("delete stale offline workers: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CreateWorkerTask records a task assignment.
func (q *Queries) CreateWorkerTask(ctx context.Context, t *domain.WorkerTask) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkerTask")
	defer span.End()
	if t.Attempt <= 0 {
		t.Attempt = 1
	}

	_, err := q.db.Exec(ctx,
		`INSERT INTO worker_tasks (id, worker_id, run_id, project_id, attempt, status, assigned_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW())`,
		t.ID, t.WorkerID, t.RunID, t.ProjectID, t.Attempt, string(t.Status),
	)
	if err != nil {
		return fmt.Errorf("create worker task: %w", err)
	}
	return nil
}

// UpdateWorkerTaskStatus transitions a worker task to a new status.
func (q *Queries) UpdateWorkerTaskStatus(ctx context.Context, taskID string, status domain.WorkerTaskStatus) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateWorkerTaskStatus")
	defer span.End()

	var query string
	switch status {
	case domain.WorkerTaskStatusAccepted:
		query = `UPDATE worker_tasks SET status = $1, accepted_at = NOW() WHERE id = $2 AND status IS DISTINCT FROM $1`
	case domain.WorkerTaskStatusResultReceived:
		query = `UPDATE worker_tasks SET status = $1 WHERE id = $2 AND status IS DISTINCT FROM $1`
	case domain.WorkerTaskStatusCompleted, domain.WorkerTaskStatusFailed:
		query = `UPDATE worker_tasks SET status = $1, finished_at = NOW() WHERE id = $2 AND status IS DISTINCT FROM $1`
	default:
		query = `UPDATE worker_tasks SET status = $1 WHERE id = $2 AND status IS DISTINCT FROM $1`
	}

	_, err := q.db.Exec(ctx, query, string(status), taskID)
	if err != nil {
		return fmt.Errorf("update worker task status: %w", err)
	}
	return nil
}

// MarkWorkerTaskResultReceived closes an active assignment to disconnect
// requeue once its TaskResult has reached the API process. The executor still
// moves the row to completed/failed only after run finalization succeeds.
func (q *Queries) MarkWorkerTaskResultReceived(ctx context.Context, taskID string) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkWorkerTaskResultReceived")
	defer span.End()

	var marked bool
	err := q.db.QueryRow(ctx,
		`WITH updated AS (
			UPDATE worker_tasks
			SET status = $1
			WHERE id = $2
			  AND status IN ('assigned', 'accepted')
			RETURNING 1
		),
		existing AS (
			SELECT 1
			FROM worker_tasks
			WHERE id = $2
			  AND status = $1
			  AND NOT EXISTS (SELECT 1 FROM updated)
		)
		SELECT EXISTS (SELECT 1 FROM updated UNION ALL SELECT 1 FROM existing)`,
		string(domain.WorkerTaskStatusResultReceived), taskID,
	).Scan(&marked)
	if err != nil {
		return false, fmt.Errorf("mark worker task result received: %w", err)
	}
	return marked, nil
}

// MarkWorkerTaskResultReceivedByAssignment durably records a worker result for
// one exact assignment before the in-memory dispatch waiter is notified.
func (q *Queries) MarkWorkerTaskResultReceivedByAssignment(
	ctx context.Context,
	taskID string,
	workerID string,
	projectID string,
	runID string,
	attempt int,
	status string,
	errorMessage string,
	output []byte,
	durationMS int64,
) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkWorkerTaskResultReceivedByAssignment")
	defer span.End()

	if attempt <= 0 {
		return false, nil
	}
	var outputJSON any
	if len(output) > 0 {
		if json.Valid(output) {
			outputJSON = json.RawMessage(output)
		}
	}

	tag, err := q.db.Exec(ctx,
		`UPDATE worker_tasks
		 SET status = $1,
		     result_status = $2,
		     result_error = NULLIF($3, ''),
		     result_output = $4,
		     result_duration_ms = $5,
		     result_received_at = NOW()
		 WHERE id = $6
		   AND worker_id = $7
		   AND project_id = $8
		   AND run_id = $9
		   AND attempt = $10
		   AND status IN ('assigned', 'accepted')`,
		string(domain.WorkerTaskStatusResultReceived),
		status,
		errorMessage,
		outputJSON,
		durationMS,
		taskID,
		workerID,
		projectID,
		runID,
		attempt,
	)
	if err != nil {
		return false, fmt.Errorf("mark worker task result received by assignment: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// ClaimRecoverableWorkerTaskResults claims durable worker results that reached
// the stream boundary but were never finalized, usually because the API process
// crashed after the handoff.
func (q *Queries) ClaimRecoverableWorkerTaskResults(ctx context.Context, cutoff time.Time, limit int) ([]domain.WorkerTask, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClaimRecoverableWorkerTaskResults")
	defer span.End()

	if limit <= 0 {
		limit = 100
	}
	rows, err := q.db.Query(ctx,
		`WITH target AS (
			SELECT wt.id
			FROM worker_tasks wt
			JOIN job_runs jr ON jr.id = wt.run_id
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE wt.status = $1
			  AND wt.result_status IS NOT NULL
			  AND wt.result_received_at IS NOT NULL
			  AND wt.result_received_at < $2
			  AND COALESCE(s.status, jr.status) = 'executing'
			ORDER BY wt.result_received_at ASC
			LIMIT $3
			FOR UPDATE OF wt SKIP LOCKED
		)
		UPDATE worker_tasks wt
		SET status = $4
		FROM target
		WHERE wt.id = target.id
		RETURNING wt.id, wt.worker_id, wt.run_id, wt.project_id, wt.attempt, wt.status,
		          wt.assigned_at, wt.accepted_at, wt.finished_at,
		          wt.result_status, wt.result_output, wt.result_error, wt.result_duration_ms, wt.result_received_at`,
		string(domain.WorkerTaskStatusResultReceived),
		cutoff,
		limit,
		string(domain.WorkerTaskStatusFinalizing),
	)
	if err != nil {
		return nil, fmt.Errorf("claim recoverable worker task results: %w", err)
	}
	defer rows.Close()

	var tasks []domain.WorkerTask
	for rows.Next() {
		var task domain.WorkerTask
		var status string
		var resultStatus, resultError *string
		var resultOutput []byte
		var resultDurationMS *int64
		var resultReceivedAt *time.Time
		if err := rows.Scan(
			&task.ID, &task.WorkerID, &task.RunID, &task.ProjectID, &task.Attempt, &status,
			&task.AssignedAt, &task.AcceptedAt, &task.FinishedAt,
			&resultStatus, &resultOutput, &resultError, &resultDurationMS, &resultReceivedAt,
		); err != nil {
			return nil, fmt.Errorf("claim recoverable worker task results scan: %w", err)
		}
		task.Status = domain.WorkerTaskStatus(status)
		task.Result = buildWorkerTaskResult(resultStatus, resultOutput, resultError, resultDurationMS, resultReceivedAt)
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim recoverable worker task results rows: %w", err)
	}
	return tasks, nil
}

// ResetWorkerTaskFinalizingToResultReceived releases a recovery claim after a
// transient finalizer failure so a later sweep can retry.
func (q *Queries) ResetWorkerTaskFinalizingToResultReceived(ctx context.Context, taskID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ResetWorkerTaskFinalizingToResultReceived")
	defer span.End()

	_, err := q.db.Exec(ctx,
		`UPDATE worker_tasks
		 SET status = $1
		 WHERE id = $2
		   AND status = $3`,
		string(domain.WorkerTaskStatusResultReceived),
		taskID,
		string(domain.WorkerTaskStatusFinalizing),
	)
	if err != nil {
		return fmt.Errorf("reset worker task finalizing to result received: %w", err)
	}
	return nil
}

// MarkOpenWorkerTaskResultReceivedByRunID closes the latest open assignment
// for a worker/run pair to disconnect requeue as soon as its TaskResult reaches
// the API stream boundary.
func (q *Queries) MarkOpenWorkerTaskResultReceivedByRunID(ctx context.Context, workerID, projectID, runID string) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkOpenWorkerTaskResultReceivedByRunID")
	defer span.End()

	tag, err := q.db.Exec(ctx,
		`WITH target AS (
			SELECT id
			FROM worker_tasks
			WHERE worker_id = $1
			  AND project_id = $2
			  AND run_id = $3
			  AND status IN ('assigned', 'accepted')
			ORDER BY assigned_at DESC
			LIMIT 1
		)
		UPDATE worker_tasks
		SET status = $4
		WHERE id IN (SELECT id FROM target)`,
		workerID, projectID, runID, string(domain.WorkerTaskStatusResultReceived),
	)
	if err != nil {
		return false, fmt.Errorf("mark open worker task result received by run id: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// GetWorkerTask fetches a task by ID.
func (q *Queries) GetWorkerTask(ctx context.Context, taskID string) (*domain.WorkerTask, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkerTask")
	defer span.End()

	var t domain.WorkerTask
	var status string
	var resultStatus, resultError *string
	var resultOutput []byte
	var resultDurationMS *int64
	var resultReceivedAt *time.Time
	err := q.db.QueryRow(ctx,
		`SELECT id, worker_id, run_id, project_id, attempt, status, assigned_at, accepted_at, finished_at,
		        result_status, result_output, result_error, result_duration_ms, result_received_at
		 FROM worker_tasks WHERE id = $1`,
		taskID,
	).Scan(
		&t.ID, &t.WorkerID, &t.RunID, &t.ProjectID, &t.Attempt, &status, &t.AssignedAt, &t.AcceptedAt, &t.FinishedAt,
		&resultStatus, &resultOutput, &resultError, &resultDurationMS, &resultReceivedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get worker task: %w", err)
	}
	t.Status = domain.WorkerTaskStatus(status)
	t.Result = buildWorkerTaskResult(resultStatus, resultOutput, resultError, resultDurationMS, resultReceivedAt)
	return &t, nil
}

// GetOpenWorkerTaskByAssignment fetches the active task row that exactly
// matches a worker result's assignment identity.
func (q *Queries) GetOpenWorkerTaskByAssignment(ctx context.Context, taskID, workerID, projectID, runID string, attempt int) (*domain.WorkerTask, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetOpenWorkerTaskByAssignment")
	defer span.End()

	if attempt <= 0 {
		return nil, nil //nolint:nilnil // nil task is the store contract for "no active assignment"
	}
	var t domain.WorkerTask
	var status string
	err := q.db.QueryRow(ctx,
		`SELECT id, worker_id, run_id, project_id, attempt, status, assigned_at, accepted_at, finished_at
		 FROM worker_tasks
		 WHERE id = $1
		   AND worker_id = $2
		   AND project_id = $3
		   AND run_id = $4
		   AND attempt = $5
		   AND status IN ('assigned', 'accepted')
		 LIMIT 1`,
		taskID, workerID, projectID, runID, attempt,
	).Scan(&t.ID, &t.WorkerID, &t.RunID, &t.ProjectID, &t.Attempt, &status, &t.AssignedAt, &t.AcceptedAt, &t.FinishedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil //nolint:nilnil // nil task is the store contract for "no active assignment"
		}
		return nil, fmt.Errorf("get open worker task by assignment: %w", err)
	}
	t.Status = domain.WorkerTaskStatus(status)
	return &t, nil
}

// GetOpenWorkerTaskByRunID fetches the latest non-terminal worker_tasks row
// for a run, scoped to both worker and project. This is the ownership check
// used by worker streams: historical failed/completed assignments must not
// authorize late results or logs after a run has been requeued.
func (q *Queries) GetOpenWorkerTaskByRunID(ctx context.Context, workerID, projectID, runID string) (*domain.WorkerTask, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetOpenWorkerTaskByRunID")
	defer span.End()

	var t domain.WorkerTask
	var status string
	err := q.db.QueryRow(ctx,
		`SELECT id, worker_id, run_id, project_id, attempt, status, assigned_at, accepted_at, finished_at
		 FROM worker_tasks
		 WHERE worker_id = $1
		   AND project_id = $2
		   AND run_id = $3
		   AND status IN ('assigned', 'accepted')
		 ORDER BY assigned_at DESC
		 LIMIT 1`,
		workerID, projectID, runID,
	).Scan(&t.ID, &t.WorkerID, &t.RunID, &t.ProjectID, &t.Attempt, &status, &t.AssignedAt, &t.AcceptedAt, &t.FinishedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil //nolint:nilnil // nil task is the store contract for "no active assignment"
		}
		return nil, fmt.Errorf("get open worker task by run id: %w", err)
	}
	t.Status = domain.WorkerTaskStatus(status)
	return &t, nil
}

// ListWorkerTasksByWorker lists tasks assigned to a worker, scoped to a
// project, optionally filtered by status. The project filter is defense in
// depth: callers should already have verified the worker belongs to the
// project, but matching on project_id at the SQL layer prevents any future
// caller (or any future cross-project worker_id artifact) from leaking tasks
// that happen to share a worker_id across projects.
func (q *Queries) ListWorkerTasksByWorker(ctx context.Context, workerID, projectID string, status domain.WorkerTaskStatus, limit, offset int) ([]domain.WorkerTask, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkerTasksByWorker")
	defer span.End()

	query := `SELECT id, worker_id, run_id, project_id, attempt, status, assigned_at, accepted_at, finished_at
	          FROM worker_tasks WHERE worker_id = $1 AND project_id = $2`
	args := []any{workerID, projectID}
	param := 3

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", param)
		args = append(args, string(status))
		param++
	}

	query += fmt.Sprintf(" ORDER BY assigned_at DESC LIMIT $%d OFFSET $%d", param, param+1)
	args = append(args, limit, offset)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list worker tasks by worker: %w", err)
	}
	defer rows.Close()

	var tasks []domain.WorkerTask
	for rows.Next() {
		var t domain.WorkerTask
		var s string
		if err := rows.Scan(&t.ID, &t.WorkerID, &t.RunID, &t.ProjectID, &t.Attempt, &s, &t.AssignedAt, &t.AcceptedAt, &t.FinishedAt); err != nil {
			return nil, fmt.Errorf("list worker tasks scan: %w", err)
		}
		t.Status = domain.WorkerTaskStatus(s)
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// RequeueOpenWorkerTasks marks a disconnected worker's open assignments as
// failed and requeues any still-executing runs they owned.
func (q *Queries) RequeueOpenWorkerTasks(ctx context.Context, workerID, projectID, reason string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RequeueOpenWorkerTasks")
	defer span.End()

	_, ok := q.db.(TxBeginner)
	if !ok {
		return 0, fmt.Errorf("requeue open worker tasks requires transaction support")
	}

	var affected int64
	err := q.withTx(ctx, func(txQ *Queries) error {
		const query = `
			WITH open_tasks AS (
				SELECT id, run_id
				FROM worker_tasks
				WHERE worker_id = $1
				  AND project_id = $2
				  AND status IN ('assigned', 'accepted')
			),
			candidates AS MATERIALIZED (
				SELECT
					ot.id AS task_id,
					s.run_id,
					s.job_id,
					COALESCE(s.concurrency_key, '') AS concurrency_key,
					COALESCE(c.attempt, s.attempt) AS attempt,
					s.status AS from_status,
					s.ready_generation,
					s.job_max_concurrency,
					s.job_max_concurrency_per_key,
					c.run_id IS NOT NULL AS uses_active_claim
				FROM open_tasks ot
				JOIN job_run_state s ON s.run_id = ot.run_id
				LEFT JOIN job_run_active_claims c
				  ON c.run_id = s.run_id
				 AND c.ready_generation = s.ready_generation
				WHERE NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
				  AND (s.status = 'executing' OR (s.status IN ('queued', 'delayed') AND c.run_id IS NOT NULL))
				FOR UPDATE OF s SKIP LOCKED
			),
			requeued_runs AS (
				UPDATE job_run_state s
				SET status = CASE WHEN c.uses_active_claim THEN s.status ELSE 'queued' END,
				    started_at = NULL,
				    finished_at = NULL,
				    heartbeat_at = NULL,
				    next_retry_at = NULL,
				    ready_generation = s.ready_generation + 1,
				    updated_at = NOW()
				FROM candidates c
				WHERE s.run_id = c.run_id
				RETURNING s.run_id, c.job_id, c.concurrency_key, c.attempt, c.from_status,
				          c.job_max_concurrency, c.job_max_concurrency_per_key,
				          c.uses_active_claim, s.ready_generation
			),
			ready_events AS (
				INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
				SELECT run_id, ready_generation, attempt, 'worker_recovered'
				FROM requeued_runs
				WHERE uses_active_claim
				ON CONFLICT (run_id, ready_generation, reason) DO NOTHING
				RETURNING 1
			),
			released AS (
				UPDATE job_active_counts c
				SET count = GREATEST(c.count - 1, 0),
				    updated_at = NOW()
				FROM requeued_runs r
				WHERE (r.job_max_concurrency IS NOT NULL OR r.job_max_concurrency_per_key IS NOT NULL)
				  AND NOT r.uses_active_claim
				  AND c.job_id = r.job_id
				  AND c.concurrency_key = r.concurrency_key
				  AND c.count <> 0
				RETURNING 1
			),
			lifecycle_events AS (
				INSERT INTO job_run_lifecycle_events (run_id, from_status, to_status, attempt, fields)
				SELECT run_id, from_status, 'queued', attempt,
				       jsonb_build_object('error', $3::text, 'error_class', 'transient')
				FROM requeued_runs
				RETURNING 1
			),
			cache_versions AS (
				INSERT INTO job_run_cache_versions (run_id, cache_version)
				SELECT run_id, strait_next_run_cache_version(run_id)
				FROM requeued_runs
				RETURNING 1
			),
			failed_tasks AS (
				UPDATE worker_tasks
				SET status = 'failed',
				    finished_at = NOW()
				WHERE id IN (SELECT id FROM open_tasks)
				RETURNING id
			)
			SELECT COUNT(*) FROM failed_tasks`

		if err := txQ.db.QueryRow(ctx, query, workerID, projectID, reason).Scan(&affected); err != nil {
			return fmt.Errorf("requeue open worker tasks: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	return affected, nil
}

func buildWorkerTaskResult(status *string, output []byte, errText *string, durationMS *int64, receivedAt *time.Time) *domain.WorkerTaskResult {
	if !hasWorkerTaskResultFields(status, output, errText, durationMS, receivedAt) {
		return nil
	}
	result := &domain.WorkerTaskResult{ReceivedAt: receivedAt}
	if status != nil {
		result.Status = *status
	}
	if len(output) > 0 {
		result.Output = json.RawMessage(append([]byte(nil), output...))
	}
	if errText != nil {
		result.Error = *errText
	}
	if durationMS != nil {
		result.DurationMS = *durationMS
	}
	return result
}

func hasWorkerTaskResultFields(status *string, output []byte, errText *string, durationMS *int64, receivedAt *time.Time) bool {
	return status != nil || len(output) > 0 || errText != nil || durationMS != nil || receivedAt != nil
}
