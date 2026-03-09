ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS compensate_step_ref;

ALTER TABLE job_versions DROP COLUMN IF EXISTS cancel_endpoint_url;
ALTER TABLE job_versions DROP COLUMN IF EXISTS sandbox_language;
ALTER TABLE job_versions DROP COLUMN IF EXISTS sandbox_code;
ALTER TABLE job_versions DROP COLUMN IF EXISTS execution_mode;
