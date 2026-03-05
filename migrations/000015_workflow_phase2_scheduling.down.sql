DROP INDEX IF EXISTS idx_workflows_enabled_cron;

ALTER TABLE workflow_version_steps
    DROP COLUMN IF EXISTS timeout_secs_override;

ALTER TABLE workflow_versions
    DROP COLUMN IF EXISTS skip_if_running,
    DROP COLUMN IF EXISTS cron_timezone,
    DROP COLUMN IF EXISTS cron,
    DROP COLUMN IF EXISTS max_parallel_steps;

ALTER TABLE workflow_runs
    DROP COLUMN IF EXISTS max_parallel_steps;

ALTER TABLE workflow_steps
    DROP COLUMN IF EXISTS timeout_secs_override;

ALTER TABLE workflows
    DROP COLUMN IF EXISTS skip_if_running,
    DROP COLUMN IF EXISTS cron_timezone,
    DROP COLUMN IF EXISTS cron,
    DROP COLUMN IF EXISTS max_parallel_steps;
