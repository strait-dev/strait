CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_webhook_deliveries_pending_claim
    ON webhook_deliveries (next_retry_at ASC, created_at ASC)
    WHERE status = 'pending' AND next_retry_at IS NOT NULL;
