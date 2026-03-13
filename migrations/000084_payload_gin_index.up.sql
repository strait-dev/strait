CREATE INDEX IF NOT EXISTS idx_job_runs_payload_gin
    ON job_runs USING gin (payload jsonb_path_ops)
    WHERE payload IS NOT NULL;
