ALTER TABLE jobs DROP COLUMN IF EXISTS skip_if_running;
ALTER TABLE job_versions DROP COLUMN IF EXISTS skip_if_running;
