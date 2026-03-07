-- Restore NOT NULL constraint on workflow_steps.job_id
-- Note: This will fail if any rows have NULL job_id values.
ALTER TABLE workflow_steps ALTER COLUMN job_id SET NOT NULL;
