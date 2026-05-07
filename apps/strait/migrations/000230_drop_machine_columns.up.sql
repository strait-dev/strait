-- Remove managed-container machine columns; orchestration-only mode uses
-- HTTP and worker execution exclusively.

-- Drop machine columns from jobs.
ALTER TABLE jobs DROP COLUMN IF EXISTS machine_preset;
ALTER TABLE jobs DROP COLUMN IF EXISTS image_uri;
ALTER TABLE jobs DROP COLUMN IF EXISTS region;

-- Drop machine columns from job_versions (mirrors jobs).
ALTER TABLE job_versions DROP COLUMN IF EXISTS machine_preset;
ALTER TABLE job_versions DROP COLUMN IF EXISTS image_uri;
ALTER TABLE job_versions DROP COLUMN IF EXISTS region;

-- Drop machine_id from job_runs.
ALTER TABLE job_runs DROP COLUMN IF EXISTS machine_id;

-- Tighten execution_mode CHECK on jobs to the two supported modes.
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_execution_mode_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_execution_mode_check CHECK (execution_mode IN ('http', 'worker'));

-- Tighten execution_mode CHECK on job_runs.
ALTER TABLE job_runs DROP CONSTRAINT IF EXISTS job_runs_execution_mode_check;
ALTER TABLE job_runs ADD CONSTRAINT job_runs_execution_mode_check CHECK (execution_mode IN ('http', 'worker'));
