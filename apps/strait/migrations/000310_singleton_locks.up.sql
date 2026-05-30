-- First-class singleton / mutex execution (STR-542).
-- Lock table is the source of truth for "who holds key K" for a given owner
-- (job or workflow). Acquisition happens synchronously, atomic with run insert.

CREATE TABLE IF NOT EXISTS singleton_locks (
    project_id    TEXT        NOT NULL,
    kind          TEXT        NOT NULL,   -- 'job' | 'workflow'
    owner_id      TEXT        NOT NULL,   -- job_id or workflow_id
    lock_key      TEXT        NOT NULL,   -- resolved key
    holder_run_id TEXT        NOT NULL,   -- job_run.id or workflow_run.id
    acquired_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until   TIMESTAMPTZ,            -- NULL for workflow holders (terminal/missing check only)
    PRIMARY KEY (project_id, kind, owner_id, lock_key)
);

-- safety-ok: index on a table created in this same migration; no existing rows to scan.
CREATE INDEX IF NOT EXISTS idx_singleton_locks_lease
    ON singleton_locks (lease_until)
    WHERE lease_until IS NOT NULL;

-- safety-ok: index on a table created in this same migration; no existing rows to scan.
CREATE INDEX IF NOT EXISTS idx_singleton_locks_holder
    ON singleton_locks (holder_run_id);

-- Job definition columns (nullable: NULL singleton_on_conflict means no singleton).
ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS singleton_key_expr JSONB,
    ADD COLUMN IF NOT EXISTS singleton_on_conflict TEXT,
    ADD COLUMN IF NOT EXISTS singleton_max_queue_depth INT;

-- NOT VALID skips the validating scan, so ADD CONSTRAINT only holds its brief
-- ACCESS EXCLUSIVE lock for the catalog change. The constraint is enforced for
-- all new and updated rows immediately; existing rows are checked separately by
-- VALIDATE CONSTRAINT in 000311 under a weaker SHARE UPDATE EXCLUSIVE lock.
ALTER TABLE jobs
    ADD CONSTRAINT jobs_singleton_on_conflict_check
    CHECK (singleton_on_conflict IS NULL OR singleton_on_conflict IN ('queue', 'drop', 'replace'))
    NOT VALID;

ALTER TABLE job_versions
    ADD COLUMN IF NOT EXISTS singleton_key_expr JSONB,
    ADD COLUMN IF NOT EXISTS singleton_on_conflict TEXT,
    ADD COLUMN IF NOT EXISTS singleton_max_queue_depth INT;

-- Workflow definition columns (only workflows + workflow_versions mirror these).
ALTER TABLE workflows
    ADD COLUMN IF NOT EXISTS singleton_key_expr JSONB,
    ADD COLUMN IF NOT EXISTS singleton_on_conflict TEXT,
    ADD COLUMN IF NOT EXISTS singleton_max_queue_depth INT;

-- See the jobs constraint above: NOT VALID here, validated in 000311.
ALTER TABLE workflows
    ADD CONSTRAINT workflows_singleton_on_conflict_check
    CHECK (singleton_on_conflict IS NULL OR singleton_on_conflict IN ('queue', 'drop', 'replace'))
    NOT VALID;

ALTER TABLE workflow_versions
    ADD COLUMN IF NOT EXISTS singleton_key_expr JSONB,
    ADD COLUMN IF NOT EXISTS singleton_on_conflict TEXT,
    ADD COLUMN IF NOT EXISTS singleton_max_queue_depth INT;

-- Runtime resolved key on run rows (nullable).
ALTER TABLE job_runs
    ADD COLUMN IF NOT EXISTS singleton_key TEXT;

-- job_runs_history must mirror job_runs column-for-column (enforced by the
-- HistoryTableColumnSync invariant), so archived singleton runs keep their key.
ALTER TABLE job_runs_history
    ADD COLUMN IF NOT EXISTS singleton_key TEXT;

ALTER TABLE workflow_runs
    ADD COLUMN IF NOT EXISTS singleton_key TEXT;

-- Waiter-lookup index for the job singleton path: CountSingletonWaiters, the
-- replace-policy waiter cancel, and the FIFO promote all filter
-- (job_id, singleton_key, status) and order by created_at. The partial
-- predicate keeps the index to parked/holding singleton runs only.
--
-- safety-ok: job_runs is a partitioned parent and Postgres rejects CONCURRENTLY
-- on partitioned tables, matching the non-concurrent partitioned-index pattern
-- in 000220 (and 000178, 000197, 000198). singleton_key was just added NULL in
-- this migration so the partial index covers zero existing rows; the build does
-- not block production writes for any meaningful window.
CREATE INDEX IF NOT EXISTS idx_job_runs_singleton_waiters
    ON job_runs (job_id, singleton_key, status, created_at)
    WHERE singleton_key IS NOT NULL;
