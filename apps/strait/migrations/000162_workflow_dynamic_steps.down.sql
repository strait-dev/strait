DROP INDEX IF EXISTS idx_workflow_dynamic_steps_parent;
DROP INDEX IF EXISTS idx_workflow_dynamic_steps_run;
DROP TABLE IF EXISTS workflow_dynamic_steps;

DELETE FROM workflow_step_runs
WHERE workflow_step_id IS NULL;

ALTER TABLE workflow_step_runs
    ALTER COLUMN workflow_step_id SET NOT NULL;
