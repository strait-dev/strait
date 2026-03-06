-- Remove attempt column from workflow_step_runs
ALTER TABLE workflow_step_runs DROP COLUMN IF EXISTS attempt;
