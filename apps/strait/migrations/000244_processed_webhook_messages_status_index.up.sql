CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_processed_webhook_messages_status_ttl
    ON processed_webhook_messages (status, processed_at);
