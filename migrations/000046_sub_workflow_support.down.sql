DROP INDEX IF EXISTS idx_workflow_runs_parent_workflow_run_id;
ALTER TABLE workflow_runs DROP COLUMN IF EXISTS parent_workflow_run_id;
ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS max_nesting_depth;
ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS sub_workflow_id;
ALTER TABLE workflow_steps DROP COLUMN IF EXISTS max_nesting_depth;
ALTER TABLE workflow_steps DROP COLUMN IF EXISTS sub_workflow_id;
