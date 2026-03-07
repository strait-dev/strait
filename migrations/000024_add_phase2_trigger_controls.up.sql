ALTER TABLE jobs ADD COLUMN IF NOT EXISTS rate_limit_max INT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS rate_limit_window_secs INT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS dedup_window_secs INT;

ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS max_concurrency INT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS execution_window_cron TEXT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS timezone TEXT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS rate_limit_max INT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS rate_limit_window_secs INT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS dedup_window_secs INT;
