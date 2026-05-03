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

