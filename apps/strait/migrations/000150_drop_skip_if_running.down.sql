ALTER TABLE jobs ADD COLUMN skip_if_running BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE job_versions ADD COLUMN skip_if_running BOOLEAN NOT NULL DEFAULT false;

UPDATE jobs SET skip_if_running = true WHERE cron_overlap_policy = 'skip';
