package store

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateJobSLO(ctx context.Context, slo *domain.JobSLO) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateJobSLO")
	defer span.End()

	const sql = `
		INSERT INTO job_slos (id, job_id, project_id, metric, target, window_hours)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at
	`

	return q.db.QueryRow(ctx, sql,
		slo.ID, slo.JobID, slo.ProjectID, slo.Metric, slo.Target, slo.WindowHours,
	).Scan(&slo.CreatedAt)
}

func (q *Queries) ListJobSLOs(ctx context.Context, jobID string) ([]domain.JobSLOStatus, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobSLOs")
	defer span.End()

	const sql = `
		SELECT s.id, s.job_id, s.project_id, s.metric, s.target, s.window_hours, s.created_at,
		       e.current_value, e.budget_remaining, e.evaluated_at
		FROM job_slos s
		LEFT JOIN LATERAL (
			SELECT current_value, budget_remaining, evaluated_at
			FROM job_slo_evaluations
			WHERE slo_id = s.id
			ORDER BY evaluated_at DESC
			LIMIT 1
		) e ON true
		WHERE s.job_id = $1
		ORDER BY s.metric, s.window_hours
	`

	rows, err := q.db.Query(ctx, sql, jobID)
	if err != nil {
		return nil, fmt.Errorf("list job slos: %w", err)
	}
	defer rows.Close()

	var results []domain.JobSLOStatus
	for rows.Next() {
		var s domain.JobSLOStatus
		if err := rows.Scan(
			&s.ID, &s.JobID, &s.ProjectID, &s.Metric, &s.Target, &s.WindowHours, &s.CreatedAt,
			&s.CurrentValue, &s.BudgetRemaining, &s.EvaluatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan job slo: %w", err)
		}
		results = append(results, s)
	}

	return results, rows.Err()
}

func (q *Queries) DeleteJobSLO(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteJobSLO")
	defer span.End()

	tag, err := q.db.Exec(ctx, `DELETE FROM job_slos WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete job slo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("slo not found: %w", pgx.ErrNoRows)
	}
	return nil
}

func (q *Queries) GetJobSLO(ctx context.Context, id string) (*domain.JobSLO, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobSLO")
	defer span.End()

	const sql = `
		SELECT id, job_id, project_id, metric, target, window_hours, created_at
		FROM job_slos WHERE id = $1
	`

	var s domain.JobSLO
	err := q.db.QueryRow(ctx, sql, id).Scan(
		&s.ID, &s.JobID, &s.ProjectID, &s.Metric, &s.Target, &s.WindowHours, &s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get job slo: %w", err)
	}
	return &s, nil
}

func (q *Queries) ListAllJobSLOs(ctx context.Context) ([]domain.JobSLO, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAllJobSLOs")
	defer span.End()

	rows, err := q.db.Query(ctx, `SELECT id, job_id, project_id, metric, target, window_hours, created_at FROM job_slos`)
	if err != nil {
		return nil, fmt.Errorf("list all job slos: %w", err)
	}
	defer rows.Close()

	var results []domain.JobSLO
	for rows.Next() {
		var s domain.JobSLO
		if err := rows.Scan(&s.ID, &s.JobID, &s.ProjectID, &s.Metric, &s.Target, &s.WindowHours, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan job slo: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

func (q *Queries) InsertSLOEvaluation(ctx context.Context, eval *domain.JobSLOEvaluation) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.InsertSLOEvaluation")
	defer span.End()

	const sql = `
		INSERT INTO job_slo_evaluations (id, slo_id, current_value, budget_remaining, evaluated_at)
		VALUES ($1, $2, $3, $4, $5)
	`

	if _, err := q.db.Exec(ctx, sql,
		eval.ID, eval.SLOID, eval.CurrentValue, eval.BudgetRemaining, eval.EvaluatedAt,
	); err != nil {
		return fmt.Errorf("insert slo evaluation: %w", err)
	}
	return nil
}

// PruneSLOEvaluations removes old evaluations, keeping the latest N per SLO.
func (q *Queries) PruneSLOEvaluations(ctx context.Context, keepPerSLO int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.PruneSLOEvaluations")
	defer span.End()

	const sql = `
		DELETE FROM job_slo_evaluations
		WHERE id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY slo_id ORDER BY evaluated_at DESC) AS rn
				FROM job_slo_evaluations
			) ranked WHERE rn > $1
		)
	`

	tag, err := q.db.Exec(ctx, sql, keepPerSLO)
	if err != nil {
		return 0, fmt.Errorf("prune slo evaluations: %w", err)
	}
	return tag.RowsAffected(), nil
}
