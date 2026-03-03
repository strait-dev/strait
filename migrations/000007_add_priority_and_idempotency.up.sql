-- Add priority column for job priority queue ordering
ALTER TABLE job_runs ADD COLUMN priority INT NOT NULL DEFAULT 0;

-- Add idempotency key for dedup on trigger endpoint
ALTER TABLE job_runs ADD COLUMN idempotency_key TEXT;

-- Partial index for priority-aware dequeue: higher priority first, then FIFO
CREATE INDEX idx_runs_priority ON job_runs(priority DESC, created_at ASC)
  WHERE status = 'queued';

-- Unique index for idempotency: one key per job, ignoring NULL keys
CREATE UNIQUE INDEX idx_runs_idempotency ON job_runs(job_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;
