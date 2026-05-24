DROP INDEX IF EXISTS idx_webhook_deliveries_claim_lease;
DROP INDEX IF EXISTS idx_webhook_deliveries_pending_claim;

ALTER TABLE webhook_deliveries
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS claim_token;
