DROP INDEX IF EXISTS idx_runs_idempotency;
DROP INDEX IF EXISTS idx_runs_priority;
ALTER TABLE job_runs DROP COLUMN IF EXISTS idempotency_key;
ALTER TABLE job_runs DROP COLUMN IF EXISTS priority;
