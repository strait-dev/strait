DROP TRIGGER IF EXISTS trg_queue_entry_sync_on_queued_status ON job_runs;
DROP FUNCTION IF EXISTS queue_entry_sync_on_queued_status();
DROP INDEX IF EXISTS idx_queue_entries_claimable_batch_order;
DROP TABLE IF EXISTS queue_batch_seal_state;
