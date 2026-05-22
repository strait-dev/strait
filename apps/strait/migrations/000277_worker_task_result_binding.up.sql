-- safety-ok: worker_tasks is an operational handoff table with bounded active rows;
-- Postgres 11+ stores the constant DEFAULT metadata-only, and the column is
-- required immediately to bind task results to the original attempt.
ALTER TABLE worker_tasks
    ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS result_status TEXT,
    ADD COLUMN IF NOT EXISTS result_output JSONB,
    ADD COLUMN IF NOT EXISTS result_error TEXT,
    ADD COLUMN IF NOT EXISTS result_duration_ms BIGINT,
    ADD COLUMN IF NOT EXISTS result_received_at TIMESTAMPTZ;

-- safety-ok: supports worker result binding lookups on the bounded worker_tasks
-- table; golang-migrate wraps migrations in a transaction, so CONCURRENTLY is
-- not viable here.
CREATE INDEX IF NOT EXISTS idx_worker_tasks_assignment
    ON worker_tasks (id, worker_id, project_id, run_id, attempt, status);
