-- Index for ListTimedOutWorkflowRuns reaper query.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workflow_runs_status_expires
    ON workflow_runs (status, expires_at)
    WHERE expires_at IS NOT NULL;

-- Index for DeleteWorkflowRunsFinishedBefore reaper query.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workflow_runs_status_finished
    ON workflow_runs (status, finished_at)
    WHERE finished_at IS NOT NULL;

-- Composite index for step run queries filtering by workflow_run_id + status.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_step_runs_workflow_run_status
    ON workflow_step_runs (workflow_run_id, status);

-- Index for CASCADE deletes on workflow_step_decisions.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_step_decisions_step_run_id
    ON workflow_step_decisions (step_run_id);
