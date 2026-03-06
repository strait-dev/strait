ALTER TABLE workflow_runs ADD COLUMN retry_of_run_id TEXT;

CREATE INDEX idx_workflow_runs_retry_of_run_id ON workflow_runs (retry_of_run_id) WHERE retry_of_run_id IS NOT NULL;
