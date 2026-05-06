ALTER TABLE webhook_deliveries
    ADD COLUMN IF NOT EXISTS claim_token TEXT,
    ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending_claim
    ON webhook_deliveries (next_retry_at ASC, created_at ASC)
    WHERE status = 'pending' AND next_retry_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_claim_lease
    ON webhook_deliveries (lease_expires_at)
    WHERE claim_token IS NOT NULL;
