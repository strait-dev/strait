-- Add is_rollback flag to job_runs so the UI can distinguish runs that used a
-- rolled-back deployment from normal runs.
ALTER TABLE job_runs
    ADD COLUMN IF NOT EXISTS is_rollback BOOL NOT NULL DEFAULT FALSE;

-- Record which deployment was displaced when a rollback is performed on a job.
-- Cleared automatically when a new successful build activates a fresh deployment.
ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS rollback_source_deployment_id TEXT
    REFERENCES code_deployments(id) ON DELETE SET NULL;
