-- Phase 3: HOT-update friendly indexes for job_runs.
--
-- The existing idx_runs_project_status (project_id, status, created_at DESC)
-- mentions the `status` column, which means every status transition
-- (queued -> dequeued -> executing -> completed) must update that index and
-- therefore cannot be a HOT (heap-only tuple) update. fillfactor=85 is wasted
-- unless every indexed column of the updated row stays the same -- so we
-- replace the broad index with a mix of HOT-safe and narrow partial indexes
-- that each target a single terminal status.
--
-- After this migration:
--   * project-scoped listings use idx_runs_project_created (no status column)
--   * "all executing runs for project X" uses idx_runs_project_executing
--   * "dead letter inbox" uses idx_runs_project_dead
--   * "upcoming delayed runs" uses idx_runs_project_delayed
--   * The hot path (queued -> dequeued -> executing -> completed) no longer
--     invalidates any index entry outside the transient per-status partial,
--     enabling HOT updates on the heap pages.

CREATE INDEX IF NOT EXISTS idx_runs_project_created
    ON job_runs (project_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_runs_project_executing
    ON job_runs (project_id, heartbeat_at)
    WHERE status = 'executing';

CREATE INDEX IF NOT EXISTS idx_runs_project_dead
    ON job_runs (project_id, finished_at DESC)
    WHERE status = 'dead_letter';

CREATE INDEX IF NOT EXISTS idx_runs_project_delayed
    ON job_runs (project_id, scheduled_at)
    WHERE status = 'delayed';

-- Drop the broad index last so queries that still reach it at migration time
-- keep a plan.
DROP INDEX IF EXISTS idx_runs_project_status;
