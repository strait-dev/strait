package store

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

type PerformanceAnalytics struct {
	SlowestJobs   []JobPerformance `json:"slowest_jobs"`
	Throughput    ThroughputStats  `json:"throughput"`
	HealthSummary HealthSummary    `json:"health_summary"`
}

type JobPerformance struct {
	JobID           string  `json:"job_id"`
	JobSlug         string  `json:"job_slug"`
	AvgDurationSecs float64 `json:"avg_duration_secs"`
	P95DurationSecs float64 `json:"p95_duration_secs"`
	TotalRuns       int     `json:"total_runs"`
	FailedRuns      int     `json:"failed_runs"`
}

type ThroughputStats struct {
	Completed   int `json:"completed"`
	Failed      int `json:"failed"`
	TimedOut    int `json:"timed_out"`
	Canceled    int `json:"canceled"`
	PeriodHours int `json:"period_hours"`
}

type HealthSummary struct {
	TotalJobs       int     `json:"total_jobs"`
	ActiveJobs      int     `json:"active_jobs"`
	SuccessRate     float64 `json:"success_rate"`
	AvgDurationSecs float64 `json:"avg_duration_secs"`
	QueueDepth      int     `json:"queue_depth"`
}

func (q *Queries) GetPerformanceAnalytics(ctx context.Context, projectID string, periodHours int) (*PerformanceAnalytics, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetPerformanceAnalytics")
	defer span.End()

	since := time.Now().Add(-time.Duration(periodHours) * time.Hour)
	result := &PerformanceAnalytics{
		SlowestJobs: make([]JobPerformance, 0, 10),
		Throughput:  ThroughputStats{PeriodHours: periodHours},
	}

	slowestQuery := `
		SELECT j.id, j.slug,
			COALESCE(AVG(EXTRACT(EPOCH FROM (r.finished_at - r.started_at))), 0) as avg_duration,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (r.finished_at - r.started_at))), 0) as p95_duration,
			COUNT(*) as total_runs,
			COUNT(*) FILTER (WHERE r.status = 'failed') as failed_runs
		FROM job_runs r
		JOIN jobs j ON j.id = r.job_id
		WHERE j.project_id = $1
			AND r.finished_at IS NOT NULL
			AND r.started_at IS NOT NULL
			AND r.created_at >= $2
		GROUP BY j.id, j.slug
		HAVING COUNT(*) >= 5
		ORDER BY avg_duration DESC
		LIMIT 10`

	rows, err := q.db.Query(ctx, slowestQuery, projectID, since)
	if err != nil {
		return nil, fmt.Errorf("analytics slowest jobs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var jp JobPerformance
		if err := rows.Scan(&jp.JobID, &jp.JobSlug, &jp.AvgDurationSecs, &jp.P95DurationSecs, &jp.TotalRuns, &jp.FailedRuns); err != nil {
			return nil, fmt.Errorf("analytics slowest jobs scan: %w", err)
		}
		result.SlowestJobs = append(result.SlowestJobs, jp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("analytics slowest jobs rows: %w", err)
	}

	throughputQuery := `
		SELECT
			COUNT(*) FILTER (WHERE status = 'completed') as completed,
			COUNT(*) FILTER (WHERE status = 'failed') as failed,
			COUNT(*) FILTER (WHERE status = 'timed_out') as timed_out,
			COUNT(*) FILTER (WHERE status = 'canceled') as canceled
		FROM job_runs
		WHERE project_id = $1 AND created_at >= $2`

	err = q.db.QueryRow(ctx, throughputQuery, projectID, since).Scan(
		&result.Throughput.Completed,
		&result.Throughput.Failed,
		&result.Throughput.TimedOut,
		&result.Throughput.Canceled,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics throughput: %w", err)
	}

	healthQuery := `
		SELECT
			(SELECT COUNT(*) FROM jobs WHERE project_id = $1) as total_jobs,
			(SELECT COUNT(*) FROM jobs WHERE project_id = $1 AND enabled = true) as active_jobs,
			CASE WHEN COUNT(*) > 0
				THEN ROUND(COUNT(*) FILTER (WHERE status = 'completed')::numeric / COUNT(*)::numeric, 4)
				ELSE 0
			END as success_rate,
			COALESCE(AVG(EXTRACT(EPOCH FROM (finished_at - started_at))) FILTER (WHERE finished_at IS NOT NULL AND started_at IS NOT NULL), 0) as avg_duration,
			(SELECT COUNT(*) FROM job_runs WHERE project_id = $1 AND status = 'queued') as queue_depth
		FROM job_runs
		WHERE project_id = $1 AND created_at >= $2`

	err = q.db.QueryRow(ctx, healthQuery, projectID, since).Scan(
		&result.HealthSummary.TotalJobs,
		&result.HealthSummary.ActiveJobs,
		&result.HealthSummary.SuccessRate,
		&result.HealthSummary.AvgDurationSecs,
		&result.HealthSummary.QueueDepth,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics health: %w", err)
	}

	return result, nil
}
