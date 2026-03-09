DROP INDEX IF EXISTS idx_workflow_runs_compensation;
ALTER TABLE workflow_runs DROP COLUMN IF EXISTS compensation_steps_completed;
ALTER TABLE workflow_runs DROP COLUMN IF EXISTS compensation_steps_total;
ALTER TABLE workflow_runs DROP COLUMN IF EXISTS compensation_status;
