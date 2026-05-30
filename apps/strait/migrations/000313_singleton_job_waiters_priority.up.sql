-- Promote singleton job waiters by priority, not just FIFO. Replace the waiter
-- index so it leads on priority (descending) before created_at, letting
-- ReleaseSingletonJobLockAndPromote pick the highest-priority parked run with an
-- index-ordered scan instead of a strict oldest-first scan.

DROP INDEX IF EXISTS idx_job_runs_singleton_waiters;

-- safety-ok: job_runs is a partitioned parent and Postgres rejects CONCURRENTLY
-- on partitioned tables, matching the non-concurrent partitioned-index pattern in
-- 000310 (and 000178, 000197, 000198). The partial predicate keeps the index to
-- singleton runs only, so the rebuild covers a small subset and does not block
-- production writes for any meaningful window.
CREATE INDEX IF NOT EXISTS idx_job_runs_singleton_waiters
    ON job_runs (job_id, singleton_key, status, priority DESC, created_at)
    WHERE singleton_key IS NOT NULL;
