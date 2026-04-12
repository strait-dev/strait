-- Phase 3: DLQ admin recovery endpoints require tracking replay lineage so
-- operators can see that a dead-lettered run was superseded by a new one.
-- Nullable FK back to job_runs(id); partial index keeps it cheap when most
-- rows never get replayed.

ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS replayed_run_id UUID NULL REFERENCES job_runs(id);
CREATE INDEX IF NOT EXISTS idx_job_runs_replayed_run_id ON job_runs(replayed_run_id) WHERE replayed_run_id IS NOT NULL;
UPDATE schema_version SET version = 198, updated_at = NOW();
