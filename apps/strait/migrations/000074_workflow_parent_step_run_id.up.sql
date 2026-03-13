ALTER TABLE workflow_runs
    ADD COLUMN parent_step_run_id TEXT REFERENCES workflow_step_runs(id);

CREATE INDEX idx_workflow_runs_parent_step_run_id ON workflow_runs(parent_step_run_id);
