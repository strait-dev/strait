DROP INDEX IF EXISTS idx_job_runs_concurrency_key_active;
ALTER TABLE job_runs DROP COLUMN IF EXISTS concurrency_key;
ALTER TABLE job_versions DROP COLUMN IF EXISTS max_concurrency_per_key;
ALTER TABLE jobs DROP COLUMN IF EXISTS max_concurrency_per_key;
