DROP TABLE IF EXISTS run_tool_calls;
DROP TABLE IF EXISTS run_usage;
DROP TABLE IF EXISTS pricing_catalog;

-- safety-ok: pre-launch coordinated rename; application code no longer reads or writes this legacy token column.
ALTER TABLE cost_stats_hourly RENAME COLUMN total_tokens TO deprecated_token_count;
