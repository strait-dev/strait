-- Remove compute metering table and its budget limit column now that
-- managed-container execution has been replaced by orchestration-only mode.
DROP TABLE IF EXISTS run_compute_usage;

ALTER TABLE project_quotas DROP COLUMN IF EXISTS compute_daily_cost_limit_microusd;
