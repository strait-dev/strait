CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_queue_entries_leased_key_denorm
    ON queue_entries(job_id, concurrency_key, run_id)
    WHERE status = 'leased' AND run_status = 'queued';

DROP TRIGGER IF EXISTS queue_entries_lease_counts_trg ON queue_entries;
DROP FUNCTION IF EXISTS job_batchlog_lease_counts_apply();
DROP TABLE IF EXISTS job_batchlog_lease_counts;
