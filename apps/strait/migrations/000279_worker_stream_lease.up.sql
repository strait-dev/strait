ALTER TABLE workers
    ADD COLUMN IF NOT EXISTS stream_lease_expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_workers_stream_lease_expires_at
    ON workers (stream_lease_expires_at);
