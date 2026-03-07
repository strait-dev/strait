-- Make job_id nullable on workflow_steps to support approval and sub-workflow step types
-- that do not reference a job.
ALTER TABLE workflow_steps ALTER COLUMN job_id DROP NOT NULL;
