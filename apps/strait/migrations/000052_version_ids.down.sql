DROP INDEX IF EXISTS idx_workflows_version_id;
DROP INDEX IF EXISTS idx_jobs_version_id;
ALTER TABLE workflow_runs DROP COLUMN IF EXISTS workflow_version_id;
ALTER TABLE job_runs DROP COLUMN IF EXISTS job_version_id;
ALTER TABLE workflow_versions DROP COLUMN IF EXISTS version_id;
ALTER TABLE job_versions DROP COLUMN IF EXISTS version_id;
ALTER TABLE workflows DROP COLUMN IF EXISTS version_id;
ALTER TABLE jobs DROP COLUMN IF EXISTS version_id;
