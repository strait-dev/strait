ALTER TABLE job_versions DROP COLUMN IF EXISTS environment_id;
ALTER TABLE jobs DROP COLUMN IF EXISTS environment_id;
