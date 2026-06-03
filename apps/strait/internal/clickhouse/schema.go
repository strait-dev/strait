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
    message_class LowCardinality(String),
    metadata_redacted String DEFAULT '{}',
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
    attempt UInt8,
    duration_ms UInt64,
    queue_wait_ms UInt64,
    cost_microusd Int64,
    compute_cost_microusd Int64,
    triggered_by LowCardinality(String),
    tags String DEFAULT '{}',
    job_version_id String DEFAULT '',
    created_at DateTime64(3),
    started_at Nullable(DateTime64(3)),
    finished_at Nullable(DateTime64(3)),
    inserted_at DateTime64(3) DEFAULT now64(3)
) ENGINE = MergeTree()
PARTITION BY toDate(inserted_at)
ORDER BY (project_id, job_id, created_at)
TTL inserted_at + INTERVAL 365 DAY
`

// WorkflowApprovalEventsTable is the DDL for the workflow_approval_events ClickHouse table.
const WorkflowApprovalEventsTable = `
CREATE TABLE IF NOT EXISTS workflow_approval_events (
    approval_id String,
    workflow_run_id String,
    step_run_id String,
    project_id String,
    status LowCardinality(String),
    requested_at DateTime64(3),
    approved_at Nullable(DateTime64(3)),
    inserted_at DateTime64(3) DEFAULT now64(3)
) ENGINE = MergeTree()
PARTITION BY toDate(inserted_at)
ORDER BY (project_id, requested_at)
TTL inserted_at + INTERVAL 365 DAY
`

// JobMetadataTable is the DDL for the job_metadata ClickHouse table.
const JobMetadataTable = `
CREATE TABLE IF NOT EXISTS job_metadata (
    job_id String,
    project_id String,
    slug String,
    inserted_at DateTime64(3) DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(inserted_at)
ORDER BY (project_id, job_id)
`

// EventTriggerEventsTable is the DDL for the event_trigger_events ClickHouse table.
const EventTriggerEventsTable = `
CREATE TABLE IF NOT EXISTS event_trigger_events (
    trigger_id String,
    event_key String,
    project_id String,
    source_type LowCardinality(String),
    status LowCardinality(String),
    timeout_secs UInt32,
    wait_duration_ms UInt64,
    created_at DateTime64(3),
    received_at Nullable(DateTime64(3)),
    inserted_at DateTime64(3) DEFAULT now64(3)
) ENGINE = MergeTree()
PARTITION BY toDate(inserted_at)
ORDER BY (project_id, event_key, created_at)
TTL inserted_at + INTERVAL 365 DAY
`

// WorkflowRunAnalyticsTable is the DDL for the workflow_run_analytics ClickHouse table.
const WorkflowRunAnalyticsTable = `
CREATE TABLE IF NOT EXISTS workflow_run_analytics (
    workflow_run_id String,
    workflow_id String,
    project_id String,
    status LowCardinality(String),
    triggered_by LowCardinality(String),
    step_count UInt16,
    duration_ms UInt64,
    created_at DateTime64(3),
    started_at Nullable(DateTime64(3)),
    finished_at Nullable(DateTime64(3)),
    inserted_at DateTime64(3) DEFAULT now64(3)
) ENGINE = MergeTree()
PARTITION BY toDate(inserted_at)
ORDER BY (project_id, workflow_id, created_at)
TTL inserted_at + INTERVAL 365 DAY
`

// WorkflowStepAnalyticsTable is the DDL for the workflow_step_analytics ClickHouse table.
const WorkflowStepAnalyticsTable = `
CREATE TABLE IF NOT EXISTS workflow_step_analytics (
    step_run_id String,
    workflow_run_id String,
    workflow_id String,
    project_id String,
    step_ref String,
    status LowCardinality(String),
    duration_ms UInt64,
    attempt UInt8,
    error String DEFAULT '',
    created_at DateTime64(3),
    started_at Nullable(DateTime64(3)),
    finished_at Nullable(DateTime64(3)),
    inserted_at DateTime64(3) DEFAULT now64(3)
) ENGINE = MergeTree()
PARTITION BY toDate(inserted_at)
ORDER BY (project_id, workflow_id, workflow_run_id, created_at)
TTL inserted_at + INTERVAL 365 DAY
`

// WebhookDeliveryEventsTable is the DDL for the webhook_delivery_events ClickHouse table.
const WebhookDeliveryEventsTable = `
CREATE TABLE IF NOT EXISTS webhook_delivery_events (
    delivery_id String,
    run_id String,
    job_id String,
    project_id String,
    webhook_host String,
    status LowCardinality(String),
    attempts UInt8,
    last_status_code UInt16,
    duration_ms UInt64,
    event_type LowCardinality(String),
    created_at DateTime64(3),
    delivered_at Nullable(DateTime64(3)),
    inserted_at DateTime64(3) DEFAULT now64(3)
) ENGINE = MergeTree()
PARTITION BY toDate(inserted_at)
ORDER BY (project_id, webhook_host, created_at)
TTL inserted_at + INTERVAL 365 DAY
`

// RunStatsDailyTable is the DDL for the pre-aggregated daily run stats table.
const RunStatsDailyTable = `
CREATE TABLE IF NOT EXISTS run_stats_daily (
    project_id String,
    job_id String,
    day Date,
    total UInt64,
    completed UInt64,
    failed UInt64,
    timed_out UInt64,
    avg_duration_ms Float64,
    total_cost_microusd Int64,
    inserted_at DateTime64(3) DEFAULT now64(3)
) ENGINE = SummingMergeTree((total, completed, failed, timed_out, total_cost_microusd))
PARTITION BY toYYYYMM(day)
ORDER BY (project_id, job_id, day)
TTL day + INTERVAL 365 DAY
`

// RunStatsDailyMV is the materialized view that populates run_stats_daily from run_analytics.
const RunStatsDailyMV = `
CREATE MATERIALIZED VIEW IF NOT EXISTS run_stats_daily_mv
TO run_stats_daily AS
SELECT
    project_id,
    job_id,
    toDate(created_at) AS day,
    count() AS total,
    countIf(status = 'completed') AS completed,
    countIf(status = 'failed') AS failed,
    countIf(status = 'timed_out') AS timed_out,
    avg(duration_ms) AS avg_duration_ms,
    sum(cost_microusd) AS total_cost_microusd
FROM run_analytics
GROUP BY project_id, job_id, day
`

// CostDailyTable is the DDL for the pre-aggregated daily cost table.
const CostDailyTable = `
CREATE TABLE IF NOT EXISTS cost_daily (
    project_id String,
    day Date,
    usage_cost_microusd Int64,
    compute_cost_microusd Int64,
    run_count UInt64,
    inserted_at DateTime64(3) DEFAULT now64(3)
) ENGINE = SummingMergeTree((usage_cost_microusd, compute_cost_microusd, run_count))
PARTITION BY toYYYYMM(day)
ORDER BY (project_id, day)
TTL day + INTERVAL 365 DAY
`

// CostDailyMV is the materialized view that populates cost_daily from run_analytics.
const CostDailyMV = `
CREATE MATERIALIZED VIEW IF NOT EXISTS cost_daily_mv
TO cost_daily AS
SELECT
    project_id,
    toDate(created_at) AS day,
    0 AS usage_cost_microusd,
    sum(compute_cost_microusd) AS compute_cost_microusd,
    count() AS run_count
FROM run_analytics
GROUP BY project_id, day
`

// schemaAlterations contains ALTER TABLE statements for adding columns on
// existing tables. Each statement must be idempotent (ADD COLUMN IF NOT EXISTS).
var schemaAlterations = []struct {
	table string
	ddl   string
}{
	{
		"run_analytics",
		"ALTER TABLE run_analytics ADD COLUMN IF NOT EXISTS tags String DEFAULT '{}'",
	},
	{
		"run_analytics",
		"ALTER TABLE run_analytics ADD COLUMN IF NOT EXISTS job_version_id String DEFAULT ''",
	},
	{
		"run_events",
		"ALTER TABLE run_events ADD COLUMN IF NOT EXISTS message_class LowCardinality(String) DEFAULT ''",
	},
	{
		"run_events",
		"ALTER TABLE run_events ADD COLUMN IF NOT EXISTS metadata_redacted String DEFAULT '{}'",
	},
	{
		"webhook_delivery_events",
		"ALTER TABLE webhook_delivery_events ADD COLUMN IF NOT EXISTS webhook_host String DEFAULT ''",
	},
}

// BillingEventsTable is the DDL for the billing_events ClickHouse table.
const BillingEventsTable = `
CREATE TABLE IF NOT EXISTS billing_events (
    timestamp  DateTime64(3, 'UTC'),
    org_id     String,
    project_id String DEFAULT '',
    event_type LowCardinality(String),
    feature    LowCardinality(String) DEFAULT '',
    plan_tier  LowCardinality(String),
    details    String DEFAULT '{}'
) ENGINE = MergeTree()
ORDER BY (org_id, event_type, timestamp)
TTL toDateTime(timestamp) + INTERVAL 90 DAY
`

// CreateSchema creates all ClickHouse tables. Idempotent (IF NOT EXISTS).
// It also applies schema alterations for columns added after initial table creation.
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
		{"workflow_approval_events", WorkflowApprovalEventsTable},
		{"job_metadata", JobMetadataTable},
		{"webhook_delivery_events", WebhookDeliveryEventsTable},
		{"workflow_run_analytics", WorkflowRunAnalyticsTable},
		{"workflow_step_analytics", WorkflowStepAnalyticsTable},
		{"event_trigger_events", EventTriggerEventsTable},
		{"run_stats_daily", RunStatsDailyTable},
		{"cost_daily", CostDailyTable},
		{"billing_events", BillingEventsTable},
	}

	for _, t := range tables {
		if err := c.Exec(ctx, t.ddl); err != nil {
			return fmt.Errorf("create table %s: %w", t.name, err)
		}
	}

	// Apply column additions to existing tables.
	for _, alt := range schemaAlterations {
		if err := c.Exec(ctx, alt.ddl); err != nil {
			return fmt.Errorf("alter table %s: %w", alt.table, err)
		}
	}

	// Create materialized views (must come after tables they write to).
	views := []struct {
		name string
		ddl  string
	}{
		{"run_stats_daily_mv", RunStatsDailyMV},
		{"cost_daily_mv", CostDailyMV},
	}
	for _, v := range views {
		if err := c.Exec(ctx, v.ddl); err != nil {
			return fmt.Errorf("create materialized view %s: %w", v.name, err)
		}
	}

	return nil
}
