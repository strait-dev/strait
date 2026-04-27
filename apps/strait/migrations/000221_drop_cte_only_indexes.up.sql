-- Drop indexes that only serve the CTE-based concurrency check path.
-- With denormalized dequeue (job_active_counts) as the default, these
-- indexes are no longer needed on the hot path. Removing them enables
-- HOT updates on the executing->completed transition because the row
-- no longer leaves a status IN ('dequeued','executing') partial index.

DROP INDEX CONCURRENTLY IF EXISTS idx_job_runs_active_by_job;
DROP INDEX CONCURRENTLY IF EXISTS idx_job_runs_concurrency_key_active;
