ALTER TABLE jobs ADD COLUMN retry_strategy TEXT;
ALTER TABLE jobs ADD COLUMN retry_delays_secs INT[];
ALTER TABLE job_versions ADD COLUMN retry_strategy TEXT;
ALTER TABLE job_versions ADD COLUMN retry_delays_secs INT[];
