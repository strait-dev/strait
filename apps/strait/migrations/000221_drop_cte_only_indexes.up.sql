-- Drop the old status-predicated indexes that blocked HOT updates.
-- idx_job_runs_stale_dequeued served ListStaleDequeued (reaper).
-- idx_runs_project_executing served ListStaleRuns (heartbeat reaper).
-- Both are replaced by idx_job_runs_inflight_started (migration 000220).
--
-- Also drop CTE-only concurrency indexes: with denormalized dequeue
-- (job_active_counts) as default, these are no longer on the hot path.
-- Removing them enables HOT on executing->completed transitions.
DROP INDEX IF EXISTS idx_job_runs_stale_dequeued;
DROP INDEX IF EXISTS idx_runs_project_executing;
DROP INDEX IF EXISTS idx_job_runs_active_by_job;
DROP INDEX IF EXISTS idx_job_runs_concurrency_key_active;
