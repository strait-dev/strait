-- Add missing columns to job_versions for full snapshot fidelity.
-- These columns exist on the jobs table but were not included in the
-- original job_versions schema, preventing accurate version reconstruction.
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS group_id TEXT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS project_id TEXT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;
