-- Restore all dropped indexes.
CREATE INDEX IF NOT EXISTS idx_job_runs_stale_dequeued
  ON job_runs (started_at) WHERE status = 'dequeued';

CREATE INDEX IF NOT EXISTS idx_runs_project_executing
  ON job_runs (project_id, heartbeat_at) WHERE status = 'executing';

CREATE INDEX IF NOT EXISTS idx_job_runs_active_by_job
  ON job_runs (job_id) WHERE status IN ('dequeued', 'executing');

CREATE INDEX IF NOT EXISTS idx_job_runs_concurrency_key_active
  ON job_runs (project_id, concurrency_key)
  WHERE concurrency_key IS NOT NULL AND status IN ('dequeued', 'executing');
