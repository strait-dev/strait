DROP INDEX IF EXISTS idx_workflow_runs_retry_of_run_id;

ALTER TABLE workflow_runs DROP COLUMN IF EXISTS retry_of_run_id;
