ALTER TABLE processed_webhook_messages
    ADD COLUMN IF NOT EXISTS status TEXT;

UPDATE processed_webhook_messages
SET status = 'processed'
WHERE status IS NULL;

ALTER TABLE processed_webhook_messages
    ALTER COLUMN status SET DEFAULT 'processed';

ALTER TABLE processed_webhook_messages
    ADD CONSTRAINT processed_webhook_messages_status_check
    CHECK (status IN ('processing', 'processed'));
