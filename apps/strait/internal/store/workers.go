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
		    last_seen_at = NOW()`

	_, err := q.db.Exec(ctx, query,
		w.ID, w.ProjectID, w.QueueName, w.Hostname, w.Version, string(w.Status),
	)
	if err != nil {
		return fmt.Errorf("register worker: %w", err)
	}
	return nil
}

// HeartbeatWorker updates last_seen_at for a worker.
func (q *Queries) HeartbeatWorker(ctx context.Context, workerID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.HeartbeatWorker")
	defer span.End()

	_, err := q.db.Exec(ctx,
		`UPDATE workers SET last_seen_at = NOW() WHERE id = $1`,
		workerID,
	)
	if err != nil {
		return fmt.Errorf("heartbeat worker: %w", err)
	}
	return nil
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
func (q *Queries) ListWorkers(ctx context.Context, projectID, queueName string, limit int) ([]domain.Worker, error) {
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

	query += fmt.Sprintf(" ORDER BY registered_at DESC LIMIT $%d", param)
	args = append(args, limit)

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

// GetWorkerTaskByRunID fetches the worker_tasks row for a specific run, scoped
// to the given workerID to prevent cross-worker ownership forgeries.
// Returns (nil, nil) when no row matches, which callers treat as "not owned".
func (q *Queries) GetWorkerTaskByRunID(ctx context.Context, workerID, runID string) (*domain.WorkerTask, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkerTaskByRunID")
	defer span.End()

	var t domain.WorkerTask
	var status string
	err := q.db.QueryRow(ctx,
		`SELECT id, worker_id, run_id, project_id, status, assigned_at, accepted_at, finished_at
		 FROM worker_tasks WHERE worker_id = $1 AND run_id = $2`,
		workerID, runID,
	).Scan(&t.ID, &t.WorkerID, &t.RunID, &t.ProjectID, &status, &t.AssignedAt, &t.AcceptedAt, &t.FinishedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil //nolint:nilnil // nil signals "not found"
		}
		return nil, fmt.Errorf("get worker task by run id: %w", err)
	}
	t.Status = domain.WorkerTaskStatus(status)
	return &t, nil
}

// ListWorkerTasksByWorker lists tasks assigned to a worker, optionally filtered by status.
func (q *Queries) ListWorkerTasksByWorker(ctx context.Context, workerID string, status domain.WorkerTaskStatus, limit int) ([]domain.WorkerTask, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkerTasksByWorker")
	defer span.End()

	query := `SELECT id, worker_id, run_id, project_id, status, assigned_at, accepted_at, finished_at
	          FROM worker_tasks WHERE worker_id = $1`
	args := []any{workerID}
	param := 2

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", param)
		args = append(args, string(status))
		param++
	}

	query += fmt.Sprintf(" ORDER BY assigned_at DESC LIMIT $%d", param)
	args = append(args, limit)

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
