-- Retire legacy agent guardrail columns without dropping them in the same
-- release. Application code no longer reads these fields; the deprecated names
-- make accidental reuse visible while preserving rolling-deploy safety.

-- safety-ok: launch branch cleanup; application code no longer reads or writes
-- these legacy agent guardrail columns, and this migration only renames them to
-- deprecated names so the old schema data can be removed in a later release.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'max_tokens_per_run')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'deprecated_agent_token_cap') THEN
        ALTER TABLE jobs RENAME COLUMN max_tokens_per_run TO deprecated_agent_token_cap;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'max_tool_calls_per_run')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'deprecated_agent_tool_call_cap') THEN
        ALTER TABLE jobs RENAME COLUMN max_tool_calls_per_run TO deprecated_agent_tool_call_cap;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'max_iterations_per_run')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'deprecated_agent_iteration_cap') THEN
        ALTER TABLE jobs RENAME COLUMN max_iterations_per_run TO deprecated_agent_iteration_cap;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'allowed_tools')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'deprecated_agent_allowed_tool_names') THEN
        ALTER TABLE jobs RENAME COLUMN allowed_tools TO deprecated_agent_allowed_tool_names;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'blocked_tools')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'deprecated_agent_blocked_tool_names') THEN
        ALTER TABLE jobs RENAME COLUMN blocked_tools TO deprecated_agent_blocked_tool_names;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'max_tokens_per_run')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'deprecated_agent_token_cap') THEN
        ALTER TABLE job_versions RENAME COLUMN max_tokens_per_run TO deprecated_agent_token_cap;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'max_tool_calls_per_run')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'deprecated_agent_tool_call_cap') THEN
        ALTER TABLE job_versions RENAME COLUMN max_tool_calls_per_run TO deprecated_agent_tool_call_cap;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'max_iterations_per_run')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'deprecated_agent_iteration_cap') THEN
        ALTER TABLE job_versions RENAME COLUMN max_iterations_per_run TO deprecated_agent_iteration_cap;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'allowed_tools')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'deprecated_agent_allowed_tool_names') THEN
        ALTER TABLE job_versions RENAME COLUMN allowed_tools TO deprecated_agent_allowed_tool_names;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'blocked_tools')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'deprecated_agent_blocked_tool_names') THEN
        ALTER TABLE job_versions RENAME COLUMN blocked_tools TO deprecated_agent_blocked_tool_names;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'project_quotas' AND column_name = 'max_tokens_per_run')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'project_quotas' AND column_name = 'deprecated_agent_token_cap') THEN
        ALTER TABLE project_quotas RENAME COLUMN max_tokens_per_run TO deprecated_agent_token_cap;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'project_quotas' AND column_name = 'max_tool_calls_per_run')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'project_quotas' AND column_name = 'deprecated_agent_tool_call_cap') THEN
        ALTER TABLE project_quotas RENAME COLUMN max_tool_calls_per_run TO deprecated_agent_tool_call_cap;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'project_quotas' AND column_name = 'max_iterations_per_run')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'project_quotas' AND column_name = 'deprecated_agent_iteration_cap') THEN
        ALTER TABLE project_quotas RENAME COLUMN max_iterations_per_run TO deprecated_agent_iteration_cap;
    END IF;
END $$;
