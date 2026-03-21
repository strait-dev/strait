package clickhouse

import (
	"context"
	"fmt"
	"time"

	"strait/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgHealthQuerier provides live Postgres data that cannot be served from ClickHouse
// (current job counts, queue depth).
type PgHealthQuerier interface {
	CountJobs(ctx context.Context, projectID string) (total, active int, err error)
	QueueDepth(ctx context.Context, projectID string) (int, error)
}

// AnalyticsStore implements analytics queries against ClickHouse tables.
// It falls back to a PgHealthQuerier for live operational data that only
// exists in Postgres (job counts, queue depth).
type AnalyticsStore struct {
	client     *Client
	pgFallback PgHealthQuerier
}

// NewAnalyticsStore creates a new ClickHouse-backed analytics store.
func NewAnalyticsStore(client *Client, pgFallback PgHealthQuerier) *AnalyticsStore {
	return &AnalyticsStore{
		client:     client,
		pgFallback: pgFallback,
	}
}

// GetPerformanceAnalytics returns run performance data from ClickHouse with
// live health data from Postgres.
func (s *AnalyticsStore) GetPerformanceAnalytics(ctx context.Context, projectID string, periodHours int) (*store.PerformanceAnalytics, error) {
	since := time.Now().Add(-time.Duration(periodHours) * time.Hour)
	result := &store.PerformanceAnalytics{
		SlowestJobs: make([]store.JobPerformance, 0, 10),
		Throughput:  store.ThroughputStats{PeriodHours: periodHours},
	}

	// Slowest jobs from run_analytics joined with job_metadata for slugs.
	slowestQuery := `
		SELECT ra.job_id,
			COALESCE(jm.slug, ra.job_id) AS job_slug,
			avg(ra.duration_ms / 1000.0) AS avg_duration,
			quantile(0.95)(ra.duration_ms / 1000.0) AS p95_duration,
			count() AS total_runs,
			countIf(ra.status = 'failed') AS failed_runs
		FROM run_analytics ra
		LEFT JOIN job_metadata FINAL jm ON jm.job_id = ra.job_id AND jm.project_id = ra.project_id
		WHERE ra.project_id = ?
			AND ra.created_at >= ?
			AND ra.duration_ms > 0
		GROUP BY ra.job_id, jm.slug
		HAVING count() >= 5
		ORDER BY avg_duration DESC
		LIMIT 10`

	rows, err := s.client.Query(ctx, slowestQuery, projectID, since)
	if err != nil {
		return nil, fmt.Errorf("clickhouse analytics slowest jobs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var jp store.JobPerformance
		if err := rows.Scan(&jp.JobID, &jp.JobSlug, &jp.AvgDurationSecs, &jp.P95DurationSecs, &jp.TotalRuns, &jp.FailedRuns); err != nil {
			return nil, fmt.Errorf("clickhouse analytics slowest jobs scan: %w", err)
		}
		result.SlowestJobs = append(result.SlowestJobs, jp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse analytics slowest jobs rows: %w", err)
	}

	// Throughput from run_analytics.
	throughputQuery := `
		SELECT
			countIf(status = 'completed') AS completed,
			countIf(status = 'failed') AS failed,
			countIf(status = 'timed_out') AS timed_out,
			countIf(status = 'canceled') AS canceled
		FROM run_analytics
		WHERE project_id = ? AND created_at >= ?`

	if err := s.client.QueryRow(ctx, throughputQuery, projectID, since).Scan(
		&result.Throughput.Completed,
		&result.Throughput.Failed,
		&result.Throughput.TimedOut,
		&result.Throughput.Canceled,
	); err != nil {
		return nil, fmt.Errorf("clickhouse analytics throughput: %w", err)
	}

	// Health summary: success rate and avg duration from ClickHouse,
	// total_jobs, active_jobs, queue_depth from Postgres.
	healthQuery := `
		SELECT
			CASE WHEN count() > 0
				THEN round(countIf(status = 'completed') / count(), 4)
				ELSE 0
			END AS success_rate,
			coalesce(avg(duration_ms / 1000.0), 0) AS avg_duration
		FROM run_analytics
		WHERE project_id = ? AND created_at >= ?`

	if err := s.client.QueryRow(ctx, healthQuery, projectID, since).Scan(
		&result.HealthSummary.SuccessRate,
		&result.HealthSummary.AvgDurationSecs,
	); err != nil {
		return nil, fmt.Errorf("clickhouse analytics health: %w", err)
	}

	// Live data from Postgres.
	if s.pgFallback != nil {
		total, active, err := s.pgFallback.CountJobs(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("clickhouse analytics count jobs: %w", err)
		}
		result.HealthSummary.TotalJobs = total
		result.HealthSummary.ActiveJobs = active

		depth, err := s.pgFallback.QueueDepth(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("clickhouse analytics queue depth: %w", err)
		}
		result.HealthSummary.QueueDepth = depth
	}

	return result, nil
}

// GetCostAnalytics returns aggregated cost data from ClickHouse.
func (s *AnalyticsStore) GetCostAnalytics(ctx context.Context, projectID string, from, to time.Time) (*store.CostAnalytics, error) {
	result := &store.CostAnalytics{
		ByModel: make([]store.CostByModel, 0),
		ByJob:   make([]store.CostByJob, 0),
	}

	// AI cost totals from run_usage_events.
	aiQuery := `
		SELECT coalesce(sum(cost_microusd), 0),
			coalesce(sum(total_tokens), 0),
			count(DISTINCT run_id)
		FROM run_usage_events
		WHERE project_id = ? AND created_at >= ? AND created_at < ?`
	if err := s.client.QueryRow(ctx, aiQuery, projectID, from, to).Scan(
		&result.TotalAICostMicrousd, &result.TotalTokens, &result.RunCount,
	); err != nil {
		return nil, fmt.Errorf("clickhouse cost analytics ai totals: %w", err)
	}

	// Compute cost totals.
	computeQuery := `
		SELECT coalesce(sum(cost_microusd), 0)
		FROM compute_usage
		WHERE project_id = ? AND started_at >= ? AND started_at < ?`
	if err := s.client.QueryRow(ctx, computeQuery, projectID, from, to).Scan(
		&result.TotalComputeCostMicrousd,
	); err != nil {
		return nil, fmt.Errorf("clickhouse cost analytics compute totals: %w", err)
	}

	// By model breakdown.
	modelQuery := `
		SELECT model,
			sum(cost_microusd),
			sum(total_tokens),
			count()
		FROM run_usage_events
		WHERE project_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY model
		ORDER BY sum(cost_microusd) DESC`
	modelRows, err := s.client.Query(ctx, modelQuery, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse cost analytics by model: %w", err)
	}
	defer modelRows.Close()
	for modelRows.Next() {
		var m store.CostByModel
		if err := modelRows.Scan(&m.Model, &m.CostMicrousd, &m.TotalTokens, &m.UsageCount); err != nil {
			return nil, fmt.Errorf("clickhouse cost analytics by model scan: %w", err)
		}
		result.ByModel = append(result.ByModel, m)
	}
	if err := modelRows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse cost analytics by model rows: %w", err)
	}

	// By job breakdown.
	jobQuery := `
		SELECT ru.job_id,
			coalesce(jm.slug, ru.job_id),
			sum(ru.cost_microusd),
			count(DISTINCT ru.run_id)
		FROM run_usage_events ru
		LEFT JOIN job_metadata FINAL jm ON jm.job_id = ru.job_id AND jm.project_id = ru.project_id
		WHERE ru.project_id = ? AND ru.created_at >= ? AND ru.created_at < ?
		GROUP BY ru.job_id, jm.slug
		ORDER BY sum(ru.cost_microusd) DESC`
	jobRows, err := s.client.Query(ctx, jobQuery, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse cost analytics by job: %w", err)
	}
	defer jobRows.Close()
	for jobRows.Next() {
		var j store.CostByJob
		if err := jobRows.Scan(&j.JobID, &j.JobSlug, &j.CostMicrousd, &j.RunCount); err != nil {
			return nil, fmt.Errorf("clickhouse cost analytics by job scan: %w", err)
		}
		result.ByJob = append(result.ByJob, j)
	}
	if err := jobRows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse cost analytics by job rows: %w", err)
	}

	return result, nil
}

// isShortPeriod returns true when the time range is 24 hours or less.
func isShortPeriod(from, to time.Time) bool {
	return to.Sub(from) <= 24*time.Hour
}

// GetCostTrends returns a time-series of cost data points from ClickHouse.
func (s *AnalyticsStore) GetCostTrends(ctx context.Context, projectID string, from, to time.Time) ([]store.CostTrendPoint, error) {
	var truncFn string
	if isShortPeriod(from, to) {
		truncFn = "toStartOfHour"
	} else {
		truncFn = "toStartOfDay"
	}

	query := fmt.Sprintf(`
		SELECT %s(ru.created_at) AS period,
			coalesce(sum(ru.cost_microusd), 0) AS ai_cost,
			0 AS compute_cost,
			coalesce(sum(ru.total_tokens), 0),
			count(DISTINCT ru.run_id)
		FROM run_usage_events ru
		WHERE ru.project_id = ? AND ru.created_at >= ? AND ru.created_at < ?
		GROUP BY period
		ORDER BY period`, truncFn)

	rows, err := s.client.Query(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse cost trends: %w", err)
	}
	defer rows.Close()

	points := make([]store.CostTrendPoint, 0)
	for rows.Next() {
		var p store.CostTrendPoint
		var period time.Time
		if err := rows.Scan(&period, &p.AICostMicrousd, &p.ComputeCostMicrousd, &p.TotalTokens, &p.RunCount); err != nil {
			return nil, fmt.Errorf("clickhouse cost trends scan: %w", err)
		}
		p.Period = period.Format(time.RFC3339)
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetTopCosts returns the top N most expensive jobs by total AI cost from ClickHouse.
func (s *AnalyticsStore) GetTopCosts(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopCostItem, error) {
	query := `
		SELECT ru.job_id,
			coalesce(jm.slug, ru.job_id),
			sum(ru.cost_microusd),
			count(DISTINCT ru.run_id)
		FROM run_usage_events ru
		LEFT JOIN job_metadata FINAL jm ON jm.job_id = ru.job_id AND jm.project_id = ru.project_id
		WHERE ru.project_id = ? AND ru.created_at >= ? AND ru.created_at < ?
		GROUP BY ru.job_id, jm.slug
		ORDER BY sum(ru.cost_microusd) DESC
		LIMIT ?`

	rows, err := s.client.Query(ctx, query, projectID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("clickhouse top costs: %w", err)
	}
	defer rows.Close()

	items := make([]store.TopCostItem, 0, limit)
	for rows.Next() {
		var item store.TopCostItem
		if err := rows.Scan(&item.ID, &item.Name, &item.CostMicrousd, &item.RunCount); err != nil {
			return nil, fmt.Errorf("clickhouse top costs scan: %w", err)
		}
		item.ItemType = "job"
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetComputeCostAnalytics returns compute costs grouped by machine preset from ClickHouse.
func (s *AnalyticsStore) GetComputeCostAnalytics(ctx context.Context, projectID string, from, to time.Time) (*store.ComputeCostAnalytics, error) {
	result := &store.ComputeCostAnalytics{
		ByPreset: make([]store.CostByPreset, 0),
	}

	totalQuery := `
		SELECT coalesce(sum(cost_microusd), 0)
		FROM compute_usage
		WHERE project_id = ? AND started_at >= ? AND started_at < ?`
	if err := s.client.QueryRow(ctx, totalQuery, projectID, from, to).Scan(&result.TotalCostMicrousd); err != nil {
		return nil, fmt.Errorf("clickhouse compute cost analytics total: %w", err)
	}

	presetQuery := `
		SELECT machine_preset,
			sum(cost_microusd),
			count(),
			coalesce(sum(duration_secs), 0)
		FROM compute_usage
		WHERE project_id = ? AND started_at >= ? AND started_at < ?
		GROUP BY machine_preset
		ORDER BY sum(cost_microusd) DESC`

	rows, err := s.client.Query(ctx, presetQuery, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse compute cost analytics by preset: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p store.CostByPreset
		if err := rows.Scan(&p.Preset, &p.CostMicrousd, &p.RunCount, &p.DurationSecs); err != nil {
			return nil, fmt.Errorf("clickhouse compute cost analytics by preset scan: %w", err)
		}
		result.ByPreset = append(result.ByPreset, p)
	}
	return result, rows.Err()
}

// GetCostOutliers finds runs whose total cost exceeds the per-job average by
// more than threshold standard deviations from ClickHouse data.
func (s *AnalyticsStore) GetCostOutliers(ctx context.Context, projectID string, from, to time.Time, threshold float64) ([]store.CostOutlier, error) {
	query := `
		WITH run_costs AS (
			SELECT
				run_id,
				job_id,
				sum(cost_microusd) AS cost_microusd
			FROM run_usage_events
			WHERE project_id = ?
				AND created_at >= ?
				AND created_at < ?
			GROUP BY run_id, job_id
		),
		job_stats AS (
			SELECT
				job_id,
				avg(cost_microusd) AS avg_cost,
				stddevPop(cost_microusd) AS stddev_cost
			FROM run_costs
			GROUP BY job_id
			HAVING stddevPop(cost_microusd) > 0
		)
		SELECT
			rc.run_id,
			rc.job_id,
			rc.cost_microusd,
			js.avg_cost,
			js.stddev_cost,
			(rc.cost_microusd - js.avg_cost) / js.stddev_cost AS deviations_above
		FROM run_costs rc
		JOIN job_stats js ON js.job_id = rc.job_id
		WHERE rc.cost_microusd > js.avg_cost + (? * js.stddev_cost)
		ORDER BY deviations_above DESC
		LIMIT 100`

	rows, err := s.client.Query(ctx, query, projectID, from, to, threshold)
	if err != nil {
		return nil, fmt.Errorf("clickhouse cost outliers: %w", err)
	}
	defer rows.Close()

	outliers := make([]store.CostOutlier, 0)
	for rows.Next() {
		var o store.CostOutlier
		if err := rows.Scan(&o.RunID, &o.JobID, &o.CostMicrousd, &o.AvgCostMicrousd, &o.StddevMicrousd, &o.DeviationsAbove); err != nil {
			return nil, fmt.Errorf("clickhouse cost outliers scan: %w", err)
		}
		outliers = append(outliers, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse cost outliers rows: %w", err)
	}

	return outliers, nil
}

// PgHealthAdapter implements PgHealthQuerier by running simple count queries
// against a Postgres connection pool.
type PgHealthAdapter struct {
	pool *pgxpool.Pool
}

// NewPgHealthAdapter creates a PgHealthQuerier backed by a pgx connection pool.
func NewPgHealthAdapter(pool *pgxpool.Pool) *PgHealthAdapter {
	return &PgHealthAdapter{pool: pool}
}

func (a *PgHealthAdapter) CountJobs(ctx context.Context, projectID string) (total, active int, err error) {
	err = a.pool.QueryRow(ctx,
		`SELECT
			(SELECT COUNT(*) FROM jobs WHERE project_id = $1),
			(SELECT COUNT(*) FROM jobs WHERE project_id = $1 AND enabled = true)`,
		projectID,
	).Scan(&total, &active)
	if err != nil {
		return 0, 0, fmt.Errorf("count jobs: %w", err)
	}
	return total, active, nil
}

func (a *PgHealthAdapter) QueueDepth(ctx context.Context, projectID string) (int, error) {
	var depth int
	err := a.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_runs WHERE project_id = $1 AND status = 'queued'`,
		projectID,
	).Scan(&depth)
	if err != nil {
		return 0, fmt.Errorf("queue depth: %w", err)
	}
	return depth, nil
}

// GetApprovalStats returns aggregate approval statistics from ClickHouse.
func (s *AnalyticsStore) GetApprovalStats(ctx context.Context, projectID string, from, to time.Time) (*store.ApprovalStats, error) {
	query := `
		SELECT
			count() AS total_requested,
			countIf(status = 'approved') AS total_approved,
			countIf(status = 'timed_out') AS total_timed_out,
			countIf(status = 'pending') AS total_pending,
			coalesce(
				avgIf(
					dateDiff('second', requested_at, approved_at),
					status = 'approved' AND approved_at IS NOT NULL
				), 0
			) AS avg_approval_time_secs
		FROM workflow_approval_events
		WHERE project_id = ?
			AND requested_at >= ?
			AND requested_at < ?`

	var stats store.ApprovalStats
	if err := s.client.QueryRow(ctx, query, projectID, from, to).Scan(
		&stats.TotalRequested,
		&stats.TotalApproved,
		&stats.TotalTimedOut,
		&stats.TotalPending,
		&stats.AvgApprovalTimeSecs,
	); err != nil {
		return nil, fmt.Errorf("clickhouse approval stats: %w", err)
	}

	return &stats, nil
}
