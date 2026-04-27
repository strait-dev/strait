-- Restore original status-predicated indexes.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_job_runs_stale_dequeued
  ON job_runs (started_at) WHERE status = 'dequeued';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_runs_project_executing
  ON job_runs (project_id, heartbeat_at) WHERE status = 'executing';

DROP INDEX CONCURRENTLY IF EXISTS idx_job_runs_inflight_started;
