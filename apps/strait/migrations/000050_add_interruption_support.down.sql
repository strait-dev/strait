DROP INDEX IF EXISTS idx_workflow_steps_compensate;
ALTER TABLE jobs DROP COLUMN IF EXISTS cancel_endpoint_url;
ALTER TABLE workflow_steps DROP COLUMN IF EXISTS compensate_step_ref;
