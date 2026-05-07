package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// RegisterWorker upserts a worker record, updating last_seen_at and status.
//
// The UPDATE branch is gated on workers.project_id = EXCLUDED.project_id so
// a worker_id colliding with a different project (e.g. across replicas
// where the in-memory cross-project rejection only covers the local
// process) cannot silently overwrite the original project's queue,
// hostname, version, or status fields. On project mismatch the upsert is
// a silent no-op; callers should detect and reject the conflict at the
// stream layer via GetWorkerByIDAcrossProjects.
func (q *Queries) RegisterWorker(ctx context.Context, w *domain.Worker) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RegisterWorker")
	defer span.End()

	query := `
		INSERT INTO workers (id, project_id, queue_name, hostname, version, status, last_seen_at, registered_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE
		SET queue_name   = EXCLUDED.queue_name,
		    hostname     = EXCLUDED.hostname,
		    version      = EXCLUDED.version,
		    status       = EXCLUDED.status,
		    last_seen_at = NOW()
		WHERE workers.project_id = EXCLUDED.project_id`

	_, err := q.db.Exec(ctx, query,
		w.ID, w.ProjectID, w.QueueName, w.Hostname, w.Version, string(w.Status),
	)
	if err != nil {
		return fmt.Errorf("register worker: %w", err)
	}
	return nil
}

// GetWorkerProjectByID returns the project_id of an existing workers row by
// its primary key, or "" with (false, nil) if no row exists. Used by the
// gRPC stream layer to reject cross-project worker_id collisions before
// any DB write or in-memory registration.
func (q *Queries) GetWorkerProjectByID(ctx context.Context, workerID string) (string, bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkerProjectByID")
	defer span.End()

	var projectID string
	err := q.db.QueryRow(ctx,
		`SELECT project_id FROM workers WHERE id = $1`,
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

// SetWorkerStatus transitions a worker to a new status.
func (q *Queries) SetWorkerStatus(ctx context.Context, workerID string, status domain.WorkerStatus) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SetWorkerStatus")
	defer span.End()

	_, err := q.db.Exec(ctx,
		`UPDATE workers SET status = $1, last_seen_at = NOW() WHERE id = $2`,
		string(status), workerID,
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
	ctx, span := otel.Tracer("strait").Start(ctx, "store.EvictStaleWorkers")
	defer span.End()

	tag, err := q.db.Exec(ctx,
		`UPDATE workers SET status = 'offline' WHERE last_seen_at < $1 AND status != 'offline'`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("evict stale workers: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CreateWorkerTask records a task assignment.
func (q *Queries) CreateWorkerTask(ctx context.Context, t *domain.WorkerTask) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkerTask")
	defer span.End()

	_, err := q.db.Exec(ctx,
		`INSERT INTO worker_tasks (id, worker_id, run_id, project_id, status, assigned_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())`,
		t.ID, t.WorkerID, t.RunID, t.ProjectID, string(t.Status),
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
		query = `UPDATE worker_tasks SET status = $1, accepted_at = NOW() WHERE id = $2`
	case domain.WorkerTaskStatusResultReceived:
		query = `UPDATE worker_tasks SET status = $1 WHERE id = $2`
	case domain.WorkerTaskStatusCompleted, domain.WorkerTaskStatusFailed:
		query = `UPDATE worker_tasks SET status = $1, finished_at = NOW() WHERE id = $2`
	default:
		query = `UPDATE worker_tasks SET status = $1 WHERE id = $2`
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

	tag, err := q.db.Exec(ctx,
		`UPDATE worker_tasks
		 SET status = $1
		 WHERE id = $2
		   AND status IN ('assigned', 'accepted', 'result_received')`,
		string(domain.WorkerTaskStatusResultReceived), taskID,
	)
	if err != nil {
		return false, fmt.Errorf("mark worker task result received: %w", err)
	}
	return tag.RowsAffected() == 1, nil
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
	err := q.db.QueryRow(ctx,
		`SELECT id, worker_id, run_id, project_id, status, assigned_at, accepted_at, finished_at
		 FROM worker_tasks WHERE id = $1`,
		taskID,
	).Scan(&t.ID, &t.WorkerID, &t.RunID, &t.ProjectID, &status, &t.AssignedAt, &t.AcceptedAt, &t.FinishedAt)
	if err != nil {
		return nil, fmt.Errorf("get worker task: %w", err)
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
		`SELECT id, worker_id, run_id, project_id, status, assigned_at, accepted_at, finished_at
		 FROM worker_tasks
		 WHERE worker_id = $1
		   AND project_id = $2
		   AND run_id = $3
		   AND status IN ('assigned', 'accepted')
		 ORDER BY assigned_at DESC
		 LIMIT 1`,
		workerID, projectID, runID,
	).Scan(&t.ID, &t.WorkerID, &t.RunID, &t.ProjectID, &status, &t.AssignedAt, &t.AcceptedAt, &t.FinishedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil //nolint:nilnil // nil signals "not found"
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

	query := `SELECT id, worker_id, run_id, project_id, status, assigned_at, accepted_at, finished_at
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
		if err := rows.Scan(&t.ID, &t.WorkerID, &t.RunID, &t.ProjectID, &s, &t.AssignedAt, &t.AcceptedAt, &t.FinishedAt); err != nil {
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

	txb, ok := q.db.(TxBeginner)
	if !ok {
		return 0, fmt.Errorf("requeue open worker tasks requires transaction support")
	}

	var affected int64
	err := WithTx(ctx, txb, func(txQ *Queries) error {
		const query = `
			WITH open_tasks AS (
				SELECT id, run_id
				FROM worker_tasks
				WHERE worker_id = $1
				  AND project_id = $2
				  AND status IN ('assigned', 'accepted')
			),
			requeued_runs AS (
				UPDATE job_runs
				SET status = 'queued',
				    started_at = NULL,
				    finished_at = NULL,
				    heartbeat_at = NULL,
				    next_retry_at = NULL,
				    error = $3,
				    error_class = 'transient'
				WHERE id IN (SELECT run_id FROM open_tasks)
				  AND status = 'executing'
				RETURNING id
			)
			UPDATE worker_tasks
			SET status = 'failed',
			    finished_at = NOW()
			WHERE id IN (SELECT id FROM open_tasks)`

		tag, err := txQ.db.Exec(ctx, query, workerID, projectID, reason)
		if err != nil {
			return fmt.Errorf("requeue open worker tasks: %w", err)
		}
		affected = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, err
	}

	return affected, nil
}
