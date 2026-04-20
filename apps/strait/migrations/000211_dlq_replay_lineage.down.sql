ALTER TABLE job_runs DROP COLUMN IF EXISTS replayed_run_id;
UPDATE schema_version SET version = 210, updated_at = NOW();
