-- Scheduler-support index for ListCronJobs: the cron poll selects enabled,
-- non-paused jobs that have a cron expression, ordered by created_at. No index
-- leads with this predicate (workflows already has idx_workflows_enabled_cron
-- for the analogous query), so the poll seq-scans jobs every tick. The partial
-- predicate keeps the index to the small active-cron set, and indexing
-- created_at DESC also serves the query's ORDER BY.
--
-- jobs is not partitioned, so this is built CONCURRENTLY to avoid an ACCESS
-- EXCLUSIVE lock on a potentially large table. CONCURRENTLY cannot run inside a
-- transaction; golang-migrate handles that automatically because this is the
-- only statement in the file.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_jobs_cron_enabled
    ON jobs (created_at DESC)
    WHERE enabled = TRUE AND NOT paused AND cron IS NOT NULL AND cron <> '';
