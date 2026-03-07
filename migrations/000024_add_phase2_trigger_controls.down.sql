ALTER TABLE job_versions DROP COLUMN IF EXISTS dedup_window_secs;
ALTER TABLE job_versions DROP COLUMN IF EXISTS rate_limit_window_secs;
ALTER TABLE job_versions DROP COLUMN IF EXISTS rate_limit_max;
ALTER TABLE job_versions DROP COLUMN IF EXISTS timezone;
ALTER TABLE job_versions DROP COLUMN IF EXISTS execution_window_cron;
ALTER TABLE job_versions DROP COLUMN IF EXISTS max_concurrency;

ALTER TABLE jobs DROP COLUMN IF EXISTS dedup_window_secs;
ALTER TABLE jobs DROP COLUMN IF EXISTS rate_limit_window_secs;
ALTER TABLE jobs DROP COLUMN IF EXISTS rate_limit_max;
