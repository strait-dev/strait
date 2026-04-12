DROP INDEX IF EXISTS idx_job_runs_replayed_run_id;
ALTER TABLE job_runs DROP COLUMN IF EXISTS replayed_run_id;
UPDATE schema_version SET version = 197, updated_at = NOW();
