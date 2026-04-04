CREATE TABLE IF NOT EXISTS processed_webhook_messages (
    msg_id TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_processed_webhook_messages_ttl ON processed_webhook_messages (processed_at);
