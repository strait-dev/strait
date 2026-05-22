-- Wave 2 Phase 1: retry side table is now authoritative for dequeue gating.
--
-- All hot-path retry scheduling writes go to job_retries (PK-only side
-- table). The dequeue predicate anti-joins against job_retries instead of
-- reading job_runs.next_retry_at, so the partial btree on job_runs
-- (next_retry_at) WHERE status='queued' is now write-amplification with
-- no readers. Drop it so the requeue UPDATE on job_runs becomes HOT.
--
-- One last backfill in case any rows were written between 000204 and now;
-- the seed is a no-op when the side table already covers every pending
-- retry.
INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at)
SELECT id, next_retry_at, attempt, COALESCE(started_at, created_at)
FROM job_runs
WHERE next_retry_at IS NOT NULL
  AND status IN ('queued', 'delayed')
ON CONFLICT (run_id) DO UPDATE
  SET next_retry_at = EXCLUDED.next_retry_at,
      attempt = EXCLUDED.attempt;

DROP INDEX IF EXISTS idx_job_runs_retry;
DROP INDEX IF EXISTS idx_runs_retry;
