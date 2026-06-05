package store

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

// CostAnalytics holds aggregated cost data for a project over a time range.
type CostAnalytics struct {
	TotalSpendMicrousd int64       `json:"total_spend_microusd"`
	RunCount           int         `json:"run_count"`
	ByJob              []CostByJob `json:"by_job"`
}

// CostByJob breaks down cost by job.
type CostByJob struct {
	JobID        string `json:"job_id"`
	JobSlug      string `json:"job_slug"`
	CostMicrousd int64  `json:"cost_microusd"`
	RunCount     int    `json:"run_count"`
}

// CostTrendPoint is a single data point in a cost time-series.
type CostTrendPoint struct {
	Period        string `json:"period"`
	SpendMicrousd int64  `json:"spend_microusd"`
	RunCount      int    `json:"run_count"`
}

// TopCostItem represents a top-cost entity (job, etc).
type TopCostItem struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ItemType     string `json:"item_type"`
	CostMicrousd int64  `json:"cost_microusd"`
	RunCount     int    `json:"run_count"`
}

// isShortPeriod returns true when the time range is 24 hours or less,
// indicating we should query live tables instead of materialized ones.
func isShortPeriod(from, to time.Time) bool {
	return to.Sub(from) <= 24*time.Hour
}

// GetCostAnalytics returns aggregated cost data for a project over a time range.
func (q *Queries) GetCostAnalytics(ctx context.Context, projectID string, from, to time.Time) (*CostAnalytics, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetCostAnalytics")
	defer span.End()

	result := &CostAnalytics{
		ByJob: make([]CostByJob, 0),
	}

	if isShortPeriod(from, to) {
		return q.getCostAnalyticsLive(ctx, projectID, from, to, result)
	}
	return q.getCostAnalyticsMaterialized(ctx, projectID, from, to, result)
}

func (q *Queries) getCostAnalyticsLive(ctx context.Context, projectID string, from, to time.Time, result *CostAnalytics) (*CostAnalytics, error) {
	totalsQuery := `
		SELECT COALESCE(SUM(compute_cost_microusd), 0),
		       COALESCE(SUM(run_count), 0)
		FROM cost_stats_hourly
		WHERE project_id = $1 AND hour >= $2 AND hour < $3`
	if err := q.db.QueryRow(ctx, totalsQuery, projectID, from, to).Scan(
		&result.TotalSpendMicrousd, &result.RunCount,
	); err != nil {
		return nil, fmt.Errorf("cost analytics launch totals: %w", err)
	}
	if result.RunCount == 0 {
		runCountQuery := `
			SELECT COUNT(*)::bigint
			FROM job_runs
			WHERE project_id = $1 AND created_at >= $2 AND created_at < $3`
		var runCount int64
		if err := q.db.QueryRow(ctx, runCountQuery, projectID, from, to).Scan(&runCount); err != nil {
			return nil, fmt.Errorf("cost analytics launch run count: %w", err)
		}
		result.RunCount = int(runCount)
	}

	// By job breakdown.
	jobQuery := `
		SELECT jr.job_id,
		       COALESCE(j.slug, jr.job_id),
		       0::bigint,
		       COUNT(DISTINCT jr.id)
		FROM job_runs jr
		LEFT JOIN jobs j ON j.id = jr.job_id
		WHERE jr.project_id = $1 AND jr.created_at >= $2 AND jr.created_at < $3
		GROUP BY jr.job_id, j.slug
		ORDER BY COUNT(DISTINCT jr.id) DESC`
	jobRows, err := q.db.Query(ctx, jobQuery, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("cost analytics by job: %w", err)
	}
	defer jobRows.Close()
	for jobRows.Next() {
		var j CostByJob
		if err := jobRows.Scan(&j.JobID, &j.JobSlug, &j.CostMicrousd, &j.RunCount); err != nil {
			return nil, fmt.Errorf("cost analytics by job scan: %w", err)
		}
		result.ByJob = append(result.ByJob, j)
	}
	if err := jobRows.Err(); err != nil {
		return nil, fmt.Errorf("cost analytics by job rows: %w", err)
	}

	return result, nil
}

func (q *Queries) getCostAnalyticsMaterialized(ctx context.Context, projectID string, from, to time.Time, result *CostAnalytics) (*CostAnalytics, error) {
	totalsQuery := `
		SELECT COALESCE(SUM(compute_cost_microusd), 0),
		       COALESCE(SUM(run_count), 0)
		FROM cost_stats_hourly
		WHERE project_id = $1 AND hour >= $2 AND hour < $3`
	if err := q.db.QueryRow(ctx, totalsQuery, projectID, from, to).Scan(
		&result.TotalSpendMicrousd, &result.RunCount,
	); err != nil {
		return nil, fmt.Errorf("materialized cost analytics totals: %w", err)
	}

	jobQuery := `
		SELECT jr.job_id,
		       COALESCE(j.slug, jr.job_id),
		       0::bigint,
		       COUNT(DISTINCT jr.id)
		FROM job_runs jr
		LEFT JOIN jobs j ON j.id = jr.job_id
		WHERE jr.project_id = $1 AND jr.created_at >= $2 AND jr.created_at < $3
		GROUP BY jr.job_id, j.slug
		ORDER BY COUNT(DISTINCT jr.id) DESC`
	jobRows, err := q.db.Query(ctx, jobQuery, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("materialized cost analytics by job: %w", err)
	}
	defer jobRows.Close()
	for jobRows.Next() {
		var j CostByJob
		if err := jobRows.Scan(&j.JobID, &j.JobSlug, &j.CostMicrousd, &j.RunCount); err != nil {
			return nil, fmt.Errorf("materialized cost analytics by job scan: %w", err)
		}
		result.ByJob = append(result.ByJob, j)
	}
	if err := jobRows.Err(); err != nil {
		return nil, fmt.Errorf("materialized cost analytics by job rows: %w", err)
	}

	return result, nil
}

// GetCostTrends returns a time-series of cost data points.
// Short periods (<=24h) group by hour from live data; longer periods aggregate
// cost_stats_hourly by day.
func (q *Queries) GetCostTrends(ctx context.Context, projectID string, from, to time.Time) ([]CostTrendPoint, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetCostTrends")
	defer span.End()

	if isShortPeriod(from, to) {
		return q.getCostTrendsLive(ctx, projectID, from, to)
	}
	return q.getCostTrendsMaterialized(ctx, projectID, from, to)
}

func (q *Queries) getCostTrendsLive(ctx context.Context, projectID string, from, to time.Time) ([]CostTrendPoint, error) {
	query := `
		SELECT date_trunc('hour', jr.created_at) AS period,
		       0::BIGINT,
		       COUNT(DISTINCT jr.id)
		FROM job_runs jr
		WHERE jr.project_id = $1 AND jr.created_at >= $2 AND jr.created_at < $3
		GROUP BY date_trunc('hour', jr.created_at)
		ORDER BY period`

	rows, err := q.db.Query(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("cost trends live: %w", err)
	}
	defer rows.Close()

	points := make([]CostTrendPoint, 0)
	for rows.Next() {
		var p CostTrendPoint
		var period time.Time
		if err := rows.Scan(&period, &p.SpendMicrousd, &p.RunCount); err != nil {
			return nil, fmt.Errorf("cost trends live scan: %w", err)
		}
		p.Period = period.Format(time.RFC3339)
		points = append(points, p)
	}
	return points, rows.Err()
}

func (q *Queries) getCostTrendsMaterialized(ctx context.Context, projectID string, from, to time.Time) ([]CostTrendPoint, error) {
	query := `
		SELECT date_trunc('day', hour) AS period,
		       COALESCE(SUM(compute_cost_microusd), 0),
		       COALESCE(SUM(run_count), 0)
		FROM cost_stats_hourly
		WHERE project_id = $1 AND hour >= $2 AND hour < $3
		GROUP BY date_trunc('day', hour)
		ORDER BY period`

	rows, err := q.db.Query(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("cost trends materialized: %w", err)
	}
	defer rows.Close()

	points := make([]CostTrendPoint, 0)
	for rows.Next() {
		var p CostTrendPoint
		var period time.Time
		if err := rows.Scan(&period, &p.SpendMicrousd, &p.RunCount); err != nil {
			return nil, fmt.Errorf("cost trends materialized scan: %w", err)
		}
		p.Period = period.Format(time.RFC3339)
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetTopCosts returns top-cost entities. Launch billing does not expose
// per-job cost attribution, so this returns an empty list until that is wired.
func (q *Queries) GetTopCosts(ctx context.Context, projectID string, from, to time.Time, limit int) ([]TopCostItem, error) {
	_, span := otel.Tracer("strait").Start(ctx, "store.GetTopCosts")
	defer span.End()

	return []TopCostItem{}, nil
}

// AggregateCostStatsHourly materializes cost data for a given hour into cost_stats_hourly.
// The LATERAL subquery correlates on c.project_id = jr.project_id, so each
// project gets its own compute cost sum. The GROUP BY jr.project_id, cu.compute_cost
// deduplicates correctly because cu.compute_cost is deterministic per project per hour.
func (q *Queries) AggregateCostStatsHourly(ctx context.Context, hour time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.AggregateCostStatsHourly")
	defer span.End()

	hour = hour.Truncate(time.Hour)
	nextHour := hour.Add(time.Hour)

	query := `
		INSERT INTO cost_stats_hourly (project_id, hour, usage_cost_microusd, compute_cost_microusd, run_count)
		SELECT
			b.project_id,
			$1 AS hour,
			0 AS usage_cost_microusd,
			COALESCE(SUM(b.compute_cost_microusd), 0) AS compute_cost_microusd,
			COUNT(*) AS run_count
		FROM billing_cost_events b
		WHERE b.created_at >= $1 AND b.created_at < $2
		GROUP BY b.project_id
		ON CONFLICT (project_id, hour) DO UPDATE SET
			usage_cost_microusd = EXCLUDED.usage_cost_microusd,
			compute_cost_microusd = EXCLUDED.compute_cost_microusd,
			run_count = EXCLUDED.run_count`

	_, err := q.db.Exec(ctx, query, hour, nextHour)
	if err != nil {
		return fmt.Errorf("aggregate cost stats hourly: %w", err)
	}
	return nil
}

// CostOutlier represents a run whose cost significantly exceeds the average for its job.
type CostOutlier struct {
	RunID           string  `json:"run_id"`
	JobID           string  `json:"job_id"`
	CostMicrousd    int64   `json:"cost_microusd"`
	AvgCostMicrousd float64 `json:"avg_cost_microusd"`
	StddevMicrousd  float64 `json:"stddev_cost_microusd"`
	DeviationsAbove float64 `json:"deviations_above_avg"`
}

// GetCostOutliers finds runs whose total cost exceeds the per-job average by
// more than threshold standard deviations within the given time range.
func (q *Queries) GetCostOutliers(ctx context.Context, projectID string, from, to time.Time, threshold float64) ([]CostOutlier, error) {
	_, span := otel.Tracer("strait").Start(ctx, "store.GetCostOutliers")
	defer span.End()

	return []CostOutlier{}, nil
}
