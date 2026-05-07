-- safety-ok: Postgres 11+ stores ADD COLUMN ... NOT NULL DEFAULT as a fast
-- metadata-only change (no row rewrite). Mirrors migration 000231 which added
-- queue_name to job_runs with the same pattern; needed so the column-sync
-- invariant between job_runs and job_runs_history holds.
ALTER TABLE job_runs_history
    ADD COLUMN IF NOT EXISTS queue_name TEXT NOT NULL DEFAULT 'default';
