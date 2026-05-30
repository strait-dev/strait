-- Restore the FIFO-only waiter index (priority column dropped from the index).

DROP INDEX IF EXISTS idx_job_runs_singleton_waiters;

-- safety-ok: see the up migration; same partitioned-parent, small-subset rebuild.
CREATE INDEX IF NOT EXISTS idx_job_runs_singleton_waiters
    ON job_runs (job_id, singleton_key, status, created_at)
    WHERE singleton_key IS NOT NULL;
