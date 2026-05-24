DROP INDEX IF EXISTS idx_workers_stream_lease_expires_at;

ALTER TABLE workers
    DROP COLUMN IF EXISTS stream_lease_expires_at;
