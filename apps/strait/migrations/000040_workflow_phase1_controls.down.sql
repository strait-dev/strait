DROP INDEX IF EXISTS idx_workflow_step_approvals_status_expires;
DROP INDEX IF EXISTS idx_workflow_runs_workflow_status_created;

DROP TABLE IF EXISTS workflow_step_approvals;
DROP TABLE IF EXISTS workflow_version_steps;
DROP TABLE IF EXISTS workflow_versions;

ALTER TABLE job_runs
    DROP COLUMN IF EXISTS retry_max_delay_secs,
    DROP COLUMN IF EXISTS retry_initial_delay_secs,
    DROP COLUMN IF EXISTS retry_backoff,
    DROP COLUMN IF EXISTS timeout_secs_override,
    DROP COLUMN IF EXISTS max_attempts_override;

ALTER TABLE workflow_runs
    DROP COLUMN IF EXISTS workflow_version;

ALTER TABLE workflow_steps
    DROP COLUMN IF EXISTS retry_max_delay_secs,
    DROP COLUMN IF EXISTS retry_initial_delay_secs,
    DROP COLUMN IF EXISTS retry_backoff,
    DROP COLUMN IF EXISTS retry_max_attempts,
    DROP COLUMN IF EXISTS approval_approvers,
    DROP COLUMN IF EXISTS approval_timeout_secs,
    DROP COLUMN IF EXISTS step_type;

ALTER TABLE workflows
    DROP COLUMN IF EXISTS max_concurrent_runs,
    DROP COLUMN IF EXISTS timeout_secs;
