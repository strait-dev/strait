DROP INDEX IF EXISTS idx_job_runs_parent_run_id;
ALTER TABLE job_runs DROP COLUMN parent_run_id;
