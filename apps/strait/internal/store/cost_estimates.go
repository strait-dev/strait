package store

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// GetJobCostEstimate returns the cached cost estimate for a job.
func (q *Queries) GetJobCostEstimate(ctx context.Context, jobID string) (*domain.JobCostEstimate, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobCostEstimate")
	defer span.End()

	query := `
		SELECT job_id, avg_cost_microusd, sample_count, updated_at
		FROM job_cost_estimates
		WHERE job_id = $1`

	var est domain.JobCostEstimate
	err := q.db.QueryRow(ctx, query, jobID).Scan(
		&est.JobID,
		&est.AvgCostMicrousd,
		&est.SampleCount,
		&est.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get job cost estimate: %w", err)
	}

	return &est, nil
}

// UpsertJobCostEstimate recomputes and upserts the average cost for a job
// based on completed runs from the last 30 days.
func (q *Queries) UpsertJobCostEstimate(ctx context.Context, jobID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertJobCostEstimate")
	defer span.End()

	query := `
		INSERT INTO job_cost_estimates (job_id, avg_cost_microusd, sample_count, updated_at)
		SELECT
			$1,
			COALESCE(AVG(ru.cost_microusd), 0)::BIGINT,
			COUNT(*)::INT,
			NOW()
		FROM run_usage ru
		JOIN job_runs jr ON jr.id = ru.run_id
		WHERE jr.job_id = $1
		  AND jr.status = 'completed'
		  AND jr.finished_at >= NOW() - INTERVAL '30 days'
		ON CONFLICT (job_id) DO UPDATE
		SET avg_cost_microusd = EXCLUDED.avg_cost_microusd,
		    sample_count      = EXCLUDED.sample_count,
		    updated_at        = EXCLUDED.updated_at`

	if _, err := q.db.Exec(ctx, query, jobID); err != nil {
		return fmt.Errorf("upsert job cost estimate: %w", err)
	}

	return nil
}

// ListActiveJobIDs returns the IDs of all enabled jobs.
func (q *Queries) ListActiveJobIDs(ctx context.Context) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListActiveJobIDs")
	defer span.End()

	query := `SELECT id FROM jobs WHERE enabled = true ORDER BY id`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list active job ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan active job id: %w", err)
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

var _ CostEstimateStore = (*Queries)(nil)
