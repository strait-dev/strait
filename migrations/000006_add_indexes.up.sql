-- Index for ListRunsByJob queries
CREATE INDEX idx_runs_job_id ON job_runs(job_id, created_at DESC);

-- Index for retry backoff: dequeue must respect next_retry_at
CREATE INDEX idx_runs_retry ON job_runs(next_retry_at)
  WHERE status = 'queued' AND next_retry_at IS NOT NULL;

-- Index for reaper: find stale dequeued runs
CREATE INDEX idx_runs_status_dequeued ON job_runs(started_at)
  WHERE status = 'dequeued';
