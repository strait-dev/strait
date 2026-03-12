-- Recreate the partial unique index for idempotency enforcement on partitioned job_runs.
-- Partitioned tables require unique indexes to include all partition columns,
-- so created_at is included here. Per-partition uniqueness is sufficient because
-- active runs (the WHERE clause statuses) reside in the current-month partition.
-- Cross-partition dedup is handled by the job_run_idempotency table (migration 000065).
CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_idempotency
  ON job_runs (job_id, idempotency_key, created_at)
  WHERE idempotency_key IS NOT NULL
    AND status IN ('delayed', 'queued', 'dequeued', 'executing', 'waiting');
