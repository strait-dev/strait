-- Recreate the partial unique index for idempotency enforcement on partitioned job_runs.
-- This index was lost during the 000066 partitioning migration because the legacy table
-- was dropped (along with its indexes) and only queue/status indexes were recreated.
-- The application relies on this index to detect duplicate idempotency_key inserts
-- (PostgreSQL error 23505 on constraint idx_runs_idempotency).
CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_idempotency
  ON job_runs (job_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL
    AND status IN ('delayed', 'queued', 'dequeued', 'executing', 'waiting');
