DROP INDEX IF EXISTS idx_job_runs_continuation_of;
ALTER TABLE job_runs DROP COLUMN IF EXISTS lineage_depth;
ALTER TABLE job_runs DROP COLUMN IF EXISTS continuation_of;
