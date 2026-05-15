-- Reverse Wave 2 Phase 2: restore the partial terminal index.

-- safety-ok: golang-migrate wraps each migration in a transaction;
-- CONCURRENTLY is not viable, and DROP INDEX takes only a brief lock
-- per partition. Matches the up-migration's swap pattern.
DROP INDEX IF EXISTS idx_runs_idempotency;

-- safety-ok: same constraint as the DROP above. Recreates the original
-- (job_id, idempotency_key, finished_at) partial-on-terminal-status index.
CREATE INDEX IF NOT EXISTS idx_runs_idempotency_terminal
    ON job_runs (job_id, idempotency_key, finished_at)
    WHERE idempotency_key IS NOT NULL
      AND status IN ('completed', 'failed', 'timed_out', 'crashed', 'system_failed', 'canceled');
