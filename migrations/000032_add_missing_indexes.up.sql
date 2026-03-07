-- Add missing foreign key indexes for query performance
CREATE INDEX IF NOT EXISTS idx_environments_parent_id ON environments(parent_id) WHERE parent_id != '';
CREATE INDEX IF NOT EXISTS idx_job_dependencies_depends_on_job_id ON job_dependencies(depends_on_job_id);
CREATE INDEX IF NOT EXISTS idx_job_secrets_job_id ON job_secrets(job_id);
CREATE INDEX IF NOT EXISTS idx_workflow_steps_job_id ON workflow_steps(job_id);
CREATE INDEX IF NOT EXISTS idx_workflow_step_runs_workflow_step_id ON workflow_step_runs(workflow_step_id);
