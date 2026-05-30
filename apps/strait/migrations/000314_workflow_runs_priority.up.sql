-- Give workflow runs a priority so singleton workflow waiters can be promoted by
-- priority, matching job singleton behavior. Default 0 keeps every existing run at
-- the baseline priority.

-- safety-ok: adding a NOT NULL column with a constant default is a metadata-only
-- change in PostgreSQL 11+, so it does not rewrite the table or hold a long lock.
ALTER TABLE workflow_runs
    ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;

-- Replace the singleton waiter index so it leads on priority (descending) before
-- created_at, keeping the priority-ordered promote pick index-ordered.
DROP INDEX IF EXISTS idx_workflow_runs_singleton_waiters;

-- safety-ok: partial index over singleton workflow runs only (singleton_key IS NOT
-- NULL), a small subset, so the non-concurrent rebuild holds only a brief lock.
CREATE INDEX IF NOT EXISTS idx_workflow_runs_singleton_waiters
    ON workflow_runs (workflow_id, singleton_key, status, priority DESC, created_at)
    WHERE singleton_key IS NOT NULL;
