ALTER TABLE worker_tasks
    ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS result_status TEXT,
    ADD COLUMN IF NOT EXISTS result_output JSONB,
    ADD COLUMN IF NOT EXISTS result_error TEXT,
    ADD COLUMN IF NOT EXISTS result_duration_ms BIGINT,
    ADD COLUMN IF NOT EXISTS result_received_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_worker_tasks_assignment
    ON worker_tasks (id, worker_id, project_id, run_id, attempt, status);
