ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS max_concurrency_per_key INT;

ALTER TABLE job_versions
    ADD COLUMN IF NOT EXISTS max_concurrency_per_key INT;

ALTER TABLE job_runs
    ADD COLUMN IF NOT EXISTS concurrency_key TEXT;

CREATE INDEX IF NOT EXISTS idx_job_runs_concurrency_key_active
    ON job_runs (project_id, concurrency_key)
    WHERE concurrency_key IS NOT NULL
      AND status IN ('dequeued', 'executing');
