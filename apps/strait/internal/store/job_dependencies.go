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
		WITH inserted AS (
			INSERT INTO job_dependencies (id, job_id, depends_on_job_id, condition)
			VALUES ($1, $2, $3, $4)
			RETURNING created_at
		),
		bumped AS (
			UPDATE jobs
			SET cache_version = cache_version + 1
			FROM inserted
			WHERE jobs.id = $2
			RETURNING jobs.cache_version
		)
		SELECT inserted.created_at, bumped.cache_version
		FROM inserted, bumped`

	if dep.Condition == "" {
		dep.Condition = "completed"
	}

	err := q.db.QueryRow(ctx, query, dep.ID, dep.JobID, dep.DependsOnJobID, dep.Condition).Scan(&dep.CreatedAt, &dep.CacheVersion)
	if err != nil {
		return fmt.Errorf("create job dependency: %w", err)
	}

	return nil
}

func (q *Queries) ListJobDependencies(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobDependency, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobDependencies")
	defer span.End()

	query := `
		SELECT jd.id, jd.job_id, jd.depends_on_job_id, jd.condition, jd.created_at, GREATEST(jd.cache_version, j.cache_version)
		FROM job_dependencies jd
		JOIN jobs j ON j.id = jd.job_id
		WHERE jd.job_id = $1`

	args := []any{jobID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND jd.created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY jd.created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list job dependencies: %w", err)
	}
	defer rows.Close()

	deps := make([]domain.JobDependency, 0, limit)
	for rows.Next() {
		var dep domain.JobDependency
		if err := rows.Scan(&dep.ID, &dep.JobID, &dep.DependsOnJobID, &dep.Condition, &dep.CreatedAt, &dep.CacheVersion); err != nil {
			return nil, fmt.Errorf("list job dependencies scan: %w", err)
		}
		deps = append(deps, dep)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list job dependencies rows: %w", err)
	}

	return deps, nil
}

var ErrJobDependencyNotFound = errors.New("job dependency not found")

func (q *Queries) GetJobDependency(ctx context.Context, id string) (*domain.JobDependency, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobDependency")
	defer span.End()

	var dep domain.JobDependency
	err := q.db.QueryRow(ctx, `
		SELECT id, job_id, depends_on_job_id, condition, created_at, cache_version
		FROM job_dependencies WHERE id = $1`, id).Scan(
		&dep.ID, &dep.JobID, &dep.DependsOnJobID, &dep.Condition, &dep.CreatedAt, &dep.CacheVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrJobDependencyNotFound
		}
		return nil, fmt.Errorf("get job dependency: %w", err)
	}
	return &dep, nil
}

func (q *Queries) DeleteJobDependency(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteJobDependency")
	defer span.End()

	query := `
		WITH deleted AS (
			DELETE FROM job_dependencies
			WHERE id = $1
			RETURNING job_id
		)
		UPDATE jobs
		SET cache_version = cache_version + 1
		FROM deleted
		WHERE jobs.id = deleted.job_id`
	if _, err := q.db.Exec(ctx, query, id); err != nil {
		return fmt.Errorf("delete job dependency: %w", err)
	}

	return nil
}

func (q *Queries) GetJobDependencyListVersion(ctx context.Context, jobID string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobDependencyListVersion")
	defer span.End()

	var version int64
	err := q.db.QueryRow(ctx, `SELECT cache_version FROM jobs WHERE id = $1`, jobID).Scan(&version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrJobNotFound
		}
		return 0, fmt.Errorf("get job dependency list version: %w", err)
	}
	return version, nil
}

func (q *Queries) ListDependentsByDependencyJob(ctx context.Context, dependsOnJobID string) ([]domain.JobDependency, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListDependentsByDependencyJob")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		SELECT id, job_id, depends_on_job_id, condition, created_at, cache_version
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
		if scanErr := rows.Scan(&dep.ID, &dep.JobID, &dep.DependsOnJobID, &dep.Condition, &dep.CreatedAt, &dep.CacheVersion); scanErr != nil {
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
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at, next_retry_at,
		       expires_at, parent_run_id, priority, idempotency_key, job_version, created_at,
		       workflow_step_run_id, execution_trace,
		       debug_mode, continuation_of, lineage_depth, tags,
		       job_version_id, created_by, batch_id, concurrency_key, execution_mode, is_rollback, replayed_run_id
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

	dependencyKey := ""
	if run.Metadata != nil {
		dependencyKey = run.Metadata["dependency_key"]
	}

	// Resolve every dependency together with the status of its latest matching
	// terminal run in a single round trip. The LATERAL join picks the most recent
	// terminal run per dependency target (scoped to the run's idempotency or
	// dependency key when present), so the per-dependency condition check below
	// needs no further query. This replaces a 1+N pattern (one ListJobDependencies
	// plus one run lookup per dependency) that ran on every trigger and, in the
	// dependency-release loop, once per waiting run. The common idempotency-keyed
	// lookup is served by idx_runs_idempotency (job_id, idempotency_key); the
	// terminal-status predicate is intentionally not backed by a dedicated partial
	// index to avoid per-transition write amplification on the partitioned
	// job_runs table (see migration 000255).
	latestFilter := ""
	args := []any{run.JobID}
	if run.IdempotencyKey != "" {
		latestFilter = " AND r.idempotency_key = $2"
		args = append(args, run.IdempotencyKey)
	} else if dependencyKey != "" {
		latestFilter = " AND r.metadata->>'dependency_key' = $2"
		args = append(args, dependencyKey)
	}

	query := `
		SELECT d.condition, latest.status
		FROM job_dependencies d
		LEFT JOIN LATERAL (
			SELECT r.status
			FROM job_runs r
			WHERE r.job_id = d.depends_on_job_id
			  AND r.status IN ('completed', 'failed', 'timed_out', 'crashed', 'system_failed', 'canceled', 'expired', 'dead_letter')` + latestFilter + `
			ORDER BY r.finished_at DESC NULLS LAST, r.created_at DESC
			LIMIT 1
		) latest ON true
		WHERE d.job_id = $1`

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("evaluate job dependencies: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var condition string
		var status *string
		if scanErr := rows.Scan(&condition, &status); scanErr != nil {
			return false, fmt.Errorf("evaluate job dependencies scan: %w", scanErr)
		}
		if status == nil {
			// No matching terminal run for this dependency yet.
			return false, nil
		}

		matched := domain.RunStatus(*status)
		switch condition {
		case "completed":
			if matched != domain.StatusCompleted {
				return false, nil
			}
		case "failed":
			if !isFailureTerminalStatus(matched) {
				return false, nil
			}
		case "any":
			if !matched.IsTerminal() {
				return false, nil
			}
		default:
			return false, fmt.Errorf("unknown dependency condition %q", condition)
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("evaluate job dependencies rows: %w", err)
	}

	return true, nil
}

func isFailureTerminalStatus(status domain.RunStatus) bool {
	switch status {
	case domain.StatusFailed, domain.StatusTimedOut, domain.StatusCrashed, domain.StatusSystemFailed, domain.StatusExpired, domain.StatusDeadLetter:
		return true
	default:
		return false
	}
}
