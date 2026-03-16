package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateJobDependency(ctx context.Context, dep *domain.JobDependency) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateJobDependency")
	defer span.End()

	if dep.JobID == dep.DependsOnJobID {
		return fmt.Errorf("create job dependency: job cannot depend on itself")
	}

	if dep.ID == "" {
		dep.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO job_dependencies (id, job_id, depends_on_job_id, condition)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at`

	if dep.Condition == "" {
		dep.Condition = "completed"
	}

	err := q.db.QueryRow(ctx, query, dep.ID, dep.JobID, dep.DependsOnJobID, dep.Condition).Scan(&dep.CreatedAt)
	if err != nil {
		return fmt.Errorf("create job dependency: %w", err)
	}

	return nil
}

func (q *Queries) ListJobDependencies(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobDependency, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobDependencies")
	defer span.End()

	query := `
		SELECT id, job_id, depends_on_job_id, condition, created_at
		FROM job_dependencies
		WHERE job_id = $1`

	args := []any{jobID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list job dependencies: %w", err)
	}
	defer rows.Close()

	deps := make([]domain.JobDependency, 0, limit)
	for rows.Next() {
		var dep domain.JobDependency
		if err := rows.Scan(&dep.ID, &dep.JobID, &dep.DependsOnJobID, &dep.Condition, &dep.CreatedAt); err != nil {
			return nil, fmt.Errorf("list job dependencies scan: %w", err)
		}
		deps = append(deps, dep)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list job dependencies rows: %w", err)
	}

	return deps, nil
}

func (q *Queries) DeleteJobDependency(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteJobDependency")
	defer span.End()

	query := `DELETE FROM job_dependencies WHERE id = $1`
	if _, err := q.db.Exec(ctx, query, id); err != nil {
		return fmt.Errorf("delete job dependency: %w", err)
	}

	return nil
}

func (q *Queries) ListDependentsByDependencyJob(ctx context.Context, dependsOnJobID string) ([]domain.JobDependency, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListDependentsByDependencyJob")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		SELECT id, job_id, depends_on_job_id, condition, created_at
		FROM job_dependencies
		WHERE depends_on_job_id = $1
		ORDER BY created_at DESC`, dependsOnJobID)
	if err != nil {
		return nil, fmt.Errorf("list dependents by dependency job: %w", err)
	}
	defer rows.Close()

	deps := make([]domain.JobDependency, 0, 8)
	for rows.Next() {
		var dep domain.JobDependency
		if scanErr := rows.Scan(&dep.ID, &dep.JobID, &dep.DependsOnJobID, &dep.Condition, &dep.CreatedAt); scanErr != nil {
			return nil, fmt.Errorf("list dependents by dependency job scan: %w", scanErr)
		}
		deps = append(deps, dep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list dependents by dependency job rows: %w", err)
	}

	return deps, nil
}

func (q *Queries) ListWaitingRunsByJobIDs(ctx context.Context, jobIDs []string, limit int) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWaitingRunsByJobIDs")
	defer span.End()

	if len(jobIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 1000
	}

	rows, err := q.db.Query(ctx, `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at, next_retry_at,
		       expires_at, parent_run_id, priority, idempotency_key, job_version, created_at,
		       workflow_step_run_id, execution_trace,
		       debug_mode, continuation_of, lineage_depth, tags,
		       job_version_id, created_by, batch_id, concurrency_key, execution_mode
		FROM job_runs
		WHERE status = 'waiting' AND job_id = ANY($1)
		ORDER BY created_at ASC
		LIMIT $2`, jobIDs, limit)
	if err != nil {
		return nil, fmt.Errorf("list waiting runs by job ids: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, limit)
	for rows.Next() {
		run, scanErr := dbscan.ScanRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list waiting runs by job ids scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list waiting runs by job ids rows: %w", err)
	}
	return runs, nil
}

func (q *Queries) AreJobDependenciesSatisfied(ctx context.Context, run *domain.JobRun) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.AreJobDependenciesSatisfied")
	defer span.End()

	deps, err := q.ListJobDependencies(ctx, run.JobID, 1000, nil)
	if err != nil {
		return false, fmt.Errorf("list job dependencies: %w", err)
	}
	if len(deps) == 0 {
		return true, nil
	}

	dependencyKey := ""
	if run.Metadata != nil {
		dependencyKey = run.Metadata["dependency_key"]
	}

	for _, dep := range deps {
		matchedRun, matchErr := q.findLatestTerminalDependencyRun(ctx, dep.DependsOnJobID, run.IdempotencyKey, dependencyKey)
		if matchErr != nil {
			return false, fmt.Errorf("find latest terminal dependency run for %s: %w", dep.DependsOnJobID, matchErr)
		}
		if matchedRun == nil {
			return false, nil
		}

		switch dep.Condition {
		case "completed":
			if matchedRun.Status != domain.StatusCompleted {
				return false, nil
			}
		case "failed":
			if !isFailureTerminalStatus(matchedRun.Status) {
				return false, nil
			}
		case "any":
			if !matchedRun.Status.IsTerminal() {
				return false, nil
			}
		default:
			return false, fmt.Errorf("unknown dependency condition %q", dep.Condition)
		}
	}

	return true, nil
}

func (q *Queries) findLatestTerminalDependencyRun(ctx context.Context, jobID, idempotencyKey, dependencyKey string) (*domain.JobRun, error) {
	baseQuery := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at, next_retry_at,
		       expires_at, parent_run_id, priority, idempotency_key, job_version, created_at,
		       workflow_step_run_id, execution_trace,
		       debug_mode, continuation_of, lineage_depth, tags,
		       job_version_id, created_by, batch_id, concurrency_key, execution_mode
		FROM job_runs
		WHERE job_id = $1
		  AND status IN ('completed', 'failed', 'timed_out', 'crashed', 'system_failed', 'canceled', 'expired', 'dead_letter')`

	args := []any{jobID}
	param := 2
	if idempotencyKey != "" {
		baseQuery += fmt.Sprintf(" AND idempotency_key = $%d", param)
		args = append(args, idempotencyKey)
	} else if dependencyKey != "" {
		baseQuery += fmt.Sprintf(" AND metadata->>'dependency_key' = $%d", param)
		args = append(args, dependencyKey)
	}

	baseQuery += " ORDER BY finished_at DESC NULLS LAST, created_at DESC LIMIT 1"

	run, err := dbscan.ScanRun(q.db.QueryRow(ctx, baseQuery, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return run, nil
}

func isFailureTerminalStatus(status domain.RunStatus) bool {
	switch status {
	case domain.StatusFailed, domain.StatusTimedOut, domain.StatusCrashed, domain.StatusSystemFailed, domain.StatusExpired, domain.StatusDeadLetter:
		return true
	default:
		return false
	}
}
