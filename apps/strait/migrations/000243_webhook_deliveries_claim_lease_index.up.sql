CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_webhook_deliveries_claim_lease
    ON webhook_deliveries (lease_expires_at)
    WHERE claim_token IS NOT NULL;
