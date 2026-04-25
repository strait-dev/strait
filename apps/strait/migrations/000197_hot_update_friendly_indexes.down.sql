-- Revert Phase 3 HOT-update-friendly indexes.
DROP INDEX IF EXISTS idx_runs_project_created;
DROP INDEX IF EXISTS idx_runs_project_executing;
DROP INDEX IF EXISTS idx_runs_project_dead;
DROP INDEX IF EXISTS idx_runs_project_delayed;

CREATE INDEX IF NOT EXISTS idx_runs_project_status
    ON job_runs (project_id, status, created_at DESC);
