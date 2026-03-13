DROP INDEX IF EXISTS idx_job_runs_batch_id;
ALTER TABLE job_runs DROP COLUMN IF EXISTS batch_id;
DROP INDEX IF EXISTS idx_batch_operations_project;
DROP TABLE IF EXISTS batch_operations;
