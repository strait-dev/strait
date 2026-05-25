DROP TRIGGER IF EXISTS trg_queue_entry_ack_on_run_status ON job_runs;
DROP FUNCTION IF EXISTS queue_entry_ack_on_run_status();
DROP TABLE IF EXISTS queue_entries;
DROP TABLE IF EXISTS queue_batch_ticks;
DROP TABLE IF EXISTS queue_batches;
