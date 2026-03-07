DROP INDEX IF EXISTS idx_environments_parent_id;
DROP INDEX IF EXISTS idx_job_dependencies_depends_on_job_id;
DROP INDEX IF EXISTS idx_job_secrets_job_id;
DROP INDEX IF EXISTS idx_workflow_steps_job_id;
DROP INDEX IF EXISTS idx_workflow_step_runs_workflow_step_id;
