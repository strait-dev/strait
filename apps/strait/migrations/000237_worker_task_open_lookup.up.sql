CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_worker_tasks_open_run_owner
    ON worker_tasks (worker_id, project_id, run_id, assigned_at DESC)
    WHERE status IN ('assigned', 'accepted');
