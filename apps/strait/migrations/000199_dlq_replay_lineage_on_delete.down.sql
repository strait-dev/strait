-- Revert replayed_run_id FK back to the default NO ACTION behavior.

ALTER TABLE job_runs DROP CONSTRAINT IF EXISTS job_runs_replayed_run_id_fkey;
ALTER TABLE job_runs
    ADD CONSTRAINT job_runs_replayed_run_id_fkey
    FOREIGN KEY (replayed_run_id) REFERENCES job_runs(id);

UPDATE schema_version SET version = 198, updated_at = NOW();
