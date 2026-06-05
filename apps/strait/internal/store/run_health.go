package store

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

// GetJobHealthStats returns aggregated health metrics for a job's runs over a given window.
// Queries hot table only; archived runs are not included.
func (q *Queries) GetJobHealthStats(ctx context.Context, jobID string, since time.Time) (*JobHealthStats, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobHealthStats")
	defer span.End()

	query := `
		SELECT
			COUNT(*) AS total_runs,
			COUNT(*) FILTER (WHERE COALESCE(s.status, jr.status) = 'completed') AS completed_runs,
			COUNT(*) FILTER (WHERE COALESCE(s.status, jr.status) = 'failed') AS failed_runs,
			COUNT(*) FILTER (WHERE COALESCE(s.status, jr.status) = 'timed_out') AS timed_out_runs,
			COUNT(*) FILTER (WHERE COALESCE(s.status, jr.status) IN ('crashed', 'system_failed')) AS crashed_runs,
			COUNT(*) FILTER (WHERE COALESCE(s.status, jr.status) = 'canceled') AS canceled_runs,
			COUNT(*) FILTER (WHERE COALESCE(s.status, jr.status) = 'expired') AS expired_runs,
			COALESCE(
				AVG(EXTRACT(EPOCH FROM (COALESCE(s.finished_at, jr.finished_at) - COALESCE(s.started_at, jr.started_at)))) FILTER (WHERE COALESCE(s.finished_at, jr.finished_at) IS NOT NULL AND COALESCE(s.started_at, jr.started_at) IS NOT NULL),
				0
			) AS avg_duration_secs,
			COALESCE(
				PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (COALESCE(s.finished_at, jr.finished_at) - COALESCE(s.started_at, jr.started_at)))) FILTER (WHERE COALESCE(s.finished_at, jr.finished_at) IS NOT NULL AND COALESCE(s.started_at, jr.started_at) IS NOT NULL),
				0
			) AS p95_duration_secs,
			COALESCE(
				PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (COALESCE(s.finished_at, jr.finished_at) - COALESCE(s.started_at, jr.started_at)))) FILTER (WHERE COALESCE(s.finished_at, jr.finished_at) IS NOT NULL AND COALESCE(s.started_at, jr.started_at) IS NOT NULL),
				0
			) AS p99_duration_secs
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.job_id = $1
			AND jr.created_at >= $2
			AND COALESCE(s.status, jr.status) IN ('completed', 'failed', 'timed_out', 'crashed', 'system_failed', 'canceled', 'expired')`

	stats := &JobHealthStats{}
	err := q.db.QueryRow(ctx, query, jobID, since).Scan(
		&stats.TotalRuns,
		&stats.CompletedRuns,
		&stats.FailedRuns,
		&stats.TimedOutRuns,
		&stats.CrashedRuns,
		&stats.CanceledRuns,
		&stats.ExpiredRuns,
		&stats.AvgDurationSecs,
		&stats.P95DurationSecs,
		&stats.P99DurationSecs,
	)
	if err != nil {
		return nil, fmt.Errorf("get job health stats: %w", err)
	}

	completeJobHealthStats(stats)

	return stats, nil
}

// GetJobHealthCounts returns count-only health metrics without the percentile
// ordered-set aggregates used by GetJobHealthStats. Use it on hot paths that
// only need success-rate style decisions.
func (q *Queries) GetJobHealthCounts(ctx context.Context, jobID string, since time.Time) (*JobHealthStats, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobHealthCounts")
	defer span.End()

	query := `
		SELECT
			COUNT(*) AS total_runs,
			COUNT(*) FILTER (WHERE COALESCE(s.status, jr.status) = 'completed') AS completed_runs,
			COUNT(*) FILTER (WHERE COALESCE(s.status, jr.status) = 'failed') AS failed_runs,
			COUNT(*) FILTER (WHERE COALESCE(s.status, jr.status) = 'timed_out') AS timed_out_runs,
			COUNT(*) FILTER (WHERE COALESCE(s.status, jr.status) IN ('crashed', 'system_failed')) AS crashed_runs,
			COUNT(*) FILTER (WHERE COALESCE(s.status, jr.status) = 'canceled') AS canceled_runs,
			COUNT(*) FILTER (WHERE COALESCE(s.status, jr.status) = 'expired') AS expired_runs
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.job_id = $1
			AND jr.created_at >= $2
			AND COALESCE(s.status, jr.status) IN ('completed', 'failed', 'timed_out', 'crashed', 'system_failed', 'canceled', 'expired')`

	stats := &JobHealthStats{}
	err := q.db.QueryRow(ctx, query, jobID, since).Scan(
		&stats.TotalRuns,
		&stats.CompletedRuns,
		&stats.FailedRuns,
		&stats.TimedOutRuns,
		&stats.CrashedRuns,
		&stats.CanceledRuns,
		&stats.ExpiredRuns,
	)
	if err != nil {
		return nil, fmt.Errorf("get job health counts: %w", err)
	}

	completeJobHealthStats(stats)
	return stats, nil
}

func completeJobHealthStats(stats *JobHealthStats) {
	if stats.TotalRuns == 0 {
		stats.HealthScore = -1 // unknown
		return
	}

	stats.SuccessRate = float64(stats.CompletedRuns) / float64(stats.TotalRuns) * 100

	// Health score: 70% success rate + 30% duration stability (0-100).
	successComponent := stats.SuccessRate * 0.7
	stabilityComponent := 0.0 // default: no duration data = no stability credit
	if stats.AvgDurationSecs > 0 {
		stabilityComponent = 30.0 // full credit for stable durations
		if stats.P95DurationSecs > 2*stats.AvgDurationSecs {
			// Penalize high variance: ratio > 2 means unstable.
			ratio := stats.P95DurationSecs / stats.AvgDurationSecs
			penalty := min((ratio-2)*15, 30) // max 30 point penalty
			stabilityComponent = max(0, 30-penalty)
		}
	}
	stats.HealthScore = min(100, successComponent+stabilityComponent)
}
