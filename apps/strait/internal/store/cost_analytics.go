package store

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

// CostAnalytics holds aggregated cost data for a project over a time range.
type CostAnalytics struct {
	TotalAICostMicrousd      int64         `json:"total_ai_cost_microusd"`
	TotalComputeCostMicrousd int64         `json:"total_compute_cost_microusd"`
	TotalTokens              int64         `json:"total_tokens"`
	RunCount                 int           `json:"run_count"`
	ByModel                  []CostByModel `json:"by_model"`
	ByJob                    []CostByJob   `json:"by_job"`
}

// CostByModel breaks down AI cost by model.
type CostByModel struct {
	Model        string `json:"model"`
	CostMicrousd int64  `json:"cost_microusd"`
	TotalTokens  int64  `json:"total_tokens"`
	UsageCount   int    `json:"usage_count"`
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
	Period              string `json:"period"`
	AICostMicrousd      int64  `json:"ai_cost_microusd"`
	ComputeCostMicrousd int64  `json:"compute_cost_microusd"`
	TotalTokens         int64  `json:"total_tokens"`
	RunCount            int    `json:"run_count"`
}

// TopCostItem represents a top-cost entity (job, etc).
type TopCostItem struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ItemType     string `json:"item_type"`
	CostMicrousd int64  `json:"cost_microusd"`
	RunCount     int    `json:"run_count"`
}

// ComputeCostAnalytics holds compute cost breakdowns.
type ComputeCostAnalytics struct {
	TotalCostMicrousd int64          `json:"total_cost_microusd"`
	ByPreset          []CostByPreset `json:"by_preset"`
}

// CostByPreset breaks down compute cost by machine preset.
type CostByPreset struct {
	Preset       string  `json:"preset"`
	CostMicrousd int64   `json:"cost_microusd"`
	RunCount     int     `json:"run_count"`
	DurationSecs float64 `json:"duration_secs"`
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
		ByModel: make([]CostByModel, 0),
		ByJob:   make([]CostByJob, 0),
	}

	if isShortPeriod(from, to) {
		return q.getCostAnalyticsLive(ctx, projectID, from, to, result)
	}
	return q.getCostAnalyticsMaterialized(ctx, projectID, from, to, result)
}

func (q *Queries) getCostAnalyticsLive(ctx context.Context, projectID string, from, to time.Time, result *CostAnalytics) (*CostAnalytics, error) {
	// Totals from run_usage (AI cost).
	aiQuery := `
		SELECT COALESCE(SUM(u.cost_microusd), 0),
		       COALESCE(SUM(u.total_tokens), 0),
		       COUNT(DISTINCT jr.id)
		FROM run_usage u
		JOIN job_runs jr ON jr.id = u.run_id
		WHERE jr.project_id = $1 AND jr.created_at >= $2 AND jr.created_at < $3`
	if err := q.db.QueryRow(ctx, aiQuery, projectID, from, to).Scan(
		&result.TotalAICostMicrousd, &result.TotalTokens, &result.RunCount,
	); err != nil {
		return nil, fmt.Errorf("cost analytics ai totals: %w", err)
	}

	// Totals from run_compute_usage (compute cost).
	computeQuery := `
		SELECT COALESCE(SUM(cost_microusd), 0)
		FROM run_compute_usage
		WHERE project_id = $1 AND created_at >= $2 AND created_at < $3`
	if err := q.db.QueryRow(ctx, computeQuery, projectID, from, to).Scan(
		&result.TotalComputeCostMicrousd,
	); err != nil {
		return nil, fmt.Errorf("cost analytics compute totals: %w", err)
	}

	// By model breakdown.
	modelQuery := `
		SELECT u.model,
		       SUM(u.cost_microusd),
		       SUM(u.total_tokens),
		       COUNT(*)
		FROM run_usage u
		JOIN job_runs jr ON jr.id = u.run_id
		WHERE jr.project_id = $1 AND jr.created_at >= $2 AND jr.created_at < $3
		GROUP BY u.model
		ORDER BY SUM(u.cost_microusd) DESC`
	rows, err := q.db.Query(ctx, modelQuery, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("cost analytics by model: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var m CostByModel
		if err := rows.Scan(&m.Model, &m.CostMicrousd, &m.TotalTokens, &m.UsageCount); err != nil {
			return nil, fmt.Errorf("cost analytics by model scan: %w", err)
		}
		result.ByModel = append(result.ByModel, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cost analytics by model rows: %w", err)
	}

	// By job breakdown.
	jobQuery := `
		SELECT jr.job_id,
		       COALESCE(j.slug, jr.job_id),
		       SUM(u.cost_microusd),
		       COUNT(DISTINCT jr.id)
		FROM run_usage u
		JOIN job_runs jr ON jr.id = u.run_id
		LEFT JOIN jobs j ON j.id = jr.job_id
		WHERE jr.project_id = $1 AND jr.created_at >= $2 AND jr.created_at < $3
		GROUP BY jr.job_id, j.slug
		ORDER BY SUM(u.cost_microusd) DESC`
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
		SELECT COALESCE(SUM(ai_cost_microusd), 0),
		       COALESCE(SUM(compute_cost_microusd), 0),
		       COALESCE(SUM(total_tokens), 0),
		       COALESCE(SUM(run_count), 0)
		FROM cost_stats_hourly
		WHERE project_id = $1 AND hour >= $2 AND hour < $3`
	if err := q.db.QueryRow(ctx, totalsQuery, projectID, from, to).Scan(
		&result.TotalAICostMicrousd, &result.TotalComputeCostMicrousd,
		&result.TotalTokens, &result.RunCount,
	); err != nil {
		return nil, fmt.Errorf("materialized cost analytics totals: %w", err)
	}

	// By model and by job breakdowns still use live tables (materialized table
	// does not store per-model or per-job splits).
	modelQuery := `
		SELECT u.model,
		       SUM(u.cost_microusd),
		       SUM(u.total_tokens),
		       COUNT(*)
		FROM run_usage u
		JOIN job_runs jr ON jr.id = u.run_id
		WHERE jr.project_id = $1 AND jr.created_at >= $2 AND jr.created_at < $3
		GROUP BY u.model
		ORDER BY SUM(u.cost_microusd) DESC`
	rows, err := q.db.Query(ctx, modelQuery, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("materialized cost analytics by model: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var m CostByModel
		if err := rows.Scan(&m.Model, &m.CostMicrousd, &m.TotalTokens, &m.UsageCount); err != nil {
			return nil, fmt.Errorf("materialized cost analytics by model scan: %w", err)
		}
		result.ByModel = append(result.ByModel, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("materialized cost analytics by model rows: %w", err)
	}

	jobQuery := `
		SELECT jr.job_id,
		       COALESCE(j.slug, jr.job_id),
		       SUM(u.cost_microusd),
		       COUNT(DISTINCT jr.id)
		FROM run_usage u
		JOIN job_runs jr ON jr.id = u.run_id
		LEFT JOIN jobs j ON j.id = jr.job_id
		WHERE jr.project_id = $1 AND jr.created_at >= $2 AND jr.created_at < $3
		GROUP BY jr.job_id, j.slug
		ORDER BY SUM(u.cost_microusd) DESC`
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
		       COALESCE(SUM(u.cost_microusd), 0),
		       0::BIGINT,
		       COALESCE(SUM(u.total_tokens), 0),
		       COUNT(DISTINCT jr.id)
		FROM run_usage u
		JOIN job_runs jr ON jr.id = u.run_id
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
		if err := rows.Scan(&period, &p.AICostMicrousd, &p.ComputeCostMicrousd, &p.TotalTokens, &p.RunCount); err != nil {
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
		       COALESCE(SUM(ai_cost_microusd), 0),
		       COALESCE(SUM(compute_cost_microusd), 0),
		       COALESCE(SUM(total_tokens), 0),
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
		if err := rows.Scan(&period, &p.AICostMicrousd, &p.ComputeCostMicrousd, &p.TotalTokens, &p.RunCount); err != nil {
			return nil, fmt.Errorf("cost trends materialized scan: %w", err)
		}
		p.Period = period.Format(time.RFC3339)
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetTopCosts returns the top N most expensive jobs by total AI cost.
func (q *Queries) GetTopCosts(ctx context.Context, projectID string, from, to time.Time, limit int) ([]TopCostItem, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetTopCosts")
	defer span.End()

	query := `
		SELECT jr.job_id,
		       COALESCE(j.slug, jr.job_id),
		       SUM(u.cost_microusd),
		       COUNT(DISTINCT jr.id)
		FROM run_usage u
		JOIN job_runs jr ON jr.id = u.run_id
		LEFT JOIN jobs j ON j.id = jr.job_id
		WHERE jr.project_id = $1 AND jr.created_at >= $2 AND jr.created_at < $3
		GROUP BY jr.job_id, j.slug
		ORDER BY SUM(u.cost_microusd) DESC
		LIMIT $4`

	rows, err := q.db.Query(ctx, query, projectID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("top costs: %w", err)
	}
	defer rows.Close()

	items := make([]TopCostItem, 0, limit)
	for rows.Next() {
		var item TopCostItem
		if err := rows.Scan(&item.ID, &item.Name, &item.CostMicrousd, &item.RunCount); err != nil {
			return nil, fmt.Errorf("top costs scan: %w", err)
		}
		item.ItemType = "job"
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetComputeCostAnalytics returns compute costs grouped by machine preset.
func (q *Queries) GetComputeCostAnalytics(ctx context.Context, projectID string, from, to time.Time) (*ComputeCostAnalytics, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetComputeCostAnalytics")
	defer span.End()

	result := &ComputeCostAnalytics{
		ByPreset: make([]CostByPreset, 0),
	}

	totalQuery := `
		SELECT COALESCE(SUM(cost_microusd), 0)
		FROM run_compute_usage
		WHERE project_id = $1 AND created_at >= $2 AND created_at < $3`
	if err := q.db.QueryRow(ctx, totalQuery, projectID, from, to).Scan(&result.TotalCostMicrousd); err != nil {
		return nil, fmt.Errorf("compute cost analytics total: %w", err)
	}

	presetQuery := `
		SELECT machine_preset,
		       SUM(cost_microusd),
		       COUNT(*),
		       COALESCE(SUM(duration_secs), 0)
		FROM run_compute_usage
		WHERE project_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY machine_preset
		ORDER BY SUM(cost_microusd) DESC`

	rows, err := q.db.Query(ctx, presetQuery, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("compute cost analytics by preset: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p CostByPreset
		if err := rows.Scan(&p.Preset, &p.CostMicrousd, &p.RunCount, &p.DurationSecs); err != nil {
			return nil, fmt.Errorf("compute cost analytics by preset scan: %w", err)
		}
		result.ByPreset = append(result.ByPreset, p)
	}
	return result, rows.Err()
}

// AggregateCostStatsHourly materializes cost data for a given hour into cost_stats_hourly.
func (q *Queries) AggregateCostStatsHourly(ctx context.Context, hour time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.AggregateCostStatsHourly")
	defer span.End()

	hour = hour.Truncate(time.Hour)
	nextHour := hour.Add(time.Hour)

	query := `
		INSERT INTO cost_stats_hourly (project_id, hour, ai_cost_microusd, compute_cost_microusd, total_tokens, run_count)
		SELECT
			jr.project_id,
			$1 AS hour,
			COALESCE(SUM(u.cost_microusd), 0) AS ai_cost_microusd,
			COALESCE(cu.compute_cost, 0) AS compute_cost_microusd,
			COALESCE(SUM(u.total_tokens), 0) AS total_tokens,
			COUNT(DISTINCT jr.id) AS run_count
		FROM job_runs jr
		LEFT JOIN run_usage u ON u.run_id = jr.id
		LEFT JOIN LATERAL (
			SELECT SUM(c.cost_microusd) AS compute_cost
			FROM run_compute_usage c
			WHERE c.project_id = jr.project_id AND c.created_at >= $1 AND c.created_at < $2
		) cu ON true
		WHERE jr.created_at >= $1 AND jr.created_at < $2
		  AND jr.status IN ('completed', 'failed', 'timed_out', 'canceled')
		GROUP BY jr.project_id, cu.compute_cost
		ON CONFLICT (project_id, hour) DO UPDATE SET
			ai_cost_microusd = EXCLUDED.ai_cost_microusd,
			compute_cost_microusd = EXCLUDED.compute_cost_microusd,
			total_tokens = EXCLUDED.total_tokens,
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
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetCostOutliers")
	defer span.End()

	query := `
		WITH run_costs AS (
			SELECT
				u.run_id,
				jr.job_id,
				SUM(u.cost_microusd) AS cost_microusd
			FROM run_usage u
			JOIN job_runs jr ON jr.id = u.run_id
			WHERE jr.project_id = $1
			  AND jr.created_at >= $2
			  AND jr.created_at < $3
			GROUP BY u.run_id, jr.job_id
		),
		job_stats AS (
			SELECT
				job_id,
				AVG(cost_microusd) AS avg_cost,
				STDDEV_POP(cost_microusd) AS stddev_cost
			FROM run_costs
			GROUP BY job_id
			HAVING STDDEV_POP(cost_microusd) > 0
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
		WHERE rc.cost_microusd > js.avg_cost + ($4 * js.stddev_cost)
		ORDER BY deviations_above DESC`

	rows, err := q.db.Query(ctx, query, projectID, from, to, threshold)
	if err != nil {
		return nil, fmt.Errorf("get cost outliers: %w", err)
	}
	defer rows.Close()

	outliers := make([]CostOutlier, 0)
	for rows.Next() {
		var o CostOutlier
		if err := rows.Scan(&o.RunID, &o.JobID, &o.CostMicrousd, &o.AvgCostMicrousd, &o.StddevMicrousd, &o.DeviationsAbove); err != nil {
			return nil, fmt.Errorf("get cost outliers scan: %w", err)
		}
		outliers = append(outliers, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get cost outliers rows: %w", err)
	}

	return outliers, nil
}
