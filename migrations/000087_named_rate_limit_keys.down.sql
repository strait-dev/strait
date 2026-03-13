ALTER TABLE job_versions DROP COLUMN IF EXISTS rate_limit_keys;
ALTER TABLE jobs DROP COLUMN IF EXISTS rate_limit_keys;
