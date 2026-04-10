DROP INDEX IF EXISTS idx_jobs_default_run_metadata_gin;
DROP INDEX IF EXISTS idx_job_runs_parent_run_id_created_at;

CREATE INDEX IF NOT EXISTS idx_job_runs_parent_run_id
    ON job_runs (parent_run_id)
    WHERE parent_run_id IS NOT NULL;
