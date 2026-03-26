CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_runs_idempotency_terminal
    ON job_runs (job_id, idempotency_key, finished_at)
    WHERE idempotency_key IS NOT NULL
      AND status IN ('completed', 'failed', 'timed_out', 'crashed', 'system_failed', 'canceled')
      AND finished_at > NOW() - INTERVAL '24 hours';
