ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS error_class TEXT;
ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_job_runs_error_class ON job_runs(error_class) WHERE error_class IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_job_runs_metadata_gin ON job_runs USING GIN (metadata);
