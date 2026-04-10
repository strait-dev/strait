CREATE TABLE IF NOT EXISTS notification_batches (
    id             TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id     TEXT NOT NULL,
    recipient_type TEXT NOT NULL,
    recipient_id   TEXT NOT NULL,
    batch_key      TEXT NOT NULL,
    channel        TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'collecting',
    events         JSONB NOT NULL DEFAULT '[]',
    event_count    INT NOT NULL DEFAULT 0,
    window_start   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    window_end     TIMESTAMPTZ NOT NULL,
    sent_at        TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notify_batches_flush
    ON notification_batches(status, window_end)
    WHERE status = 'collecting';

CREATE UNIQUE INDEX IF NOT EXISTS idx_notify_batches_active
    ON notification_batches(project_id, recipient_id, batch_key, channel)
    WHERE status = 'collecting';
