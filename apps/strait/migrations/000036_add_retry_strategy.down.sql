ALTER TABLE job_versions DROP COLUMN IF EXISTS retry_delays_secs;
ALTER TABLE job_versions DROP COLUMN IF EXISTS retry_strategy;
ALTER TABLE jobs DROP COLUMN IF EXISTS retry_delays_secs;
ALTER TABLE jobs DROP COLUMN IF EXISTS retry_strategy;
