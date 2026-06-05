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

// AggregateHourlyStats materializes run statistics for a given hour into job_stats_hourly.
func (q *Queries) AggregateHourlyStats(ctx context.Context, hour time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.AggregateHourlyStats")
	defer span.End()

	// Truncate to hour boundary.
	hour = hour.Truncate(time.Hour)
	nextHour := hour.Add(time.Hour)

	// Launch performance analytics do not read retired model usage cost. Runtime
	// billing totals come from billing cost events, and per-job compute-cost
	// attribution can be wired here when that source includes job IDs.
	query := `
		WITH target_runs AS (
			SELECT
				jr.id,
				jr.job_id,
				jr.project_id,
				COALESCE(s.status, jr.status) AS status,
				COALESCE(s.started_at, jr.started_at) AS started_at,
				COALESCE(s.finished_at, jr.finished_at) AS finished_at
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE jr.created_at >= $1 AND jr.created_at < $2
			  AND COALESCE(s.status, jr.status) IN ('completed', 'failed', 'timed_out', 'canceled')
		)
		INSERT INTO job_stats_hourly (job_id, project_id, hour, total, completed, failed, timed_out, canceled, avg_duration_ms, p95_duration_ms, total_cost_microusd)
		SELECT
			tr.job_id,
			tr.project_id,
			$1 AS hour,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE tr.status = 'completed') AS completed,
			COUNT(*) FILTER (WHERE tr.status = 'failed') AS failed,
			COUNT(*) FILTER (WHERE tr.status = 'timed_out') AS timed_out,
			COUNT(*) FILTER (WHERE tr.status = 'canceled') AS canceled,
			COALESCE(AVG(EXTRACT(EPOCH FROM (tr.finished_at - tr.started_at)) * 1000)::BIGINT, 0) AS avg_duration_ms,
			COALESCE((PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (tr.finished_at - tr.started_at)) * 1000))::BIGINT, 0) AS p95_duration_ms,
			0 AS total_cost_microusd
		FROM target_runs tr
		GROUP BY tr.job_id, tr.project_id
		ON CONFLICT (job_id, hour) DO UPDATE SET
			total = EXCLUDED.total,
			completed = EXCLUDED.completed,
			failed = EXCLUDED.failed,
			timed_out = EXCLUDED.timed_out,
			canceled = EXCLUDED.canceled,
			avg_duration_ms = EXCLUDED.avg_duration_ms,
			p95_duration_ms = EXCLUDED.p95_duration_ms,
			total_cost_microusd = EXCLUDED.total_cost_microusd`

	_, err := q.db.Exec(ctx, query, hour, nextHour)
	if err != nil {
		return fmt.Errorf("aggregate hourly stats: %w", err)
	}
	return nil
}

func (q *Queries) GetPerformanceAnalytics(ctx context.Context, projectID string, periodHours int) (*PerformanceAnalytics, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetPerformanceAnalytics")
	defer span.End()

	// Use materialized path for ranges > 24 hours.
	if periodHours > 24 {
		return q.getPerformanceAnalyticsMaterialized(ctx, projectID, periodHours)
	}

	since := time.Now().Add(-time.Duration(periodHours) * time.Hour)
	result := &PerformanceAnalytics{
		SlowestJobs: make([]JobPerformance, 0, 10),
		Throughput:  ThroughputStats{PeriodHours: periodHours},
	}

	slowestQuery := `
		WITH runs AS (
			SELECT
				r.job_id,
				r.created_at,
				COALESCE(s.status, r.status) AS status,
				COALESCE(s.started_at, r.started_at) AS started_at,
				COALESCE(s.finished_at, r.finished_at) AS finished_at
			FROM job_runs r
			LEFT JOIN job_run_read_state s ON s.run_id = r.id
			WHERE r.project_id = $1
			  AND r.created_at >= $2
		)
		SELECT j.id, j.slug,
			COALESCE(AVG(EXTRACT(EPOCH FROM (r.finished_at - r.started_at))), 0) as avg_duration,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (r.finished_at - r.started_at))), 0) as p95_duration,
			COUNT(*) as total_runs,
			COUNT(*) FILTER (WHERE r.status = 'failed') as failed_runs
		FROM runs r
		JOIN jobs j ON j.id = r.job_id
		WHERE r.finished_at IS NOT NULL
			AND r.started_at IS NOT NULL
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
		WITH runs AS (
			SELECT COALESCE(s.status, r.status) AS status
			FROM job_runs r
			LEFT JOIN job_run_read_state s ON s.run_id = r.id
			WHERE r.project_id = $1 AND r.created_at >= $2
		)
		SELECT
			COUNT(*) FILTER (WHERE status = 'completed') as completed,
			COUNT(*) FILTER (WHERE status = 'failed') as failed,
			COUNT(*) FILTER (WHERE status = 'timed_out') as timed_out,
			COUNT(*) FILTER (WHERE status = 'canceled') as canceled
		FROM runs`

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
		WITH runs AS (
			SELECT
				COALESCE(s.status, r.status) AS status,
				COALESCE(s.started_at, r.started_at) AS started_at,
				COALESCE(s.finished_at, r.finished_at) AS finished_at
			FROM job_runs r
			LEFT JOIN job_run_read_state s ON s.run_id = r.id
			WHERE r.project_id = $1 AND r.created_at >= $2
		)
		SELECT
			(SELECT COUNT(*) FROM jobs WHERE project_id = $1) as total_jobs,
			(SELECT COUNT(*) FROM jobs WHERE project_id = $1 AND enabled = true) as active_jobs,
			CASE WHEN COUNT(*) > 0
				THEN ROUND(COUNT(*) FILTER (WHERE status = 'completed')::numeric / COUNT(*)::numeric, 4)
				ELSE 0
			END as success_rate,
			COALESCE(AVG(EXTRACT(EPOCH FROM (finished_at - started_at))) FILTER (WHERE finished_at IS NOT NULL AND started_at IS NOT NULL), 0) as avg_duration,
			(SELECT COUNT(*) FROM job_run_read_state WHERE project_id = $1 AND status = 'queued') as queue_depth
		FROM runs`

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

// getPerformanceAnalyticsMaterialized uses the pre-aggregated job_stats_hourly table.
func (q *Queries) getPerformanceAnalyticsMaterialized(ctx context.Context, projectID string, periodHours int) (*PerformanceAnalytics, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetPerformanceAnalyticsMaterialized")
	defer span.End()

	since := time.Now().Add(-time.Duration(periodHours) * time.Hour)
	result := &PerformanceAnalytics{
		SlowestJobs: make([]JobPerformance, 0, 10),
		Throughput:  ThroughputStats{PeriodHours: periodHours},
	}

	slowestQuery := `
		SELECT s.job_id, COALESCE(j.slug, s.job_id),
			COALESCE(AVG(s.avg_duration_ms) / 1000.0, 0) as avg_duration,
			COALESCE(MAX(s.p95_duration_ms) / 1000.0, 0) as p95_duration,
			SUM(s.total) as total_runs,
			SUM(s.failed) as failed_runs
		FROM job_stats_hourly s
		LEFT JOIN jobs j ON j.id = s.job_id
		WHERE s.project_id = $1 AND s.hour >= $2
		GROUP BY s.job_id, j.slug
		HAVING SUM(s.total) >= 5
		ORDER BY avg_duration DESC
		LIMIT 10`

	rows, err := q.db.Query(ctx, slowestQuery, projectID, since)
	if err != nil {
		return nil, fmt.Errorf("materialized analytics slowest jobs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var jp JobPerformance
		if err := rows.Scan(&jp.JobID, &jp.JobSlug, &jp.AvgDurationSecs, &jp.P95DurationSecs, &jp.TotalRuns, &jp.FailedRuns); err != nil {
			return nil, fmt.Errorf("materialized analytics slowest jobs scan: %w", err)
		}
		result.SlowestJobs = append(result.SlowestJobs, jp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("materialized analytics slowest jobs rows: %w", err)
	}

	throughputQuery := `
		SELECT
			COALESCE(SUM(completed), 0),
			COALESCE(SUM(failed), 0),
			COALESCE(SUM(timed_out), 0),
			COALESCE(SUM(canceled), 0)
		FROM job_stats_hourly
		WHERE project_id = $1 AND hour >= $2`

	err = q.db.QueryRow(ctx, throughputQuery, projectID, since).Scan(
		&result.Throughput.Completed,
		&result.Throughput.Failed,
		&result.Throughput.TimedOut,
		&result.Throughput.Canceled,
	)
	if err != nil {
		return nil, fmt.Errorf("materialized analytics throughput: %w", err)
	}

	// Health summary still reads live data (it includes queue depth and active jobs).
	healthQuery := `
		SELECT
			(SELECT COUNT(*) FROM jobs WHERE project_id = $1) as total_jobs,
			(SELECT COUNT(*) FROM jobs WHERE project_id = $1 AND enabled = true) as active_jobs,
			CASE WHEN COALESCE(SUM(s.total), 0) > 0
				THEN ROUND(SUM(s.completed)::numeric / SUM(s.total)::numeric, 4)
				ELSE 0
			END as success_rate,
			COALESCE(AVG(s.avg_duration_ms) / 1000.0, 0) as avg_duration,
			(SELECT COUNT(*) FROM job_run_read_state WHERE project_id = $1 AND status = 'queued') as queue_depth
		FROM job_stats_hourly s
		WHERE s.project_id = $1 AND s.hour >= $2`

	err = q.db.QueryRow(ctx, healthQuery, projectID, since).Scan(
		&result.HealthSummary.TotalJobs,
		&result.HealthSummary.ActiveJobs,
		&result.HealthSummary.SuccessRate,
		&result.HealthSummary.AvgDurationSecs,
		&result.HealthSummary.QueueDepth,
	)
	if err != nil {
		return nil, fmt.Errorf("materialized analytics health: %w", err)
	}

	return result, nil
}
