-- Reaper-support index for ListStalledWorkflowRuns: the reaper scans running
-- workflow runs ordered by started_at to find runs whose steps have all
-- finished but the run never transitioned out of 'running'. No existing index
-- leads with status or started_at, so the reaper seq-scans workflow_runs (which
-- grows unbounded with history) every cycle. The partial predicate keeps this
-- index to the small currently-running set, so it stays cheap to maintain.
--
-- workflow_runs is not partitioned, so this is built CONCURRENTLY to avoid an
-- ACCESS EXCLUSIVE lock on a potentially large table. CONCURRENTLY cannot run
-- inside a transaction; golang-migrate handles that automatically because this
-- is the only statement in the file.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workflow_runs_stalled
    ON workflow_runs (started_at)
    WHERE status = 'running';
