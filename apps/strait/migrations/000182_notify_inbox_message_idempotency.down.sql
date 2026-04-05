DROP INDEX IF EXISTS idx_notify_inbox_items_message_id;

ALTER TABLE inbox_items
    DROP COLUMN IF EXISTS message_id;
