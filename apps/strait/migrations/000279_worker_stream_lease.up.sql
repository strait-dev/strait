ALTER TABLE workers
    ADD COLUMN IF NOT EXISTS stream_lease_expires_at TIMESTAMPTZ;

-- safety-ok: supports disconnect sweep lookups on the bounded workers table;
-- golang-migrate runs this migration in a transaction, so CONCURRENTLY is not
-- viable here.
CREATE INDEX IF NOT EXISTS idx_workers_stream_lease_expires_at
    ON workers (stream_lease_expires_at);
