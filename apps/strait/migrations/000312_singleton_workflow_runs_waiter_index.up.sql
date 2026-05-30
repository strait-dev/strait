-- Waiter-lookup index for the workflow singleton path: CountSingletonWaiters,
-- the replace-policy waiter cancel, and the FIFO promote all filter
-- (workflow_id, singleton_key, status) and order by created_at. The partial
-- predicate keeps the index to parked/holding singleton runs only.
--
-- workflow_runs is not partitioned, so this is built CONCURRENTLY to avoid an
-- ACCESS EXCLUSIVE lock on a potentially large table. CONCURRENTLY cannot run
-- inside a transaction; golang-migrate handles that automatically because this
-- is the only statement in the file.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workflow_runs_singleton_waiters
    ON workflow_runs (workflow_id, singleton_key, status, created_at)
    WHERE singleton_key IS NOT NULL;
