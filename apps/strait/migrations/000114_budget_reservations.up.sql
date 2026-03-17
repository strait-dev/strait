-- Budget reservation: add status column for reserve/commit lifecycle.
ALTER TABLE run_compute_usage ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'committed';

CREATE INDEX IF NOT EXISTS idx_run_compute_usage_project_status_day
  ON run_compute_usage (project_id, status, created_at);
