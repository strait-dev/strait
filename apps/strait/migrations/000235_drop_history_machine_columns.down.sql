-- Restore columns dropped in 000235.up.sql. Existing history rows get NULL
-- values for these columns (the underlying live table no longer carries them).
ALTER TABLE job_runs_history ADD COLUMN IF NOT EXISTS machine_id TEXT;
ALTER TABLE job_runs_history ADD COLUMN IF NOT EXISTS deployment_id TEXT;
ALTER TABLE job_runs_history ADD COLUMN IF NOT EXISTS pinned_image_uri TEXT;
ALTER TABLE job_runs_history ADD COLUMN IF NOT EXISTS pinned_image_digest TEXT;
