ALTER TABLE processed_webhook_messages
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'processed';

ALTER TABLE processed_webhook_messages
    ADD CONSTRAINT processed_webhook_messages_status_check
    CHECK (status IN ('processing', 'processed'));

CREATE INDEX IF NOT EXISTS idx_processed_webhook_messages_status_ttl
    ON processed_webhook_messages (status, processed_at);
