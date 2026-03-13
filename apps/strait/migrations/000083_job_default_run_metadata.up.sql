ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS default_run_metadata JSONB NOT NULL DEFAULT '{}';

ALTER TABLE job_versions
    ADD COLUMN IF NOT EXISTS default_run_metadata JSONB NOT NULL DEFAULT '{}';
