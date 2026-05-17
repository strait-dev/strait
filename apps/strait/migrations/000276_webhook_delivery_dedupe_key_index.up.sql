CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_webhook_deliveries_dedupe_key
    ON webhook_deliveries (dedupe_key)
    WHERE dedupe_key IS NOT NULL AND dedupe_key <> '';
