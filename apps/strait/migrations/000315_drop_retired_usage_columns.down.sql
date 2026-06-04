ALTER TABLE cost_stats_hourly ADD COLUMN IF NOT EXISTS deprecated_token_count BIGINT NOT NULL DEFAULT 0;

ALTER TABLE jobs ADD COLUMN IF NOT EXISTS deprecated_agent_token_cap BIGINT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS deprecated_agent_tool_call_cap INT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS deprecated_agent_iteration_cap INT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS deprecated_agent_allowed_tool_names TEXT[];
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS deprecated_agent_blocked_tool_names TEXT[];

ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS deprecated_agent_token_cap BIGINT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS deprecated_agent_tool_call_cap INT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS deprecated_agent_iteration_cap INT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS deprecated_agent_allowed_tool_names TEXT[];
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS deprecated_agent_blocked_tool_names TEXT[];

ALTER TABLE project_quotas ADD COLUMN IF NOT EXISTS deprecated_agent_token_cap BIGINT;
ALTER TABLE project_quotas ADD COLUMN IF NOT EXISTS deprecated_agent_tool_call_cap INT;
ALTER TABLE project_quotas ADD COLUMN IF NOT EXISTS deprecated_agent_iteration_cap INT;
