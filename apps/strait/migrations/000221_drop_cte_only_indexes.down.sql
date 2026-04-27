-- Restore CTE-path indexes.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_job_runs_active_by_job
  ON job_runs (job_id) WHERE status IN ('dequeued', 'executing');

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_job_runs_concurrency_key_active
  ON job_runs (project_id, concurrency_key)
  WHERE concurrency_key IS NOT NULL AND status IN ('dequeued', 'executing');
