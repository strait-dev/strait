-- Reaper-support index for ListOrphanedStepRuns: the reaper finds step runs
-- still marked 'running' whose backing job_run has already reached a terminal
-- state. The query filters workflow_step_runs on status = 'running' without a
-- workflow_run_id, but the only composite index leads with workflow_run_id, so
-- the reaper seq-scans workflow_step_runs (which grows with history) every
-- cycle. The partial predicate keeps this index to the small currently-running
-- set and lets the planner drive the join to job_runs from it.
--
-- workflow_step_runs is not partitioned, so this is built CONCURRENTLY to avoid
-- an ACCESS EXCLUSIVE lock. CONCURRENTLY cannot run inside a transaction;
-- golang-migrate handles that automatically because this is the only statement
-- in the file.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workflow_step_runs_running
    ON workflow_step_runs (id)
    WHERE status = 'running';
