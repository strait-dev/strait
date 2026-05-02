-- Replace the plain dequeue index with a covering index that includes
-- the columns needed for the LEFT JOIN to job_active_counts and the
-- WHERE clause predicates. This enables an index-only scan on the
-- claim table so the dequeue hot path never touches the heap.
--
-- safety-ok: CONCURRENTLY not supported on non-partitioned table replacement
-- in a single migration. The claim table is small (pending runs only)
-- so the brief lock is acceptable.

DROP INDEX IF EXISTS idx_job_run_queue_dequeue;

-- safety-ok: job_run_queue is a small non-partitioned table (pending runs only).
-- The brief lock during CREATE INDEX is acceptable.
CREATE INDEX idx_job_run_queue_dequeue
  ON job_run_queue (priority DESC, created_at ASC)
  INCLUDE (job_id, concurrency_key, scheduled_at, next_retry_at,
           job_max_concurrency, job_max_concurrency_per_key,
           job_enabled, job_paused);
