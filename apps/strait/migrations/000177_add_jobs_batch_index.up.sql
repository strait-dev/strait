-- Add index on (batch_window_secs, batch_max_size) to support queries that
-- locate batch-enabled jobs. Without this index, finding jobs with
-- batch_window_secs > 0 or a specific batch_max_size requires a full table scan.
CREATE INDEX IF NOT EXISTS idx_jobs_on_batch_window_secs_batch_max_size
    ON jobs (batch_window_secs, batch_max_size);
