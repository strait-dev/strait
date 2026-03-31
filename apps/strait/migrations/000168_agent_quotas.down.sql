ALTER TABLE project_quotas
    DROP COLUMN IF EXISTS max_agents,
    DROP COLUMN IF EXISTS max_agent_runs_per_month,
    DROP COLUMN IF EXISTS max_agent_channels;
