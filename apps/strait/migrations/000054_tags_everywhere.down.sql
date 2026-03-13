DROP INDEX IF EXISTS idx_jobs_tags;
DROP INDEX IF EXISTS idx_job_runs_tags;
DROP INDEX IF EXISTS idx_workflow_runs_tags;
DROP INDEX IF EXISTS idx_workflows_tags;
ALTER TABLE job_runs DROP COLUMN IF EXISTS tags;
ALTER TABLE workflow_runs DROP COLUMN IF EXISTS tags;
ALTER TABLE workflows DROP COLUMN IF EXISTS tags;
