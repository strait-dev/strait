ALTER TABLE job_versions DROP COLUMN IF EXISTS cron_overlap_policy;

ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_cron_overlap_policy_check;

ALTER TABLE jobs DROP COLUMN IF EXISTS cron_overlap_policy;
