package clickhouse

import (
	"context"
	"fmt"
	"time"
)

// AgentRunTimelinePoint represents a single time bucket in the agent run timeline.
type AgentRunTimelinePoint struct {
	Bucket    string  `json:"bucket"`
	Total     uint64  `json:"total"`
	Completed uint64  `json:"completed"`
	Failed    uint64  `json:"failed"`
	AvgMs     float64 `json:"avg_duration_ms"`
}

// AgentCostSummary is the aggregate cost data for an agent over a time range.
type AgentCostSummary struct {
	TotalRuns         uint64  `json:"total_runs"`
	TotalTokens       uint64  `json:"total_tokens"`
	PromptTokens      uint64  `json:"prompt_tokens"`
	CompletionTokens  uint64  `json:"completion_tokens"`
	TotalCostMicrousd int64   `json:"total_cost_microusd"`
	AvgCostMicrousd   float64 `json:"avg_cost_microusd"`
	ToolCallCount     uint64  `json:"tool_call_count"`
	CheckpointCount   uint64  `json:"checkpoint_count"`
}

// AgentModelBreakdownRow shows cost and usage per model for an agent.
type AgentModelBreakdownRow struct {
	Model        string `json:"model"`
	Provider     string `json:"provider"`
	Runs         uint64 `json:"runs"`
	TotalTokens  uint64 `json:"total_tokens"`
	CostMicrousd int64  `json:"cost_microusd"`
}

// AgentRankingRow represents an agent in the top-agents ranking.
type AgentRankingRow struct {
	AgentID       string  `json:"agent_id"`
	AgentSlug     string  `json:"agent_slug"`
	Runs          uint64  `json:"runs"`
	CostMicrousd  int64   `json:"cost_microusd"`
	TotalTokens   uint64  `json:"total_tokens"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
}

// GetAgentRunTimeline returns run counts bucketed by time for a specific agent.
func (s *AnalyticsStore) GetAgentRunTimeline(ctx context.Context, projectID, agentID string, from, to time.Time, bucket string) ([]AgentRunTimelinePoint, error) {
	truncFn := "toDate"
	if bucket == "hour" {
		truncFn = "toStartOfHour"
	}

	query := fmt.Sprintf(`
		SELECT
			toString(%s(created_at)) AS bucket,
			count() AS total,
			countIf(status = 'completed') AS completed,
			countIf(status IN ('failed', 'system_failed')) AS failed,
			avg(duration_ms) AS avg_ms
		FROM agent_run_analytics
		WHERE project_id = ? AND created_at >= ? AND created_at <= ?
	`, truncFn)

	if agentID != "" {
		query += " AND agent_id = ?"
	}
	query += " GROUP BY bucket ORDER BY bucket"

	args := []any{projectID, from, to}
	if agentID != "" {
		args = append(args, agentID)
	}

	rows, err := s.client.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("agent run timeline: %w", err)
	}
	defer rows.Close()

	var result []AgentRunTimelinePoint
	for rows.Next() {
		var p AgentRunTimelinePoint
		if err := rows.Scan(&p.Bucket, &p.Total, &p.Completed, &p.Failed, &p.AvgMs); err != nil {
			return nil, fmt.Errorf("scan agent run timeline: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("agent run timeline rows: %w", err)
	}
	return result, nil
}

// GetAgentCostSummary returns aggregate cost and usage data for an agent.
func (s *AnalyticsStore) GetAgentCostSummary(ctx context.Context, projectID, agentID string, from, to time.Time) (*AgentCostSummary, error) {
	query := `
		SELECT
			count() AS total_runs,
			sum(total_tokens) AS total_tokens,
			sum(prompt_tokens) AS prompt_tokens,
			sum(completion_tokens) AS completion_tokens,
			sum(cost_microusd) AS total_cost,
			avg(cost_microusd) AS avg_cost,
			sum(tool_call_count) AS tool_calls,
			sum(checkpoint_count) AS checkpoints
		FROM agent_run_analytics
		WHERE project_id = ? AND agent_id = ? AND created_at >= ? AND created_at <= ?
	`

	row, err := s.client.QueryRow(ctx, query, projectID, agentID, from, to)
	if err != nil {
		return nil, fmt.Errorf("agent cost summary: %w", err)
	}
	var r AgentCostSummary
	if err := row.Scan(&r.TotalRuns, &r.TotalTokens, &r.PromptTokens, &r.CompletionTokens, &r.TotalCostMicrousd, &r.AvgCostMicrousd, &r.ToolCallCount, &r.CheckpointCount); err != nil {
		return nil, fmt.Errorf("agent cost summary: %w", err)
	}
	return &r, nil
}

// GetAgentModelBreakdown returns per-model cost and usage for an agent.
func (s *AnalyticsStore) GetAgentModelBreakdown(ctx context.Context, projectID, agentID string, from, to time.Time) ([]AgentModelBreakdownRow, error) {
	query := `
		SELECT
			model,
			provider,
			count() AS runs,
			sum(total_tokens) AS total_tokens,
			sum(cost_microusd) AS cost_microusd
		FROM agent_run_analytics
		WHERE project_id = ? AND agent_id = ? AND created_at >= ? AND created_at <= ?
			AND model != ''
		GROUP BY model, provider
		ORDER BY cost_microusd DESC
	`

	rows, err := s.client.Query(ctx, query, projectID, agentID, from, to)
	if err != nil {
		return nil, fmt.Errorf("agent model breakdown: %w", err)
	}
	defer rows.Close()

	var result []AgentModelBreakdownRow
	for rows.Next() {
		var r AgentModelBreakdownRow
		if err := rows.Scan(&r.Model, &r.Provider, &r.Runs, &r.TotalTokens, &r.CostMicrousd); err != nil {
			return nil, fmt.Errorf("scan agent model breakdown: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("agent model breakdown rows: %w", err)
	}
	return result, nil
}

// GetAgentTopAgents returns the top agents by cost or run count.
func (s *AnalyticsStore) GetAgentTopAgents(ctx context.Context, projectID string, from, to time.Time, limit int) ([]AgentRankingRow, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT
			agent_id,
			agent_slug,
			count() AS runs,
			sum(cost_microusd) AS cost_microusd,
			sum(total_tokens) AS total_tokens,
			avg(duration_ms) AS avg_duration_ms
		FROM agent_run_analytics
		WHERE project_id = ? AND created_at >= ? AND created_at <= ?
		GROUP BY agent_id, agent_slug
		ORDER BY cost_microusd DESC
		LIMIT ?
	`

	rows, err := s.client.Query(ctx, query, projectID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("agent top agents: %w", err)
	}
	defer rows.Close()

	var result []AgentRankingRow
	for rows.Next() {
		var r AgentRankingRow
		if err := rows.Scan(&r.AgentID, &r.AgentSlug, &r.Runs, &r.CostMicrousd, &r.TotalTokens, &r.AvgDurationMs); err != nil {
			return nil, fmt.Errorf("scan agent top agents: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("agent top agents rows: %w", err)
	}
	return result, nil
}


// AgentDailyCost represents a single day's cost for an agent.
type AgentDailyCost struct {
	AgentID      string    `json:"agent_id"`
	AgentSlug    string    `json:"agent_slug"`
	Date         time.Time `json:"date"`
	CostMicrousd int64     `json:"cost_microusd"`
}

// QueryAgentDailyCosts returns per-agent daily cost totals for the given project and time window.
func (s *AnalyticsStore) QueryAgentDailyCosts(ctx context.Context, projectID string, from, to time.Time) ([]AgentDailyCost, error) {
	query := `
		SELECT
			agent_id,
			agent_slug,
			toDate(created_at) AS day,
			sum(cost_microusd) AS daily_cost
		FROM agent_run_analytics
		WHERE project_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY agent_id, agent_slug, day
		ORDER BY agent_id, day`

	rows, err := s.client.Query(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query agent daily costs: %w", err)
	}
	defer rows.Close()

	var results []AgentDailyCost
	for rows.Next() {
		var r AgentDailyCost
		if err := rows.Scan(&r.AgentID, &r.AgentSlug, &r.Date, &r.CostMicrousd); err != nil {
			return nil, fmt.Errorf("scan agent daily cost: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("agent daily costs rows: %w", err)
	}
	return results, nil
}