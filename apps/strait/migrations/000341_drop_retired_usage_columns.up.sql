-- safety-ok: launch cleanup; application code no longer reads or writes these
-- retired model-usage and agent guardrail columns, and public/API surfaces have
-- schema guards preventing their reintroduction.
ALTER TABLE cost_stats_hourly DROP COLUMN IF EXISTS deprecated_token_count;

-- safety-ok: launch cleanup after the rename migration retired these fields and application
-- code stopped reading or writing the deprecated names.
ALTER TABLE jobs
    DROP COLUMN IF EXISTS deprecated_agent_token_cap,
    DROP COLUMN IF EXISTS deprecated_agent_tool_call_cap,
    DROP COLUMN IF EXISTS deprecated_agent_iteration_cap,
    DROP COLUMN IF EXISTS deprecated_agent_allowed_tool_names,
    DROP COLUMN IF EXISTS deprecated_agent_blocked_tool_names;

-- safety-ok: launch cleanup after the rename migration retired these fields and application
-- code stopped reading or writing the deprecated names.
ALTER TABLE job_versions
    DROP COLUMN IF EXISTS deprecated_agent_token_cap,
    DROP COLUMN IF EXISTS deprecated_agent_tool_call_cap,
    DROP COLUMN IF EXISTS deprecated_agent_iteration_cap,
    DROP COLUMN IF EXISTS deprecated_agent_allowed_tool_names,
    DROP COLUMN IF EXISTS deprecated_agent_blocked_tool_names;

-- safety-ok: launch cleanup after the rename migration retired these fields and application
-- code stopped reading or writing the deprecated names.
ALTER TABLE project_quotas
    DROP COLUMN IF EXISTS deprecated_agent_token_cap,
    DROP COLUMN IF EXISTS deprecated_agent_tool_call_cap,
    DROP COLUMN IF EXISTS deprecated_agent_iteration_cap;
