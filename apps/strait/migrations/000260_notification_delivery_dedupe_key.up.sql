ALTER TABLE notification_deliveries
    ADD COLUMN IF NOT EXISTS dedupe_key TEXT;
