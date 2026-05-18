DROP INDEX IF EXISTS idx_worker_tasks_assignment;

ALTER TABLE worker_tasks
    DROP COLUMN IF EXISTS result_received_at,
    DROP COLUMN IF EXISTS result_duration_ms,
    DROP COLUMN IF EXISTS result_error,
    DROP COLUMN IF EXISTS result_output,
    DROP COLUMN IF EXISTS result_status,
    DROP COLUMN IF EXISTS attempt;
