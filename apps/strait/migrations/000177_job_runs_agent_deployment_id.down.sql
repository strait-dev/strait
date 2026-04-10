DROP INDEX IF EXISTS idx_job_runs_job_agent_deployment;
ALTER TABLE job_runs DROP COLUMN IF EXISTS agent_deployment_id;
