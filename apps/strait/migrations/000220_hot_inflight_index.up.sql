-- Replace status-predicated partial indexes that block HOT updates on
-- dequeued->executing transitions with a single non-status-predicated
-- index on in-flight runs.
--
-- idx_job_runs_stale_dequeued: (started_at) WHERE status = 'dequeued'
--   Used by: store.ListStaleDequeued (reaper, every 30s)
--
-- idx_runs_project_executing: (project_id, heartbeat_at) WHERE status = 'executing'
--   Used by: store.ListStaleRuns (heartbeat reaper, every 30s)
--
-- Replacement: idx_job_runs_inflight_started covers both use cases.
-- It does NOT predicate on status, so status transitions between
-- 'dequeued' and 'executing' become HOT-eligible.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_job_runs_inflight_started
  ON job_runs (started_at ASC)
  WHERE finished_at IS NULL AND started_at IS NOT NULL;

DROP INDEX CONCURRENTLY IF EXISTS idx_job_runs_stale_dequeued;
DROP INDEX CONCURRENTLY IF EXISTS idx_runs_project_executing;
