DROP INDEX IF EXISTS idx_workflow_progression_events_processed_cleanup;
DROP INDEX IF EXISTS idx_queue_entries_acked_cleanup;
DROP INDEX IF EXISTS idx_queue_entries_claimable_worker_denorm;
DROP INDEX IF EXISTS idx_queue_entries_claimable_http_denorm;

ALTER TABLE queue_entries
    DROP COLUMN IF EXISTS environment_id,
    DROP COLUMN IF EXISTS queue_name,
    DROP COLUMN IF EXISTS execution_mode;
