-- Restore machine columns and relax execution_mode constraints.

-- Restore machine columns on jobs.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS machine_preset TEXT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS image_uri TEXT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS region TEXT;

-- Restore machine columns on job_versions.
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS machine_preset TEXT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS image_uri TEXT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS region TEXT;

-- Restore machine_id on job_runs.
ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS machine_id TEXT;

-- Drop the tightened constraints; original constraint (if any) was broader.
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_execution_mode_check;
ALTER TABLE job_runs DROP CONSTRAINT IF EXISTS job_runs_execution_mode_check;
