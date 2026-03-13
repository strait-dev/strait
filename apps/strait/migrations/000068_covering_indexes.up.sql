CREATE INDEX IF NOT EXISTS idx_runs_queue_covering
    ON job_runs (created_at ASC)
    INCLUDE (job_id, priority, scheduled_at, next_retry_at)
    WHERE status = 'queued';

CREATE INDEX IF NOT EXISTS idx_runs_status_project_covering
    ON job_runs (project_id, status, created_at DESC)
    INCLUDE (job_id, priority, scheduled_at, next_retry_at);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending
    ON webhook_deliveries (next_retry_at ASC)
    WHERE status = 'pending' AND next_retry_at IS NOT NULL;
