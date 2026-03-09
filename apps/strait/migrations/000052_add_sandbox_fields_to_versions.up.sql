-- Add sandbox/cancel columns to job_versions so version snapshots preserve them.
ALTER TABLE job_versions ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'http';
ALTER TABLE job_versions ADD COLUMN sandbox_code TEXT;
ALTER TABLE job_versions ADD COLUMN sandbox_language TEXT;
ALTER TABLE job_versions ADD COLUMN cancel_endpoint_url TEXT;

-- Add compensate_step_ref to workflow_version_steps so version snapshots preserve it.
ALTER TABLE workflow_version_steps ADD COLUMN compensate_step_ref TEXT;
