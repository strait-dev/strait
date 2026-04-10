CREATE TABLE IF NOT EXISTS notify_provider_callback_receipts (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id    TEXT NOT NULL,
    provider_id   TEXT NOT NULL,
    provider      TEXT NOT NULL,
    callback_id   TEXT NOT NULL,
    event_type    TEXT NOT NULL DEFAULT '',
    message_id    TEXT NOT NULL DEFAULT '',
    payload_hash  TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_notify_callback_receipts_identity
    ON notify_provider_callback_receipts (project_id, provider_id, callback_id);

CREATE INDEX IF NOT EXISTS idx_notify_callback_receipts_expiry
    ON notify_provider_callback_receipts (expires_at);
