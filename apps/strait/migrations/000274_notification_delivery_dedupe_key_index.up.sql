CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_notification_deliveries_dedupe_key
    ON notification_deliveries (dedupe_key)
    WHERE dedupe_key IS NOT NULL AND dedupe_key <> '';
