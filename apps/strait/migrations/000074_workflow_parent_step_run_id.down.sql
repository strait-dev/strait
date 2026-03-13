DROP INDEX IF EXISTS idx_workflow_runs_parent_step_run_id;

ALTER TABLE workflow_runs
    DROP COLUMN IF EXISTS parent_step_run_id;
