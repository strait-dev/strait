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

ALTER TABLE jobs
    ADD CONSTRAINT jobs_singleton_on_conflict_check
    CHECK (singleton_on_conflict IS NULL OR singleton_on_conflict IN ('queue', 'drop', 'replace'));

ALTER TABLE job_versions
    ADD COLUMN IF NOT EXISTS singleton_key_expr JSONB,
    ADD COLUMN IF NOT EXISTS singleton_on_conflict TEXT,
    ADD COLUMN IF NOT EXISTS singleton_max_queue_depth INT;

-- Workflow definition columns (only workflows + workflow_versions mirror these).
ALTER TABLE workflows
    ADD COLUMN IF NOT EXISTS singleton_key_expr JSONB,
    ADD COLUMN IF NOT EXISTS singleton_on_conflict TEXT,
    ADD COLUMN IF NOT EXISTS singleton_max_queue_depth INT;

ALTER TABLE workflows
    ADD CONSTRAINT workflows_singleton_on_conflict_check
    CHECK (singleton_on_conflict IS NULL OR singleton_on_conflict IN ('queue', 'drop', 'replace'));

ALTER TABLE workflow_versions
    ADD COLUMN IF NOT EXISTS singleton_key_expr JSONB,
    ADD COLUMN IF NOT EXISTS singleton_on_conflict TEXT,
    ADD COLUMN IF NOT EXISTS singleton_max_queue_depth INT;

-- Runtime resolved key on run rows (nullable).
ALTER TABLE job_runs
    ADD COLUMN IF NOT EXISTS singleton_key TEXT;

ALTER TABLE workflow_runs
    ADD COLUMN IF NOT EXISTS singleton_key TEXT;
