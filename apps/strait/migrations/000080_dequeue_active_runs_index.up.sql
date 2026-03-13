CREATE INDEX IF NOT EXISTS idx_job_runs_active_by_job
ON job_runs (job_id)
WHERE status IN ('dequeued', 'executing');