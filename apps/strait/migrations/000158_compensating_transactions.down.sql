DROP TABLE IF EXISTS compensation_runs;
ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS compensation_timeout_secs;
ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS compensation_job_id;
ALTER TABLE workflow_steps DROP COLUMN IF EXISTS compensation_timeout_secs;
ALTER TABLE workflow_steps DROP COLUMN IF EXISTS compensation_job_id;
