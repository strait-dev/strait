CREATE INDEX IF NOT EXISTS idx_job_runs_metadata_gin ON job_runs USING GIN (metadata jsonb_path_ops);
CREATE INDEX IF NOT EXISTS idx_job_runs_tags_gin ON job_runs USING GIN (tags jsonb_path_ops);
