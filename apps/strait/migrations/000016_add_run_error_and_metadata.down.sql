DROP INDEX IF EXISTS idx_job_runs_metadata_gin;
DROP INDEX IF EXISTS idx_job_runs_error_class;

ALTER TABLE job_runs DROP COLUMN IF EXISTS metadata;
ALTER TABLE job_runs DROP COLUMN IF EXISTS error_class;
