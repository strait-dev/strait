-- Drop the idempotency index (reverting to pre-fix state).
DROP INDEX IF EXISTS idx_runs_idempotency;
