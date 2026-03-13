-- 000065: Separate idempotency tracking table (Plan 3.2)
-- After partitioning job_runs, the unique index on (job_id, idempotency_key)
-- becomes per-partition. This table provides global dedup across partitions.

CREATE TABLE IF NOT EXISTS job_run_idempotency (
  job_id          TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  run_id          TEXT NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at      TIMESTAMPTZ,
  PRIMARY KEY (job_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_idempotency_expires
  ON job_run_idempotency (expires_at)
  WHERE expires_at IS NOT NULL;

-- Backfill from existing job_runs where idempotency_key is set
-- Only backfill recent entries (within a reasonable dedup window)
INSERT INTO job_run_idempotency (job_id, idempotency_key, run_id, created_at)
SELECT job_id, idempotency_key, id, created_at
FROM job_runs
WHERE idempotency_key IS NOT NULL
  AND created_at > NOW() - INTERVAL '30 days'
ON CONFLICT (job_id, idempotency_key) DO NOTHING;
