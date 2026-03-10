ALTER TABLE workflows
    ADD COLUMN max_parallel_steps INT NOT NULL DEFAULT 0,
    ADD COLUMN cron TEXT,
    ADD COLUMN cron_timezone TEXT,
    ADD COLUMN skip_if_running BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE workflow_steps
    ADD COLUMN timeout_secs_override INT NOT NULL DEFAULT 0;

ALTER TABLE workflow_runs
    ADD COLUMN max_parallel_steps INT NOT NULL DEFAULT 0;

ALTER TABLE workflow_versions
    ADD COLUMN max_parallel_steps INT NOT NULL DEFAULT 0,
    ADD COLUMN cron TEXT,
    ADD COLUMN cron_timezone TEXT,
    ADD COLUMN skip_if_running BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE workflow_version_steps
    ADD COLUMN timeout_secs_override INT NOT NULL DEFAULT 0;

CREATE INDEX idx_workflows_enabled_cron ON workflows(enabled, cron) WHERE cron IS NOT NULL AND cron <> '';
