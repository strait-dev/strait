-- Add execution_mode to job_runs for filtered queries without joining jobs.
ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS execution_mode TEXT NOT NULL DEFAULT 'http';

-- Partial index for listing managed runs efficiently.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_job_runs_execution_mode_managed
    ON job_runs (project_id, created_at DESC)
    WHERE execution_mode = 'managed';
