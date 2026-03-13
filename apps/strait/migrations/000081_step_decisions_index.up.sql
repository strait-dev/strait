-- Index for CASCADE deletes on workflow_step_decisions.
CREATE INDEX IF NOT EXISTS idx_step_decisions_step_run_id
    ON workflow_step_decisions (step_run_id);
