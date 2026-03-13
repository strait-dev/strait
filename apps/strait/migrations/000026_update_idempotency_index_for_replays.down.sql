DROP INDEX IF EXISTS idx_runs_idempotency;

CREATE UNIQUE INDEX idx_runs_idempotency ON job_runs(job_id, idempotency_key)
WHERE idempotency_key IS NOT NULL;
