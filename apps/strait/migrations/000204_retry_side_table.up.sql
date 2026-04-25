-- R2 Phase 7: retry side table.
--
-- UPDATE job_runs SET next_retry_at = ... is indexed by idx_runs_retry
-- (partial btree on next_retry_at) and therefore cannot be a HOT update.
-- Every retry schedule churns job_runs indexes and defeats Phase 3's
-- HOT-update wins.
--
-- Moving retry scheduling to a small, PK-only side table lets the
-- original job_runs row stay in 'queued' with no index-column updates.
-- Dequeue anti-joins against the side table; DelayedPoller walks the
-- side table to promote rows when next_retry_at fires.

CREATE TABLE IF NOT EXISTS job_retries (
    run_id        TEXT PRIMARY KEY,
    next_retry_at TIMESTAMPTZ NOT NULL,
    attempt       INT NOT NULL DEFAULT 1,
    scheduled_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_job_retries_next_retry_at
    ON job_retries (next_retry_at);

-- Seed from current state so enabling the side-table path doesn't lose
-- pending retries.
INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at)
SELECT id, next_retry_at, attempt, COALESCE(started_at, created_at)
FROM job_runs
WHERE next_retry_at IS NOT NULL
  AND status = 'queued'
ON CONFLICT (run_id) DO UPDATE
  SET next_retry_at = EXCLUDED.next_retry_at,
      attempt = EXCLUDED.attempt;
