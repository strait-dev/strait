package store

import (
	"context"
	"fmt"

	"orchestrator/internal/domain"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateJobDependency(ctx context.Context, dep *domain.JobDependency) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.CreateJobDependency")
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

func (q *Queries) ListJobDependencies(ctx context.Context, jobID string) ([]domain.JobDependency, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListJobDependencies")
	defer span.End()

	query := `
		SELECT id, job_id, depends_on_job_id, condition, created_at
		FROM job_dependencies
		WHERE job_id = $1
		ORDER BY created_at DESC`

	rows, err := q.db.Query(ctx, query, jobID)
	if err != nil {
		return nil, fmt.Errorf("list job dependencies: %w", err)
	}
	defer rows.Close()

	deps := make([]domain.JobDependency, 0)
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
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.DeleteJobDependency")
	defer span.End()

	query := `DELETE FROM job_dependencies WHERE id = $1`
	if _, err := q.db.Exec(ctx, query, id); err != nil {
		return fmt.Errorf("delete job dependency: %w", err)
	}

	return nil
}
