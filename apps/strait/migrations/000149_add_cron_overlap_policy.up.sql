ALTER TABLE jobs ADD COLUMN cron_overlap_policy TEXT NOT NULL DEFAULT 'allow';

ALTER TABLE jobs ADD CONSTRAINT jobs_cron_overlap_policy_check
  CHECK (cron_overlap_policy IN ('allow', 'skip', 'cancel_running'));

UPDATE jobs SET cron_overlap_policy = 'skip' WHERE skip_if_running = true;

ALTER TABLE job_versions ADD COLUMN cron_overlap_policy TEXT NOT NULL DEFAULT 'allow';
