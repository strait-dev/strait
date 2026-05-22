-- Restore both retry indexes. 000178 recreated idx_job_runs_retry after
-- the original 000006 idx_runs_retry; here we re-create both forms for
-- complete downgrade coverage.
CREATE INDEX IF NOT EXISTS idx_job_runs_retry
    ON job_runs (next_retry_at)
    WHERE status = 'queued' AND next_retry_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_runs_retry
    ON job_runs (next_retry_at)
    WHERE status = 'queued' AND next_retry_at IS NOT NULL;
