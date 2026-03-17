DROP INDEX IF EXISTS idx_run_compute_usage_project_status_day;
ALTER TABLE run_compute_usage DROP COLUMN IF EXISTS status;
