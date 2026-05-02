ALTER TABLE inbox_items
    ADD COLUMN IF NOT EXISTS message_id TEXT;

-- safety-ok: notify feature is pre-launch; inbox_items has no production rows yet
CREATE UNIQUE INDEX IF NOT EXISTS idx_notify_inbox_items_message_id
    ON inbox_items(message_id)
    WHERE message_id IS NOT NULL;
