DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'deprecated_agent_token_cap')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'max_tokens_per_run') THEN
        ALTER TABLE jobs RENAME COLUMN deprecated_agent_token_cap TO max_tokens_per_run;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'deprecated_agent_tool_call_cap')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'max_tool_calls_per_run') THEN
        ALTER TABLE jobs RENAME COLUMN deprecated_agent_tool_call_cap TO max_tool_calls_per_run;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'deprecated_agent_iteration_cap')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'max_iterations_per_run') THEN
        ALTER TABLE jobs RENAME COLUMN deprecated_agent_iteration_cap TO max_iterations_per_run;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'deprecated_agent_allowed_tool_names')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'allowed_tools') THEN
        ALTER TABLE jobs RENAME COLUMN deprecated_agent_allowed_tool_names TO allowed_tools;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'deprecated_agent_blocked_tool_names')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'jobs' AND column_name = 'blocked_tools') THEN
        ALTER TABLE jobs RENAME COLUMN deprecated_agent_blocked_tool_names TO blocked_tools;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'deprecated_agent_token_cap')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'max_tokens_per_run') THEN
        ALTER TABLE job_versions RENAME COLUMN deprecated_agent_token_cap TO max_tokens_per_run;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'deprecated_agent_tool_call_cap')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'max_tool_calls_per_run') THEN
        ALTER TABLE job_versions RENAME COLUMN deprecated_agent_tool_call_cap TO max_tool_calls_per_run;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'deprecated_agent_iteration_cap')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'max_iterations_per_run') THEN
        ALTER TABLE job_versions RENAME COLUMN deprecated_agent_iteration_cap TO max_iterations_per_run;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'deprecated_agent_allowed_tool_names')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'allowed_tools') THEN
        ALTER TABLE job_versions RENAME COLUMN deprecated_agent_allowed_tool_names TO allowed_tools;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'deprecated_agent_blocked_tool_names')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'job_versions' AND column_name = 'blocked_tools') THEN
        ALTER TABLE job_versions RENAME COLUMN deprecated_agent_blocked_tool_names TO blocked_tools;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'project_quotas' AND column_name = 'deprecated_agent_token_cap')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'project_quotas' AND column_name = 'max_tokens_per_run') THEN
        ALTER TABLE project_quotas RENAME COLUMN deprecated_agent_token_cap TO max_tokens_per_run;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'project_quotas' AND column_name = 'deprecated_agent_tool_call_cap')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'project_quotas' AND column_name = 'max_tool_calls_per_run') THEN
        ALTER TABLE project_quotas RENAME COLUMN deprecated_agent_tool_call_cap TO max_tool_calls_per_run;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'project_quotas' AND column_name = 'deprecated_agent_iteration_cap')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'project_quotas' AND column_name = 'max_iterations_per_run') THEN
        ALTER TABLE project_quotas RENAME COLUMN deprecated_agent_iteration_cap TO max_iterations_per_run;
    END IF;
END $$;
