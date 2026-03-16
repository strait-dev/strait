DROP INDEX IF EXISTS idx_job_runs_execution_mode_managed;
ALTER TABLE job_runs DROP COLUMN IF EXISTS execution_mode;
