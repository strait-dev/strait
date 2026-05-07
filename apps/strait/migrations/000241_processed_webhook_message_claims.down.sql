DROP INDEX IF EXISTS idx_processed_webhook_messages_status_ttl;

ALTER TABLE processed_webhook_messages
    DROP CONSTRAINT IF EXISTS processed_webhook_messages_status_check;

ALTER TABLE processed_webhook_messages
    DROP COLUMN IF EXISTS status;
