package store

import (
	"context"
	"fmt"
	"time"

	"strait/internal/domain"

	"go.opentelemetry.io/otel"
)

// costHistoryLimit is the number of recent completed runs used to compute the
// rolling average in GetJobCostEstimate.
const costHistoryLimit = 30

// flatRateCostMicrousd is the fallback per-run cost when no ClickHouse history
// exists for a job. Mirrors billing.WorkerCostPerRunMicrousd (20 micro-USD =
// $0.00002/run). Keep in sync with internal/billing/plans.go.
const flatRateCostMicrousd int64 = 20

// GetJobCostEstimate returns the rolling average compute cost for the given
// job derived from the last 30 completed runs stored in ClickHouse
// run_analytics. When ClickHouse is unavailable or no history exists for the
// job, the function falls back to the plan-tier flat rate
// (flatRateCostMicrousd, currently 20 micro-USD).
func (q *Queries) GetJobCostEstimate(ctx context.Context, jobID string) (*domain.JobCostEstimate, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobCostEstimate")
	defer span.End()

	if q.chDB != nil {
		est, err := q.jobCostEstimateFromClickHouse(ctx, jobID)
		if err != nil {
			// ClickHouse is optional; fall through to the flat-rate fallback,
			// but record the failure on the span so an outage degrading every
			// estimate to the flat rate is visible in traces rather than silent.
			span.RecordError(err)
			span.AddEvent("clickhouse cost estimate unavailable; using flat-rate fallback")
		} else if est != nil {
			return est, nil
		}
	}

	// Flat-rate fallback: no ClickHouse or no history for this job.
	return &domain.JobCostEstimate{
		JobID:           jobID,
		AvgCostMicrousd: flatRateCostMicrousd,
		SampleCount:     0,
		UpdatedAt:       time.Now(),
	}, nil
}

// jobCostEstimateFromClickHouse queries run_analytics for the rolling average
// compute_cost_microusd over the last costHistoryLimit completed runs of the
// job. Returns nil (no error) when the job has no history yet.
func (q *Queries) jobCostEstimateFromClickHouse(ctx context.Context, jobID string) (*domain.JobCostEstimate, error) {
	// The LIMIT is embedded so the average is over the most-recent
	// 30 rows rather than a window across all history.
	const chQuery = `
		SELECT avg(compute_cost_microusd), count()
		FROM (
			SELECT compute_cost_microusd
			FROM run_analytics
			WHERE job_id = ?
			  AND status = 'completed'
			ORDER BY finished_at DESC
			LIMIT ?
		)`

	row := q.chDB.QueryRowContext(ctx, chQuery, jobID, costHistoryLimit)

	var avgCost float64
	var sampleCount int
	if err := row.Scan(&avgCost, &sampleCount); err != nil {
		return nil, fmt.Errorf("clickhouse job cost estimate scan: %w", err)
	}
	if sampleCount == 0 {
		return nil, nil
	}

	return &domain.JobCostEstimate{
		JobID:           jobID,
		AvgCostMicrousd: int64(avgCost),
		SampleCount:     sampleCount,
		UpdatedAt:       time.Now(),
	}, nil
}
