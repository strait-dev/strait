ALTER TABLE jobs DROP COLUMN IF EXISTS retry_priority_boost;
ALTER TABLE job_versions DROP COLUMN IF EXISTS retry_priority_boost;
