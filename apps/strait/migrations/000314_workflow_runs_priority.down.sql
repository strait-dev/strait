-- Restore the FIFO-only singleton waiter index and drop the priority column.

DROP INDEX IF EXISTS idx_workflow_runs_singleton_waiters;

-- safety-ok: partial index over singleton workflow runs only; small subset, brief lock.
CREATE INDEX IF NOT EXISTS idx_workflow_runs_singleton_waiters
    ON workflow_runs (workflow_id, singleton_key, status, created_at)
    WHERE singleton_key IS NOT NULL;

ALTER TABLE workflow_runs DROP COLUMN IF EXISTS priority;
