package clickhouse

import (
	"context"
	"fmt"
)

// RunEventsTable is the DDL for the run_events ClickHouse table.
const RunEventsTable = `
CREATE TABLE IF NOT EXISTS run_events (
    event_id String,
    run_id String,
    project_id String,
    job_id String,
    event_type LowCardinality(String),
    level LowCardinality(String),
    message String,
    metadata String,
    created_at DateTime64(3),
    inserted_at DateTime64(3) DEFAULT now64(3)
) ENGINE = MergeTree()
PARTITION BY toDate(inserted_at)
ORDER BY (project_id, run_id, created_at)
TTL inserted_at + INTERVAL 90 DAY
`

// RunAnalyticsTable is the DDL for the run_analytics ClickHouse table.
const RunAnalyticsTable = `
CREATE TABLE IF NOT EXISTS run_analytics (
    run_id String,
    job_id String,
    project_id String,
    status LowCardinality(String),
    execution_mode LowCardinality(String),
    machine_preset LowCardinality(String),
    attempt UInt8,
    duration_ms UInt64,
    queue_wait_ms UInt64,
    cost_microusd Int64,
    compute_cost_microusd Int64,
    triggered_by LowCardinality(String),
    created_at DateTime64(3),
    started_at Nullable(DateTime64(3)),
    finished_at Nullable(DateTime64(3)),
    inserted_at DateTime64(3) DEFAULT now64(3)
) ENGINE = MergeTree()
PARTITION BY toDate(inserted_at)
ORDER BY (project_id, job_id, created_at)
TTL inserted_at + INTERVAL 365 DAY
`

// ComputeUsageTable is the DDL for the compute_usage ClickHouse table.
const ComputeUsageTable = `
CREATE TABLE IF NOT EXISTS compute_usage (
    run_id String,
    project_id String,
    machine_preset LowCardinality(String),
    machine_id String,
    duration_secs Float64,
    cost_microusd Int64,
    started_at DateTime64(3),
    finished_at DateTime64(3),
    inserted_at DateTime64(3) DEFAULT now64(3)
) ENGINE = MergeTree()
PARTITION BY toDate(inserted_at)
ORDER BY (project_id, started_at)
TTL inserted_at + INTERVAL 365 DAY
`

// CreateSchema creates all ClickHouse tables. Idempotent (IF NOT EXISTS).
func CreateSchema(ctx context.Context, c *Client) error {
	if c == nil {
		return nil
	}

	tables := []struct {
		name string
		ddl  string
	}{
		{"run_events", RunEventsTable},
		{"run_analytics", RunAnalyticsTable},
		{"compute_usage", ComputeUsageTable},
	}

	for _, t := range tables {
		if err := c.Exec(ctx, t.ddl); err != nil {
			return fmt.Errorf("create table %s: %w", t.name, err)
		}
	}

	return nil
}
