-- Migration 198 added job_runs.replayed_run_id as a self-referential FK with
-- the implicit default (NO ACTION). Because job_runs is partitioned by month
-- and subject to retention partition drops, a dropped original run referenced
-- as replayed_run_id would cause a deletion to fail or leave a dangling
-- reference. Switch the FK to ON DELETE SET NULL so retention drops and
-- explicit purges can proceed; the lineage simply becomes null when the
-- pointed-to row disappears.

ALTER TABLE job_runs DROP CONSTRAINT IF EXISTS job_runs_replayed_run_id_fkey;
ALTER TABLE job_runs
    ADD CONSTRAINT job_runs_replayed_run_id_fkey
    FOREIGN KEY (replayed_run_id) REFERENCES job_runs(id) ON DELETE SET NULL;

UPDATE schema_version SET version = 199, updated_at = NOW();
