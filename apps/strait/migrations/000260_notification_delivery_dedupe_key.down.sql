DROP INDEX IF EXISTS idx_notification_deliveries_dedupe_key;

ALTER TABLE notification_deliveries
    DROP COLUMN IF EXISTS dedupe_key;
