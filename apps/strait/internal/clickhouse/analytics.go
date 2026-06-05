package clickhouse

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"strait/internal/httputil"
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

	throughputRow, err := s.client.QueryRow(ctx, throughputQuery, projectID, since)
	if err != nil {
		return nil, fmt.Errorf("clickhouse analytics throughput: %w", err)
	}
	if err := throughputRow.Scan(
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

	healthRow, err := s.client.QueryRow(ctx, healthQuery, projectID, since)
	if err != nil {
		return nil, fmt.Errorf("clickhouse analytics health: %w", err)
	}
	if err := healthRow.Scan(
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
		ByJob: make([]store.CostByJob, 0),
	}

	usageQuery := `
		SELECT coalesce(sum(compute_cost_microusd), 0),
			count()
		FROM run_analytics
		WHERE project_id = ? AND created_at >= ? AND created_at < ?`
	usageRow, err := s.client.QueryRow(ctx, usageQuery, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse cost analytics launch totals: %w", err)
	}
	if err := usageRow.Scan(
		&result.TotalSpendMicrousd,
		&result.RunCount,
	); err != nil {
		return nil, fmt.Errorf("clickhouse cost analytics launch totals: %w", err)
	}

	// By job breakdown.
	jobQuery := `
		SELECT ra.job_id,
			coalesce(jm.slug, ra.job_id),
			coalesce(sum(ra.compute_cost_microusd), 0),
			count()
		FROM run_analytics ra
		LEFT JOIN job_metadata FINAL jm ON jm.job_id = ra.job_id AND jm.project_id = ra.project_id
		WHERE ra.project_id = ? AND ra.created_at >= ? AND ra.created_at < ?
		GROUP BY ra.job_id, jm.slug
		ORDER BY coalesce(sum(ra.compute_cost_microusd), 0) DESC, count() DESC`
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
		SELECT %s(ra.created_at) AS period,
			coalesce(sum(ra.compute_cost_microusd), 0) AS compute_cost,
			count()
		FROM run_analytics ra
		WHERE ra.project_id = ? AND ra.created_at >= ? AND ra.created_at < ?
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
		if err := rows.Scan(&period, &p.SpendMicrousd, &p.RunCount); err != nil {
			return nil, fmt.Errorf("clickhouse cost trends scan: %w", err)
		}
		p.Period = period.Format(time.RFC3339)
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetTopCosts returns top-cost entities by recorded compute cost.
func (s *AnalyticsStore) GetTopCosts(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopCostItem, error) {
	query := `
		SELECT ra.job_id,
			coalesce(jm.slug, ra.job_id),
			coalesce(sum(ra.compute_cost_microusd), 0) AS cost,
			count() AS run_count
		FROM run_analytics ra
		LEFT JOIN job_metadata FINAL jm ON jm.job_id = ra.job_id AND jm.project_id = ra.project_id
		WHERE ra.project_id = ? AND ra.created_at >= ? AND ra.created_at < ?
		GROUP BY ra.job_id, jm.slug
		ORDER BY cost DESC, run_count DESC
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

// GetCostOutliers finds runs whose total cost exceeds the per-job average by
// more than threshold standard deviations from ClickHouse data.
func (s *AnalyticsStore) GetCostOutliers(ctx context.Context, projectID string, from, to time.Time, threshold float64) ([]store.CostOutlier, error) {
	query := `
		WITH run_costs AS (
			SELECT
				run_id,
				job_id,
				sum(compute_cost_microusd) AS cost_microusd
			FROM run_analytics
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
		`SELECT COUNT(*) FROM job_run_read_state WHERE project_id = $1 AND status = 'queued'`,
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
	approvalRow, err := s.client.QueryRow(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse approval stats: %w", err)
	}
	if err := approvalRow.Scan(
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

// Run Analytics.

// GetRunTimeline returns run status counts grouped by time bucket.
func (s *AnalyticsStore) GetRunTimeline(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.RunTimelineBucket, error) {
	truncFn := "toStartOfDay"
	if bucket == "hour" {
		truncFn = "toStartOfHour"
	}

	query := fmt.Sprintf(`
		SELECT %s(created_at) AS period,
			countIf(status = 'completed') AS completed,
			countIf(status = 'failed') AS failed,
			countIf(status = 'timed_out') AS timed_out,
			count() AS total
		FROM run_analytics
		WHERE project_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY period
		ORDER BY period`, truncFn)

	rows, err := s.client.Query(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse run timeline: %w", err)
	}
	defer rows.Close()

	result := make([]store.RunTimelineBucket, 0)
	for rows.Next() {
		var b store.RunTimelineBucket
		var period time.Time
		if err := rows.Scan(&period, &b.Completed, &b.Failed, &b.TimedOut, &b.Total); err != nil {
			return nil, fmt.Errorf("clickhouse run timeline scan: %w", err)
		}
		b.Period = period.Format(time.RFC3339)
		result = append(result, b)
	}
	return result, rows.Err()
}

// GetRunDurationDistribution returns runs bucketed by duration range.
func (s *AnalyticsStore) GetRunDurationDistribution(ctx context.Context, projectID string, from, to time.Time) ([]store.RunDurationBucket, error) {
	query := `
		SELECT
			multiIf(
				duration_ms < 1000, '<1s',
				duration_ms < 5000, '1-5s',
				duration_ms < 30000, '5-30s',
				duration_ms < 60000, '30-60s',
				'>60s'
			) AS range,
			count() AS count
		FROM run_analytics
		WHERE project_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY range
		ORDER BY min(duration_ms)`

	rows, err := s.client.Query(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse duration distribution: %w", err)
	}
	defer rows.Close()

	buckets := make([]store.RunDurationBucket, 0)
	var total int
	for rows.Next() {
		var b store.RunDurationBucket
		if err := rows.Scan(&b.Range, &b.Count); err != nil {
			return nil, fmt.Errorf("clickhouse duration distribution scan: %w", err)
		}
		total += b.Count
		buckets = append(buckets, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse duration distribution rows: %w", err)
	}

	for i := range buckets {
		if total > 0 {
			buckets[i].Pct = float64(buckets[i].Count) / float64(total) * 100
		}
	}
	return buckets, nil
}

// GetRunFailureReasons returns top failure messages from run events.
func (s *AnalyticsStore) GetRunFailureReasons(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.RunFailureReason, error) {
	query := `
		SELECT message_class,
			count() AS count,
			max(created_at) AS last_seen,
			any(run_id) AS example_run_id
		FROM run_events
		WHERE project_id = ? AND level = 'error' AND created_at >= ? AND created_at < ?
		GROUP BY message_class
		ORDER BY count DESC
		LIMIT ?`

	rows, err := s.client.Query(ctx, query, projectID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("clickhouse failure reasons: %w", err)
	}
	defer rows.Close()

	type failureReasonAgg struct {
		reason   store.RunFailureReason
		lastSeen time.Time
	}
	byReason := make(map[string]*failureReasonAgg)
	order := make([]string, 0)
	for rows.Next() {
		var rawMessage string
		var count int
		var lastSeen time.Time
		var exampleRunID string
		if err := rows.Scan(&rawMessage, &count, &lastSeen, &exampleRunID); err != nil {
			return nil, fmt.Errorf("clickhouse failure reasons scan: %w", err)
		}
		reason := safeRunFailureReason(rawMessage)
		agg, ok := byReason[reason]
		if !ok {
			agg = &failureReasonAgg{
				reason: store.RunFailureReason{
					Message:      reason,
					ExampleRunID: exampleRunID,
				},
				lastSeen: lastSeen,
			}
			byReason[reason] = agg
			order = append(order, reason)
		}
		agg.reason.Count += count
		if lastSeen.After(agg.lastSeen) || agg.reason.LastSeen == "" {
			agg.lastSeen = lastSeen
			agg.reason.LastSeen = lastSeen.Format(time.RFC3339)
			agg.reason.ExampleRunID = exampleRunID
		}
	}
	result := make([]store.RunFailureReason, 0, len(order))
	for _, reason := range order {
		result = append(result, byReason[reason].reason)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})
	return result, rows.Err()
}

func safeRunFailureReason(message string) string {
	normalized := strings.ToLower(strings.TrimSpace(message))
	if normalized == "" {
		return "unknown_error"
	}
	switch {
	case strings.Contains(normalized, "timeout") || strings.Contains(normalized, "deadline exceeded"):
		return "timeout"
	case strings.Contains(normalized, "context canceled") || strings.Contains(normalized, "context cancelled"):
		return "canceled"
	case strings.Contains(normalized, "rate limit") || strings.Contains(normalized, "too many requests") || strings.Contains(normalized, "status 429"):
		return "rate_limited"
	case strings.Contains(normalized, "dns") ||
		strings.Contains(normalized, "no such host") ||
		strings.Contains(normalized, "connection refused") ||
		strings.Contains(normalized, "connection reset") ||
		strings.Contains(normalized, "network"):
		return "network_error"
	case strings.Contains(normalized, "status 4") || strings.Contains(normalized, "http 4"):
		return "http_client_error"
	case strings.Contains(normalized, "status 5") || strings.Contains(normalized, "http 5"):
		return "http_server_error"
	case strings.Contains(normalized, "panic"):
		return "panic"
	case strings.Contains(normalized, "invalid") || strings.Contains(normalized, "validation"):
		return "validation_error"
	default:
		return "application_error"
	}
}

// GetRunSummary returns aggregate run statistics.
func (s *AnalyticsStore) GetRunSummary(ctx context.Context, projectID string, from, to time.Time) (*store.RunSummary, error) {
	query := `
		SELECT
			count() AS total,
			countIf(status = 'completed') AS completed,
			countIf(status = 'failed') AS failed,
			countIf(status = 'timed_out') AS timed_out,
			CASE WHEN count() > 0
				THEN countIf(status = 'completed') / count()
				ELSE 0
			END AS success_rate,
			coalesce(avg(duration_ms), 0) AS avg_duration_ms,
			coalesce(quantile(0.95)(duration_ms), 0) AS p95_duration_ms
		FROM run_analytics
		WHERE project_id = ? AND created_at >= ? AND created_at < ?`

	var summary store.RunSummary
	summaryRow, err := s.client.QueryRow(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse run summary: %w", err)
	}
	if err := summaryRow.Scan(
		&summary.Total, &summary.Completed, &summary.Failed, &summary.TimedOut,
		&summary.SuccessRate, &summary.AvgDurationMs, &summary.P95DurationMs,
	); err != nil {
		return nil, fmt.Errorf("clickhouse run summary: %w", err)
	}
	return &summary, nil
}

// GetRunsByTrigger returns run stats grouped by trigger type.
func (s *AnalyticsStore) GetRunsByTrigger(ctx context.Context, projectID string, from, to time.Time) ([]store.RunsByTrigger, error) {
	query := `
		SELECT triggered_by,
			count() AS total,
			countIf(status = 'completed') AS completed,
			countIf(status = 'failed') AS failed,
			coalesce(avg(duration_ms), 0) AS avg_duration_ms
		FROM run_analytics
		WHERE project_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY triggered_by
		ORDER BY total DESC`

	rows, err := s.client.Query(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse runs by trigger: %w", err)
	}
	defer rows.Close()

	result := make([]store.RunsByTrigger, 0)
	for rows.Next() {
		var r store.RunsByTrigger
		if err := rows.Scan(&r.TriggerType, &r.Total, &r.Completed, &r.Failed, &r.AvgDurationMs); err != nil {
			return nil, fmt.Errorf("clickhouse runs by trigger scan: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// Job Analytics.

// GetJobHistory returns a timeline of run stats for a specific job.
func (s *AnalyticsStore) GetJobHistory(ctx context.Context, projectID, jobID string, from, to time.Time, bucket string) ([]store.JobHistoryBucket, error) {
	truncFn := "toStartOfDay"
	if bucket == "hour" {
		truncFn = "toStartOfHour"
	}

	query := fmt.Sprintf(`
		SELECT %s(created_at) AS period,
			countIf(status = 'completed') AS completed,
			countIf(status = 'failed') AS failed,
			coalesce(avg(duration_ms), 0) AS avg_duration_ms,
			coalesce(quantile(0.95)(duration_ms), 0) AS p95_duration_ms
		FROM run_analytics
		WHERE project_id = ? AND job_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY period
		ORDER BY period`, truncFn)

	rows, err := s.client.Query(ctx, query, projectID, jobID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse job history: %w", err)
	}
	defer rows.Close()

	result := make([]store.JobHistoryBucket, 0)
	for rows.Next() {
		var b store.JobHistoryBucket
		var period time.Time
		if err := rows.Scan(&period, &b.Completed, &b.Failed, &b.AvgDurationMs, &b.P95DurationMs); err != nil {
			return nil, fmt.Errorf("clickhouse job history scan: %w", err)
		}
		b.Period = period.Format(time.RFC3339)
		result = append(result, b)
	}
	return result, rows.Err()
}

// GetJobComparison compares metrics across specified jobs.
func (s *AnalyticsStore) GetJobComparison(ctx context.Context, projectID string, jobIDs []string, from, to time.Time) ([]store.JobComparison, error) {
	query := `
		SELECT ra.job_id,
			COALESCE(jm.slug, ra.job_id) AS slug,
			count() AS total,
			CASE WHEN count() > 0
				THEN countIf(ra.status = 'completed') / count()
				ELSE 0
			END AS success_rate,
			coalesce(avg(ra.duration_ms), 0) AS avg_duration_ms,
			coalesce(sum(ra.compute_cost_microusd), 0) AS cost
		FROM run_analytics ra
		LEFT JOIN job_metadata FINAL jm ON jm.job_id = ra.job_id AND jm.project_id = ra.project_id
		WHERE ra.project_id = ? AND ra.job_id IN ? AND ra.created_at >= ? AND ra.created_at < ?
		GROUP BY ra.job_id, jm.slug
		ORDER BY total DESC`

	rows, err := s.client.Query(ctx, query, projectID, jobIDs, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse job comparison: %w", err)
	}
	defer rows.Close()

	result := make([]store.JobComparison, 0)
	for rows.Next() {
		var j store.JobComparison
		if err := rows.Scan(&j.JobID, &j.Slug, &j.Total, &j.SuccessRate, &j.AvgDurationMs, &j.Cost); err != nil {
			return nil, fmt.Errorf("clickhouse job comparison scan: %w", err)
		}
		result = append(result, j)
	}
	return result, rows.Err()
}

// GetJobReliability ranks jobs by failure rate.
func (s *AnalyticsStore) GetJobReliability(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.JobReliability, error) {
	query := `
		SELECT ra.job_id,
			COALESCE(jm.slug, ra.job_id) AS slug,
			count() AS total,
			CASE WHEN count() > 0
				THEN countIf(ra.status = 'completed') / count()
				ELSE 0
			END AS success_rate,
			countIf(ra.status = 'failed') AS failed,
			0 AS consecutive_failures
		FROM run_analytics ra
		LEFT JOIN job_metadata FINAL jm ON jm.job_id = ra.job_id AND jm.project_id = ra.project_id
		WHERE ra.project_id = ? AND ra.created_at >= ? AND ra.created_at < ?
		GROUP BY ra.job_id, jm.slug
		HAVING count() >= 5
		ORDER BY success_rate ASC
		LIMIT ?`

	rows, err := s.client.Query(ctx, query, projectID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("clickhouse job reliability: %w", err)
	}
	defer rows.Close()

	result := make([]store.JobReliability, 0)
	for rows.Next() {
		var j store.JobReliability
		if err := rows.Scan(&j.JobID, &j.Slug, &j.Total, &j.SuccessRate, &j.Failed, &j.ConsecutiveFailures); err != nil {
			return nil, fmt.Errorf("clickhouse job reliability scan: %w", err)
		}
		result = append(result, j)
	}
	return result, rows.Err()
}

// GetRunsByVersion groups run stats by job version.
func (s *AnalyticsStore) GetRunsByVersion(ctx context.Context, projectID, jobID string, from, to time.Time) ([]store.RunsByVersion, error) {
	query := `
		SELECT job_version_id,
			count() AS total,
			countIf(status = 'completed') AS completed,
			countIf(status = 'failed') AS failed,
			coalesce(avg(duration_ms), 0) AS avg_duration_ms
		FROM run_analytics
		WHERE project_id = ? AND job_id = ? AND created_at >= ? AND created_at < ?
			AND job_version_id != ''
		GROUP BY job_version_id
		ORDER BY total DESC`

	rows, err := s.client.Query(ctx, query, projectID, jobID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse runs by version: %w", err)
	}
	defer rows.Close()

	result := make([]store.RunsByVersion, 0)
	for rows.Next() {
		var r store.RunsByVersion
		if err := rows.Scan(&r.VersionID, &r.Total, &r.Completed, &r.Failed, &r.AvgDurationMs); err != nil {
			return nil, fmt.Errorf("clickhouse runs by version scan: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetJobCostRanking ranks jobs by total cost.
func (s *AnalyticsStore) GetJobCostRanking(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.JobCostRanking, error) {
	query := `
		SELECT ra.job_id,
			COALESCE(jm.slug, ra.job_id) AS slug,
			coalesce(sum(ra.compute_cost_microusd), 0) AS total_cost,
			count() AS run_count,
			CASE WHEN count() > 0
				THEN coalesce(sum(ra.compute_cost_microusd), 0) / count()
				ELSE 0
			END AS avg_cost_per_run
		FROM run_analytics ra
		LEFT JOIN job_metadata FINAL jm ON jm.job_id = ra.job_id AND jm.project_id = ra.project_id
		WHERE ra.project_id = ? AND ra.created_at >= ? AND ra.created_at < ?
		GROUP BY ra.job_id, jm.slug
		ORDER BY total_cost DESC, run_count DESC
		LIMIT ?`

	rows, err := s.client.Query(ctx, query, projectID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("clickhouse job cost ranking: %w", err)
	}
	defer rows.Close()

	result := make([]store.JobCostRanking, 0)
	for rows.Next() {
		var j store.JobCostRanking
		if err := rows.Scan(&j.JobID, &j.Slug, &j.TotalCost, &j.RunCount, &j.AvgCostPerRun); err != nil {
			return nil, fmt.Errorf("clickhouse job cost ranking scan: %w", err)
		}
		result = append(result, j)
	}
	return result, rows.Err()
}

// GetTopFailingJobs returns jobs sorted by failure count.
func (s *AnalyticsStore) GetTopFailingJobs(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopFailingJob, error) {
	query := `
		SELECT ra.job_id,
			COALESCE(jm.slug, ra.job_id) AS slug,
			countIf(ra.status = 'failed') AS failed_count,
			count() AS total,
			CASE WHEN count() > 0
				THEN countIf(ra.status = 'failed') / count()
				ELSE 0
			END AS failure_rate
		FROM run_analytics ra
		LEFT JOIN job_metadata FINAL jm ON jm.job_id = ra.job_id AND jm.project_id = ra.project_id
		WHERE ra.project_id = ? AND ra.created_at >= ? AND ra.created_at < ?
		GROUP BY ra.job_id, jm.slug
		HAVING countIf(ra.status = 'failed') > 0
		ORDER BY failed_count DESC
		LIMIT ?`

	rows, err := s.client.Query(ctx, query, projectID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("clickhouse top failing jobs: %w", err)
	}
	defer rows.Close()

	result := make([]store.TopFailingJob, 0)
	for rows.Next() {
		var j store.TopFailingJob
		if err := rows.Scan(&j.JobID, &j.Slug, &j.FailedCount, &j.Total, &j.FailureRate); err != nil {
			return nil, fmt.Errorf("clickhouse top failing jobs scan: %w", err)
		}
		result = append(result, j)
	}
	return result, rows.Err()
}

// Tag Analytics.

// GetTagSummary groups run stats by tag key/value pairs.
func (s *AnalyticsStore) GetTagSummary(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TagSummary, error) {
	query := `
		SELECT
			tupleElement(kv, 1) AS tag_key,
			tupleElement(kv, 2) AS tag_value,
			count() AS total,
			countIf(status = 'completed') AS completed,
			countIf(status = 'failed') AS failed,
			coalesce(avg(duration_ms), 0) AS avg_duration_ms
		FROM run_analytics
		ARRAY JOIN arrayJoin(JSONExtractKeysAndValues(tags, 'String')) AS kv
		WHERE project_id = ? AND created_at >= ? AND created_at < ?
			AND tags != '{}'
		GROUP BY tag_key, tag_value
		ORDER BY total DESC
		LIMIT ?`

	rows, err := s.client.Query(ctx, query, projectID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("clickhouse tag summary: %w", err)
	}
	defer rows.Close()

	result := make([]store.TagSummary, 0)
	for rows.Next() {
		var t store.TagSummary
		if err := rows.Scan(&t.TagKey, &t.TagValue, &t.Total, &t.Completed, &t.Failed, &t.AvgDurationMs); err != nil {
			return nil, fmt.Errorf("clickhouse tag summary scan: %w", err)
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// GetTopFailingTags ranks tags by failure rate.
func (s *AnalyticsStore) GetTopFailingTags(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopFailingTag, error) {
	query := `
		SELECT
			tupleElement(kv, 1) AS tag_key,
			tupleElement(kv, 2) AS tag_value,
			countIf(status = 'failed') AS failed,
			count() AS total,
			CASE WHEN count() > 0
				THEN countIf(status = 'failed') / count()
				ELSE 0
			END AS failure_rate
		FROM run_analytics
		ARRAY JOIN arrayJoin(JSONExtractKeysAndValues(tags, 'String')) AS kv
		WHERE project_id = ? AND created_at >= ? AND created_at < ?
			AND tags != '{}'
		GROUP BY tag_key, tag_value
		HAVING countIf(status = 'failed') > 0
		ORDER BY failed DESC
		LIMIT ?`

	rows, err := s.client.Query(ctx, query, projectID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("clickhouse top failing tags: %w", err)
	}
	defer rows.Close()

	result := make([]store.TopFailingTag, 0)
	for rows.Next() {
		var t store.TopFailingTag
		if err := rows.Scan(&t.TagKey, &t.TagValue, &t.Failed, &t.Total, &t.FailureRate); err != nil {
			return nil, fmt.Errorf("clickhouse top failing tags scan: %w", err)
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// GetTagCost groups cost by tag key/value pairs.
func (s *AnalyticsStore) GetTagCost(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TagCost, error) {
	query := `
		SELECT
			tupleElement(kv, 1) AS tag_key,
			tupleElement(kv, 2) AS tag_value,
			coalesce(sum(compute_cost_microusd), 0) AS total_cost,
			count() AS run_count
		FROM run_analytics
		ARRAY JOIN arrayJoin(JSONExtractKeysAndValues(tags, 'String')) AS kv
		WHERE project_id = ? AND created_at >= ? AND created_at < ?
			AND tags != '{}'
		GROUP BY tag_key, tag_value
		ORDER BY total_cost DESC
		LIMIT ?`

	rows, err := s.client.Query(ctx, query, projectID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("clickhouse tag cost: %w", err)
	}
	defer rows.Close()

	result := make([]store.TagCost, 0)
	for rows.Next() {
		var t store.TagCost
		if err := rows.Scan(&t.TagKey, &t.TagValue, &t.TotalCost, &t.RunCount); err != nil {
			return nil, fmt.Errorf("clickhouse tag cost scan: %w", err)
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// Workflow Analytics.

// GetWorkflowStepDurations returns duration stats per workflow step.
func (s *AnalyticsStore) GetWorkflowStepDurations(ctx context.Context, projectID, workflowID string, from, to time.Time) ([]store.StepDuration, error) {
	query := `
		SELECT step_ref,
			coalesce(avg(duration_ms), 0) AS avg_ms,
			coalesce(quantile(0.95)(duration_ms), 0) AS p95_ms,
			count() AS count,
			CASE WHEN count() > 0
				THEN countIf(status = 'failed') / count()
				ELSE 0
			END AS failure_rate
		FROM workflow_step_analytics
		WHERE project_id = ? AND workflow_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY step_ref
		ORDER BY avg_ms DESC`

	rows, err := s.client.Query(ctx, query, projectID, workflowID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse workflow step durations: %w", err)
	}
	defer rows.Close()

	result := make([]store.StepDuration, 0)
	for rows.Next() {
		var d store.StepDuration
		if err := rows.Scan(&d.StepRef, &d.AvgMs, &d.P95Ms, &d.Count, &d.FailureRate); err != nil {
			return nil, fmt.Errorf("clickhouse workflow step durations scan: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// GetWorkflowCompletionRates returns completion counts by time bucket.
func (s *AnalyticsStore) GetWorkflowCompletionRates(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.WorkflowCompletionBucket, error) {
	truncFn := "toStartOfDay"
	if bucket == "hour" {
		truncFn = "toStartOfHour"
	}

	query := fmt.Sprintf(`
		SELECT %s(created_at) AS period,
			countIf(status = 'completed') AS completed,
			countIf(status = 'failed') AS failed,
			countIf(status = 'timed_out') AS timed_out
		FROM workflow_run_analytics
		WHERE project_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY period
		ORDER BY period`, truncFn)

	rows, err := s.client.Query(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse workflow completion rates: %w", err)
	}
	defer rows.Close()

	result := make([]store.WorkflowCompletionBucket, 0)
	for rows.Next() {
		var b store.WorkflowCompletionBucket
		var period time.Time
		if err := rows.Scan(&period, &b.Completed, &b.Failed, &b.TimedOut); err != nil {
			return nil, fmt.Errorf("clickhouse workflow completion rates scan: %w", err)
		}
		b.Period = period.Format(time.RFC3339)
		result = append(result, b)
	}
	return result, rows.Err()
}

// GetWorkflowSummary returns aggregate workflow analytics.
func (s *AnalyticsStore) GetWorkflowSummary(ctx context.Context, projectID string, from, to time.Time) (*store.WorkflowSummary, error) {
	query := `
		SELECT
			count() AS total,
			countIf(status = 'completed') AS completed,
			countIf(status = 'failed') AS failed,
			CASE WHEN count() > 0
				THEN countIf(status = 'completed') / count()
				ELSE 0
			END AS success_rate,
			coalesce(avg(duration_ms), 0) AS avg_duration_ms
		FROM workflow_run_analytics
		WHERE project_id = ? AND created_at >= ? AND created_at < ?`

	var summary store.WorkflowSummary
	wfRow, err := s.client.QueryRow(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse workflow summary: %w", err)
	}
	if err := wfRow.Scan(
		&summary.Total, &summary.Completed, &summary.Failed,
		&summary.SuccessRate, &summary.AvgDurationMs,
	); err != nil {
		return nil, fmt.Errorf("clickhouse workflow summary: %w", err)
	}
	return &summary, nil
}

// Webhook Analytics.

// GetWebhookDeliveryStats returns delivery stats per webhook URL.
func (s *AnalyticsStore) GetWebhookDeliveryStats(ctx context.Context, projectID string, from, to time.Time) ([]store.WebhookEndpointStats, error) {
	query := `
		SELECT webhook_host,
			count() AS total,
			countIf(status = 'delivered') AS delivered,
			countIf(status = 'failed') AS failed,
			countIf(status = 'dead') AS dead,
			coalesce(avg(duration_ms), 0) AS avg_latency_ms,
			coalesce(quantile(0.95)(duration_ms), 0) AS p95_latency_ms
		FROM webhook_delivery_events
		WHERE project_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY webhook_host
		ORDER BY total DESC`

	rows, err := s.client.Query(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse webhook delivery stats: %w", err)
	}
	defer rows.Close()

	result := make([]store.WebhookEndpointStats, 0)
	for rows.Next() {
		var s store.WebhookEndpointStats
		if err := rows.Scan(&s.URL, &s.Total, &s.Delivered, &s.Failed, &s.Dead, &s.AvgLatencyMs, &s.P95LatencyMs); err != nil {
			return nil, fmt.Errorf("clickhouse webhook delivery stats scan: %w", err)
		}
		s.URL = redactWebhookAnalyticsURL(s.URL)
		result = append(result, s)
	}
	return result, rows.Err()
}

// GetWebhookEndpointHealth returns per-URL health over time.
func (s *AnalyticsStore) GetWebhookEndpointHealth(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.WebhookHealthBucket, error) {
	truncFn := "toStartOfDay"
	if bucket == "hour" {
		truncFn = "toStartOfHour"
	}

	query := fmt.Sprintf(`
		SELECT webhook_host,
			%s(created_at) AS period,
			CASE WHEN count() > 0
				THEN countIf(status = 'delivered') / count()
				ELSE 0
			END AS success_rate,
			coalesce(avg(duration_ms), 0) AS avg_latency_ms
		FROM webhook_delivery_events
		WHERE project_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY webhook_host, period
		ORDER BY webhook_host, period`, truncFn)

	rows, err := s.client.Query(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse webhook endpoint health: %w", err)
	}
	defer rows.Close()

	result := make([]store.WebhookHealthBucket, 0)
	for rows.Next() {
		var b store.WebhookHealthBucket
		var period time.Time
		if err := rows.Scan(&b.URL, &period, &b.SuccessRate, &b.AvgLatencyMs); err != nil {
			return nil, fmt.Errorf("clickhouse webhook endpoint health scan: %w", err)
		}
		b.URL = redactWebhookAnalyticsURL(b.URL)
		b.Period = period.Format(time.RFC3339)
		result = append(result, b)
	}
	return result, rows.Err()
}

// GetTopFailingWebhooks ranks webhook endpoints by failure count.
func (s *AnalyticsStore) GetTopFailingWebhooks(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopFailingEndpoint, error) {
	query := `
		SELECT webhook_host,
			countIf(status = 'failed') AS failed,
			count() AS total,
			CASE WHEN count() > 0
				THEN countIf(status = 'failed') / count()
				ELSE 0
			END AS failure_rate,
			'' AS last_error
		FROM webhook_delivery_events
		WHERE project_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY webhook_host
		HAVING countIf(status = 'failed') > 0
		ORDER BY failed DESC
		LIMIT ?`

	rows, err := s.client.Query(ctx, query, projectID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("clickhouse top failing webhooks: %w", err)
	}
	defer rows.Close()

	result := make([]store.TopFailingEndpoint, 0)
	for rows.Next() {
		var e store.TopFailingEndpoint
		if err := rows.Scan(&e.URL, &e.Failed, &e.Total, &e.FailureRate, &e.LastError); err != nil {
			return nil, fmt.Errorf("clickhouse top failing webhooks scan: %w", err)
		}
		e.URL = redactWebhookAnalyticsURL(e.URL)
		result = append(result, e)
	}
	return result, rows.Err()
}

func redactWebhookAnalyticsURL(rawURL string) string {
	return httputil.RedactURLForLog(rawURL)
}

// Event Analytics.

// GetEventVolume returns event trigger counts by time bucket.
func (s *AnalyticsStore) GetEventVolume(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.EventVolumeBucket, error) {
	truncFn := "toStartOfDay"
	if bucket == "hour" {
		truncFn = "toStartOfHour"
	}

	query := fmt.Sprintf(`
		SELECT %s(created_at) AS period,
			countIf(status = 'created') AS created,
			countIf(status = 'received') AS received,
			countIf(status = 'timed_out') AS timed_out
		FROM event_trigger_events
		WHERE project_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY period
		ORDER BY period`, truncFn)

	rows, err := s.client.Query(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse event volume: %w", err)
	}
	defer rows.Close()

	result := make([]store.EventVolumeBucket, 0)
	for rows.Next() {
		var b store.EventVolumeBucket
		var period time.Time
		if err := rows.Scan(&period, &b.Created, &b.Received, &b.TimedOut); err != nil {
			return nil, fmt.Errorf("clickhouse event volume scan: %w", err)
		}
		b.Period = period.Format(time.RFC3339)
		result = append(result, b)
	}
	return result, rows.Err()
}

// GetEventLatency returns aggregate event wait duration statistics.
func (s *AnalyticsStore) GetEventLatency(ctx context.Context, projectID string, from, to time.Time) (*store.EventLatencyStats, error) {
	query := `
		SELECT
			coalesce(avg(wait_duration_ms), 0) AS avg_ms,
			coalesce(quantile(0.50)(wait_duration_ms), 0) AS p50_ms,
			coalesce(quantile(0.95)(wait_duration_ms), 0) AS p95_ms,
			coalesce(quantile(0.99)(wait_duration_ms), 0) AS p99_ms,
			count() AS count
		FROM event_trigger_events
		WHERE project_id = ? AND created_at >= ? AND created_at < ?
			AND status = 'received' AND wait_duration_ms > 0`

	var stats store.EventLatencyStats
	latencyRow, err := s.client.QueryRow(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse event latency: %w", err)
	}
	if err := latencyRow.Scan(
		&stats.AvgMs, &stats.P50Ms, &stats.P95Ms, &stats.P99Ms, &stats.Count,
	); err != nil {
		return nil, fmt.Errorf("clickhouse event latency: %w", err)
	}
	return &stats, nil
}

// Cost Analytics (new endpoints).

// GetCostForecast projects future costs based on recent daily rate.
func (s *AnalyticsStore) GetCostForecast(ctx context.Context, projectID string, from, to time.Time) (*store.CostForecast, error) {
	query := `
		WITH daily AS (
			SELECT toStartOfDay(created_at) AS day,
				sum(compute_cost_microusd) AS daily_cost
			FROM run_analytics
			WHERE project_id = ? AND created_at >= ? AND created_at < ?
			GROUP BY day
			ORDER BY day
		)
		SELECT
			coalesce(avg(daily_cost), 0) AS daily_rate,
			coalesce(avg(daily_cost) * 30, 0) AS projected_monthly,
			CASE WHEN count() >= 2
				THEN ((last_value(daily_cost) - first_value(daily_cost)) / greatest(first_value(daily_cost), 1)) * 100
				ELSE 0
			END AS trend_pct
		FROM daily`

	var forecast store.CostForecast
	forecastRow, err := s.client.QueryRow(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse cost forecast: %w", err)
	}
	if err := forecastRow.Scan(
		&forecast.DailyRate, &forecast.ProjectedMonthly, &forecast.TrendPct,
	); err != nil {
		return nil, fmt.Errorf("clickhouse cost forecast: %w", err)
	}
	return &forecast, nil
}

// GetCostByTrigger groups cost by trigger type.
func (s *AnalyticsStore) GetCostByTrigger(ctx context.Context, projectID string, from, to time.Time) ([]store.CostByTrigger, error) {
	query := `
		SELECT ra.triggered_by,
			coalesce(sum(ra.compute_cost_microusd), 0) AS cost,
			count() AS run_count,
			0 AS pct
		FROM run_analytics ra
		WHERE ra.project_id = ? AND ra.created_at >= ? AND ra.created_at < ?
		GROUP BY ra.triggered_by
		ORDER BY cost DESC`

	rows, err := s.client.Query(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("clickhouse cost by trigger: %w", err)
	}
	defer rows.Close()

	result := make([]store.CostByTrigger, 0)
	var totalCost int64
	for rows.Next() {
		var c store.CostByTrigger
		if err := rows.Scan(&c.Trigger, &c.Cost, &c.RunCount, &c.Pct); err != nil {
			return nil, fmt.Errorf("clickhouse cost by trigger scan: %w", err)
		}
		totalCost += c.Cost
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse cost by trigger rows: %w", err)
	}

	for i := range result {
		if totalCost > 0 {
			result[i].Pct = float64(result[i].Cost) / float64(totalCost) * 100
		}
	}
	return result, nil
}
